/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ctrl "sigs.k8s.io/controller-runtime"

	searchv1alpha1 "github.com/example/elasticsearch-operator/api/v1alpha1"
)

var _ = Describe("ElasticsearchCluster Controller", func() {
	var (
		ctx        context.Context
		reconciler *ElasticsearchClusterReconciler
		cr         *searchv1alpha1.ElasticsearchCluster
		ns         *corev1.Namespace
		crName     string
		nsName     string
	)

	BeforeEach(func() {
		ctx = context.Background()

		nsName = fmt.Sprintf("test-ns-%d", time.Now().UnixNano())
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: nsName},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		crName = fmt.Sprintf("test-es-%d", time.Now().UnixNano())
		cr = &searchv1alpha1.ElasticsearchCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      crName,
				Namespace: nsName,
			},
			Spec: searchv1alpha1.ElasticsearchClusterSpec{
				Replicas: 3,
				Version:  "8.14",
				Storage: searchv1alpha1.StorageSpec{
					Size: "10Gi",
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())

		// Re-fetch to get UID and ResourceVersion for owner references
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: nsName}, cr)).To(Succeed())

		reconciler = &ElasticsearchClusterReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: record.NewFakeRecorder(100),
		}
	})

	AfterEach(func() {
		// Remove finalizer before deletion
		current := &searchv1alpha1.ElasticsearchCluster{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: nsName}, current); err == nil {
			if controllerutil.ContainsFinalizer(current, elasticsearchClusterFinalizer) {
				controllerutil.RemoveFinalizer(current, elasticsearchClusterFinalizer)
				_ = k8sClient.Update(ctx, current)
			}
			_ = k8sClient.Delete(ctx, current)
		}
		_ = k8sClient.Delete(ctx, ns)
	})

	// -----------------------------------------------------------------------
	// Lifecycle tests
	// -----------------------------------------------------------------------
	Describe("When reconciling a ElasticsearchCluster", func() {
		It("should add finalizer on first reconciliation", func() {
			_, err := reconciler.Reconcile(ctx, reconcileRequest(crName, nsName))
			Expect(err).NotTo(HaveOccurred())

			updated := &searchv1alpha1.ElasticsearchCluster{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: nsName}, updated)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updated, elasticsearchClusterFinalizer)).To(BeTrue())
		})

		It("should create all managed resources", func() {
			_, err := reconciler.Reconcile(ctx, reconcileRequest(crName, nsName))
			Expect(err).NotTo(HaveOccurred())

			// Secret
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-auth", Namespace: nsName}, secret)).To(Succeed())

			// ConfigMap
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-config", Namespace: nsName}, cm)).To(Succeed())

			// HTTP Service
			httpSvc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-http", Namespace: nsName}, httpSvc)).To(Succeed())

			// Transport Service
			transportSvc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-transport", Namespace: nsName}, transportSvc)).To(Succeed())

			// StatefulSet
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: nsName}, sts)).To(Succeed())
		})

		It("should be idempotent on repeated reconciliation", func() {
			// First reconcile
			_, err := reconciler.Reconcile(ctx, reconcileRequest(crName, nsName))
			Expect(err).NotTo(HaveOccurred())

			// Capture resource versions
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-auth", Namespace: nsName}, secret)).To(Succeed())
			secretRV := secret.ResourceVersion

			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-config", Namespace: nsName}, cm)).To(Succeed())
			cmRV := cm.ResourceVersion

			// Second reconcile
			_, err = reconciler.Reconcile(ctx, reconcileRequest(crName, nsName))
			Expect(err).NotTo(HaveOccurred())

			// Verify resource versions unchanged
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-auth", Namespace: nsName}, secret)).To(Succeed())
			Expect(secret.ResourceVersion).To(Equal(secretRV))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-config", Namespace: nsName}, cm)).To(Succeed())
			Expect(cm.ResourceVersion).To(Equal(cmRV))
		})

		It("should handle deletion with finalizer cleanup", func() {
			// First reconcile to add finalizer and create resources
			_, err := reconciler.Reconcile(ctx, reconcileRequest(crName, nsName))
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer is present
			updated := &searchv1alpha1.ElasticsearchCluster{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: nsName}, updated)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updated, elasticsearchClusterFinalizer)).To(BeTrue())

			// Delete the CR
			Expect(k8sClient.Delete(ctx, updated)).To(Succeed())

			// Re-fetch to get the deletion timestamp
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: nsName}, updated)).To(Succeed())
			Expect(updated.DeletionTimestamp).NotTo(BeNil())

			// Reconcile should handle deletion and remove finalizer
			_, err = reconciler.Reconcile(ctx, reconcileRequest(crName, nsName))
			Expect(err).NotTo(HaveOccurred())

			// Verify CR is gone (finalizer removed allows deletion to complete)
			err = k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: nsName}, updated)
			Expect(err).To(HaveOccurred())
		})
	})

	// -----------------------------------------------------------------------
	// reconcileSecret
	// -----------------------------------------------------------------------
	Context("When reconciling Secret", func() {
		It("should create Secret with ELASTIC_PASSWORD", func() {
			Expect(reconciler.reconcileSecret(ctx, cr)).To(Succeed())

			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-auth", Namespace: nsName}, secret)).To(Succeed())
			Expect(secret.Data).To(HaveKey("ELASTIC_PASSWORD"))
			Expect(secret.Data).To(HaveKey("ELASTIC_USERNAME"))
			Expect(string(secret.Data["ELASTIC_USERNAME"])).To(Equal("elastic"))
			Expect(secret.OwnerReferences).To(HaveLen(1))
		})

		It("should not recreate existing Secret (idempotent)", func() {
			Expect(reconciler.reconcileSecret(ctx, cr)).To(Succeed())

			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-auth", Namespace: nsName}, secret)).To(Succeed())
			originalVersion := secret.ResourceVersion

			Expect(reconciler.reconcileSecret(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-auth", Namespace: nsName}, secret)).To(Succeed())
			Expect(secret.ResourceVersion).To(Equal(originalVersion))
		})

		It("should skip Secret creation when existingSecret is set", func() {
			cr.Spec.Auth = &searchv1alpha1.AuthSpec{
				ExistingSecret: "my-existing-secret",
			}
			Expect(reconciler.reconcileSecret(ctx, cr)).To(Succeed())

			secret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-auth", Namespace: nsName}, secret)
			Expect(err).To(HaveOccurred())
		})
	})

	// -----------------------------------------------------------------------
	// reconcileConfigMap
	// -----------------------------------------------------------------------
	Context("When reconciling ConfigMap", func() {
		It("should create ConfigMap with elasticsearch.yml", func() {
			Expect(reconciler.reconcileConfigMap(ctx, cr)).To(Succeed())

			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-config", Namespace: nsName}, cm)).To(Succeed())
			Expect(cm.Data).To(HaveKey("elasticsearch.yml"))
			Expect(cm.Data["elasticsearch.yml"]).To(ContainSubstring("cluster.name: " + crName))
			Expect(cm.Data["elasticsearch.yml"]).To(ContainSubstring("http.port: 9200"))
			Expect(cm.Data["elasticsearch.yml"]).To(ContainSubstring("transport.port: 9300"))
			Expect(cm.OwnerReferences).To(HaveLen(1))
		})

		It("should not recreate existing ConfigMap (idempotent)", func() {
			Expect(reconciler.reconcileConfigMap(ctx, cr)).To(Succeed())

			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-config", Namespace: nsName}, cm)).To(Succeed())
			originalVersion := cm.ResourceVersion

			Expect(reconciler.reconcileConfigMap(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-config", Namespace: nsName}, cm)).To(Succeed())
			Expect(cm.ResourceVersion).To(Equal(originalVersion))
		})
	})

	// -----------------------------------------------------------------------
	// reconcileHTTPService
	// -----------------------------------------------------------------------
	Context("When reconciling HTTP Service", func() {
		It("should create ClusterIP Service on port 9200", func() {
			Expect(reconciler.reconcileHTTPService(ctx, cr)).To(Succeed())

			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-http", Namespace: nsName}, svc)).To(Succeed())
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(9200)))
			Expect(svc.Spec.Ports[0].Name).To(Equal("http"))
			Expect(svc.OwnerReferences).To(HaveLen(1))
		})
	})

	// -----------------------------------------------------------------------
	// reconcileTransportService
	// -----------------------------------------------------------------------
	Context("When reconciling Transport Service", func() {
		It("should create headless Service on port 9300", func() {
			Expect(reconciler.reconcileTransportService(ctx, cr)).To(Succeed())

			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-transport", Namespace: nsName}, svc)).To(Succeed())
			Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(9300)))
			Expect(svc.Spec.Ports[0].Name).To(Equal("transport"))
			Expect(svc.OwnerReferences).To(HaveLen(1))
		})
	})

	// -----------------------------------------------------------------------
	// reconcileStatefulSet
	// -----------------------------------------------------------------------
	Context("When reconciling StatefulSet", func() {
		It("should create StatefulSet with replicas and 2 ports", func() {
			Expect(reconciler.reconcileStatefulSet(ctx, cr)).To(Succeed())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: nsName}, sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
			Expect(sts.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.Containers[0].Ports).To(HaveLen(2))
			Expect(sts.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(9200)))
			Expect(sts.Spec.Template.Spec.Containers[0].Ports[1].ContainerPort).To(Equal(int32(9300)))
			Expect(sts.OwnerReferences).To(HaveLen(1))
		})

		It("should update replicas when spec changes", func() {
			Expect(reconciler.reconcileStatefulSet(ctx, cr)).To(Succeed())

			// Update CR replicas
			cr.Spec.Replicas = 5
			Expect(reconciler.reconcileStatefulSet(ctx, cr)).To(Succeed())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: nsName}, sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(5)))
		})
	})

	// -----------------------------------------------------------------------
	// reconcileBackupCronJob
	// -----------------------------------------------------------------------
	Context("When reconciling Backup CronJob", func() {
		It("should create CronJob when backup is enabled with schedule", func() {
			cr.Spec.Backup = &searchv1alpha1.BackupSpec{
				Enabled:  true,
				Schedule: "0 2 * * *",
			}
			Expect(reconciler.reconcileBackupCronJob(ctx, cr)).To(Succeed())

			cronJob := &batchv1.CronJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-backup", Namespace: nsName}, cronJob)).To(Succeed())
			Expect(cronJob.Spec.Schedule).To(Equal("0 2 * * *"))
			Expect(cronJob.OwnerReferences).To(HaveLen(1))
		})

		It("should not create CronJob when backup is disabled", func() {
			cr.Spec.Backup = nil
			Expect(reconciler.reconcileBackupCronJob(ctx, cr)).To(Succeed())

			cronJob := &batchv1.CronJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-backup", Namespace: nsName}, cronJob)
			Expect(err).To(HaveOccurred())
		})

		It("should update schedule when it changes", func() {
			cr.Spec.Backup = &searchv1alpha1.BackupSpec{
				Enabled:  true,
				Schedule: "0 2 * * *",
			}
			Expect(reconciler.reconcileBackupCronJob(ctx, cr)).To(Succeed())

			// Update schedule
			cr.Spec.Backup.Schedule = "0 4 * * *"
			Expect(reconciler.reconcileBackupCronJob(ctx, cr)).To(Succeed())

			cronJob := &batchv1.CronJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-backup", Namespace: nsName}, cronJob)).To(Succeed())
			Expect(cronJob.Spec.Schedule).To(Equal("0 4 * * *"))
		})
	})

	// -----------------------------------------------------------------------
	// reconcileMaster
	// -----------------------------------------------------------------------
	Context("When reconciling Master Deployment", func() {
		It("should create master Deployment when enabled", func() {
			cr.Spec.Master = &searchv1alpha1.MasterSpec{
				Enabled:  true,
				Replicas: 3,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			}
			Expect(reconciler.reconcileMaster(ctx, cr)).To(Succeed())

			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-master", Namespace: nsName}, dep)).To(Succeed())
			Expect(*dep.Spec.Replicas).To(Equal(int32(3)))
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(9200)))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[1].ContainerPort).To(Equal(int32(9300)))
			Expect(dep.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "master"))
			Expect(dep.OwnerReferences).To(HaveLen(1))
		})

		It("should not create master when disabled", func() {
			cr.Spec.Master = nil
			Expect(reconciler.reconcileMaster(ctx, cr)).To(Succeed())

			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-master", Namespace: nsName}, dep)
			Expect(err).To(HaveOccurred())
		})

		It("should be idempotent for master", func() {
			cr.Spec.Master = &searchv1alpha1.MasterSpec{
				Enabled:  true,
				Replicas: 3,
			}
			Expect(reconciler.reconcileMaster(ctx, cr)).To(Succeed())

			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-master", Namespace: nsName}, dep)).To(Succeed())
			originalVersion := dep.ResourceVersion

			Expect(reconciler.reconcileMaster(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-master", Namespace: nsName}, dep)).To(Succeed())
			Expect(dep.ResourceVersion).To(Equal(originalVersion))
		})

		It("should delete master when disabled", func() {
			cr.Spec.Master = &searchv1alpha1.MasterSpec{
				Enabled:  true,
				Replicas: 3,
			}
			Expect(reconciler.reconcileMaster(ctx, cr)).To(Succeed())

			// Verify it was created
			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-master", Namespace: nsName}, dep)).To(Succeed())

			// Disable master
			cr.Spec.Master.Enabled = false
			Expect(reconciler.reconcileMaster(ctx, cr)).To(Succeed())

			// Verify it was deleted
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-master", Namespace: nsName}, dep)
			Expect(err).To(HaveOccurred())
		})
	})

	// -----------------------------------------------------------------------
	// reconcileNetworkPolicy
	// -----------------------------------------------------------------------
	Context("When reconciling NetworkPolicy", func() {
		It("should create NetworkPolicy with correct ports and labels", func() {
			Expect(reconciler.reconcileNetworkPolicy(ctx, cr)).To(Succeed())

			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-network-policy", Namespace: nsName}, np)).To(Succeed())

			// Verify labels match
			expectedLabels := labelsForElasticsearchCluster(cr)
			for k, v := range expectedLabels {
				Expect(np.Labels).To(HaveKeyWithValue(k, v))
			}

			// Verify pod selector
			Expect(np.Spec.PodSelector.MatchLabels).To(Equal(expectedLabels))

			// Verify policy types
			Expect(np.Spec.PolicyTypes).To(ConsistOf(
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			))

			// Verify ingress rules — ports 9200 and 9300
			Expect(np.Spec.Ingress).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].Ports).To(HaveLen(2))
			Expect(np.Spec.Ingress[0].Ports[0].Port.IntValue()).To(Equal(9200))
			Expect(np.Spec.Ingress[0].Ports[1].Port.IntValue()).To(Equal(9300))

			// Verify owner reference
			Expect(np.OwnerReferences).To(HaveLen(1))
			Expect(np.OwnerReferences[0].Name).To(Equal(crName))
		})

		It("should not recreate existing NetworkPolicy (idempotent)", func() {
			Expect(reconciler.reconcileNetworkPolicy(ctx, cr)).To(Succeed())

			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-network-policy", Namespace: nsName}, np)).To(Succeed())
			originalVersion := np.ResourceVersion

			Expect(reconciler.reconcileNetworkPolicy(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-network-policy", Namespace: nsName}, np)).To(Succeed())
			Expect(np.ResourceVersion).To(Equal(originalVersion))
		})
	})

	// -----------------------------------------------------------------------
	// Helper functions
	// -----------------------------------------------------------------------
	Context("When testing helper functions", func() {
		It("should generate correct labels", func() {
			labels := labelsForElasticsearchCluster(cr)
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", "elasticsearch"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/instance", crName))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "elasticsearch-operator"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/part-of", crName))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/version", "8.14"))
		})

		It("should generate password with correct length", func() {
			password := generatePassword()
			Expect(password).To(HaveLen(passwordLength))
			// Verify a second call produces a different password (randomness)
			password2 := generatePassword()
			Expect(password).NotTo(Equal(password2))
		})
	})
})

// reconcileRequest creates a ctrl.Request for the given name and namespace.
func reconcileRequest(name, namespace string) ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
}

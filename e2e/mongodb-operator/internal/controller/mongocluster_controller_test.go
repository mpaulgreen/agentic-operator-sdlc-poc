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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	databasev1alpha1 "github.com/example/mongodb-operator/api/v1alpha1"
)

var _ = Describe("MongoCluster Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	// ============================================================
	// Lifecycle Tests
	// ============================================================
	Context("When reconciling a MongoCluster", func() {
		var (
			ctx        context.Context
			name       string
			namespace  string
			key        types.NamespacedName
			cr         *databasev1alpha1.MongoCluster
			reconciler *MongoClusterReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			name = fmt.Sprintf("test-%d", time.Now().UnixNano())
			namespace = "default"
			key = types.NamespacedName{Name: name, Namespace: namespace}

			cr = &databasev1alpha1.MongoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: databasev1alpha1.MongoClusterSpec{
					Replicas: 3,
					Version:  "7.0",
					Storage: databasev1alpha1.StorageSpec{
						Size: "10Gi",
					},
				},
			}

			reconciler = &MongoClusterReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			resource := &databasev1alpha1.MongoCluster{}
			if err := k8sClient.Get(ctx, key, resource); err == nil {
				// Remove finalizer to allow cleanup
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should add finalizer on first reconciliation", func() {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			updated := &databasev1alpha1.MongoCluster{}
			Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement("database.mongodb.example.com/finalizer"))
		})

		It("should create all managed resources", func() {
			// Multiple reconciliations to ensure all resources are created
			for i := 0; i < 3; i++ {
				_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			}

			// Verify Admin Secret
			adminSecret := &corev1.Secret{}
			adminKey := types.NamespacedName{Name: fmt.Sprintf("%s-admin", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, adminKey, adminSecret)).To(Succeed())

			// Verify KeyFile Secret
			keyFileSecret := &corev1.Secret{}
			keyFileKey := types.NamespacedName{Name: fmt.Sprintf("%s-keyfile", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, keyFileKey, keyFileSecret)).To(Succeed())

			// Verify ConfigMap
			configMap := &corev1.ConfigMap{}
			configMapKey := types.NamespacedName{Name: fmt.Sprintf("%s-config", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, configMapKey, configMap)).To(Succeed())

			// Verify Headless Service
			headlessSvc := &corev1.Service{}
			headlessKey := types.NamespacedName{Name: fmt.Sprintf("%s-headless", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, headlessKey, headlessSvc)).To(Succeed())

			// Verify Client Service
			clientSvc := &corev1.Service{}
			clientKey := types.NamespacedName{Name: fmt.Sprintf("%s-client", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, clientKey, clientSvc)).To(Succeed())

			// Verify StatefulSet
			statefulSet := &appsv1.StatefulSet{}
			stsKey := types.NamespacedName{Name: name, Namespace: namespace}
			Expect(k8sClient.Get(ctx, stsKey, statefulSet)).To(Succeed())
		})

		It("should be idempotent on repeated reconciliation", func() {
			// First reconcile creates everything
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile should succeed without errors
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			_ = result
		})

		It("should handle deletion with finalizer cleanup", func() {
			// First reconcile to add finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updated := &databasev1alpha1.MongoCluster{}
			Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
			Expect(updated.Finalizers).NotTo(BeEmpty())

			// Delete the resource
			Expect(k8sClient.Delete(ctx, updated)).To(Succeed())

			// Reconcile should handle deletion and remove finalizer
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was removed (resource may or may not still exist)
			deleted := &databasev1alpha1.MongoCluster{}
			err = k8sClient.Get(ctx, key, deleted)
			if err == nil {
				Expect(deleted.Finalizers).To(BeEmpty())
			}
			// If err != nil, resource was already garbage collected -- expected
		})
	})

	// ============================================================
	// Per-Method Tests: reconcileAdminSecret
	// ============================================================
	Context("When reconciling Admin Secret", func() {
		var (
			ctx        context.Context
			name       string
			namespace  string
			key        types.NamespacedName
			cr         *databasev1alpha1.MongoCluster
			reconciler *MongoClusterReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			name = fmt.Sprintf("test-%d", time.Now().UnixNano())
			namespace = "default"
			key = types.NamespacedName{Name: name, Namespace: namespace}

			cr = &databasev1alpha1.MongoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: databasev1alpha1.MongoClusterSpec{
					Replicas: 3,
					Version:  "7.0",
					Storage: databasev1alpha1.StorageSpec{
						Size: "10Gi",
					},
				},
			}

			reconciler = &MongoClusterReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			// Re-fetch to get UID for owner references
			Expect(k8sClient.Get(ctx, key, cr)).To(Succeed())
		})

		AfterEach(func() {
			resource := &databasev1alpha1.MongoCluster{}
			if err := k8sClient.Get(ctx, key, resource); err == nil {
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create Secret with MONGO_INITDB_ROOT_PASSWORD key when absent", func() {
			Expect(reconciler.reconcileAdminSecret(ctx, cr)).To(Succeed())

			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{Name: fmt.Sprintf("%s-admin", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, secretKey, secret)).To(Succeed())

			// Verify credential keys exist
			Expect(secret.Data).To(HaveKey("MONGO_INITDB_ROOT_PASSWORD"))
			Expect(secret.Data).To(HaveKey("MONGO_INITDB_ROOT_USERNAME"))

			// Verify owner reference
			Expect(secret.OwnerReferences).To(HaveLen(1))
			Expect(secret.OwnerReferences[0].Name).To(Equal(name))

			// Verify labels
			Expect(secret.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "mongodb"))
			Expect(secret.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", name))
			Expect(secret.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "mongodb-operator"))
		})

		It("should not recreate existing Secret (idempotent)", func() {
			Expect(reconciler.reconcileAdminSecret(ctx, cr)).To(Succeed())

			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{Name: fmt.Sprintf("%s-admin", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, secretKey, secret)).To(Succeed())
			originalVersion := secret.ResourceVersion

			// Reconcile again
			Expect(reconciler.reconcileAdminSecret(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, secretKey, secret)).To(Succeed())
			Expect(secret.ResourceVersion).To(Equal(originalVersion))
		})

		It("should skip creation when existingSecret is provided", func() {
			cr.Spec.Auth = &databasev1alpha1.AuthSpec{
				ExistingSecret: "my-existing-secret",
			}
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			Expect(reconciler.reconcileAdminSecret(ctx, cr)).To(Succeed())

			// Verify that no admin Secret was created
			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{Name: fmt.Sprintf("%s-admin", name), Namespace: namespace}
			err := k8sClient.Get(ctx, secretKey, secret)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	// ============================================================
	// Per-Method Tests: reconcileKeyFileSecret
	// ============================================================
	Context("When reconciling KeyFile Secret", func() {
		var (
			ctx        context.Context
			name       string
			namespace  string
			key        types.NamespacedName
			cr         *databasev1alpha1.MongoCluster
			reconciler *MongoClusterReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			name = fmt.Sprintf("test-%d", time.Now().UnixNano())
			namespace = "default"
			key = types.NamespacedName{Name: name, Namespace: namespace}

			cr = &databasev1alpha1.MongoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: databasev1alpha1.MongoClusterSpec{
					Replicas: 3,
					Version:  "7.0",
					Storage: databasev1alpha1.StorageSpec{
						Size: "10Gi",
					},
				},
			}

			reconciler = &MongoClusterReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, key, cr)).To(Succeed())
		})

		AfterEach(func() {
			resource := &databasev1alpha1.MongoCluster{}
			if err := k8sClient.Get(ctx, key, resource); err == nil {
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create keyFile Secret when absent", func() {
			Expect(reconciler.reconcileKeyFileSecret(ctx, cr)).To(Succeed())

			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{Name: fmt.Sprintf("%s-keyfile", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, secretKey, secret)).To(Succeed())

			// Verify keyfile data exists
			Expect(secret.Data).To(HaveKey("keyfile"))

			// Verify owner reference
			Expect(secret.OwnerReferences).To(HaveLen(1))
			Expect(secret.OwnerReferences[0].Name).To(Equal(name))

			// Verify labels
			Expect(secret.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "mongodb"))
			Expect(secret.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "mongodb-operator"))
		})

		It("should skip creation when auth.keyFile is provided", func() {
			cr.Spec.Auth = &databasev1alpha1.AuthSpec{
				KeyFile: "my-existing-keyfile",
			}
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			Expect(reconciler.reconcileKeyFileSecret(ctx, cr)).To(Succeed())

			// Verify that no keyfile Secret was created
			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{Name: fmt.Sprintf("%s-keyfile", name), Namespace: namespace}
			err := k8sClient.Get(ctx, secretKey, secret)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	// ============================================================
	// Per-Method Tests: reconcileConfigMap
	// ============================================================
	Context("When reconciling ConfigMap", func() {
		var (
			ctx        context.Context
			name       string
			namespace  string
			key        types.NamespacedName
			cr         *databasev1alpha1.MongoCluster
			reconciler *MongoClusterReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			name = fmt.Sprintf("test-%d", time.Now().UnixNano())
			namespace = "default"
			key = types.NamespacedName{Name: name, Namespace: namespace}

			cr = &databasev1alpha1.MongoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: databasev1alpha1.MongoClusterSpec{
					Replicas: 3,
					Version:  "7.0",
					Storage: databasev1alpha1.StorageSpec{
						Size: "10Gi",
					},
				},
			}

			reconciler = &MongoClusterReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, key, cr)).To(Succeed())
		})

		AfterEach(func() {
			resource := &databasev1alpha1.MongoCluster{}
			if err := k8sClient.Get(ctx, key, resource); err == nil {
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create ConfigMap with mongod.conf when absent", func() {
			Expect(reconciler.reconcileConfigMap(ctx, cr)).To(Succeed())

			configMap := &corev1.ConfigMap{}
			cmKey := types.NamespacedName{Name: fmt.Sprintf("%s-config", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, cmKey, configMap)).To(Succeed())

			// Verify configuration content
			Expect(configMap.Data).To(HaveKey("mongod.conf"))
			Expect(configMap.Data["mongod.conf"]).To(ContainSubstring("port: 27017"))
			Expect(configMap.Data["mongod.conf"]).To(ContainSubstring("replication"))
			Expect(configMap.Data["mongod.conf"]).To(ContainSubstring("replSetName"))

			// Verify owner reference
			Expect(configMap.OwnerReferences).To(HaveLen(1))
			Expect(configMap.OwnerReferences[0].Name).To(Equal(name))

			// Verify labels
			Expect(configMap.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "mongodb"))
			Expect(configMap.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "mongodb-operator"))
		})

		It("should not recreate existing ConfigMap (idempotent)", func() {
			Expect(reconciler.reconcileConfigMap(ctx, cr)).To(Succeed())

			configMap := &corev1.ConfigMap{}
			cmKey := types.NamespacedName{Name: fmt.Sprintf("%s-config", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, cmKey, configMap)).To(Succeed())
			originalVersion := configMap.ResourceVersion

			// Reconcile again
			Expect(reconciler.reconcileConfigMap(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, cmKey, configMap)).To(Succeed())
			Expect(configMap.ResourceVersion).To(Equal(originalVersion))
		})
	})

	// ============================================================
	// Per-Method Tests: reconcileServices
	// ============================================================
	Context("When reconciling Services", func() {
		var (
			ctx        context.Context
			name       string
			namespace  string
			key        types.NamespacedName
			cr         *databasev1alpha1.MongoCluster
			reconciler *MongoClusterReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			name = fmt.Sprintf("test-%d", time.Now().UnixNano())
			namespace = "default"
			key = types.NamespacedName{Name: name, Namespace: namespace}

			cr = &databasev1alpha1.MongoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: databasev1alpha1.MongoClusterSpec{
					Replicas: 3,
					Version:  "7.0",
					Storage: databasev1alpha1.StorageSpec{
						Size: "10Gi",
					},
				},
			}

			reconciler = &MongoClusterReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, key, cr)).To(Succeed())
		})

		AfterEach(func() {
			resource := &databasev1alpha1.MongoCluster{}
			if err := k8sClient.Get(ctx, key, resource); err == nil {
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create headless Service with ClusterIP None and port 27017", func() {
			Expect(reconciler.reconcileHeadlessService(ctx, cr)).To(Succeed())

			service := &corev1.Service{}
			svcKey := types.NamespacedName{Name: fmt.Sprintf("%s-headless", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, svcKey, service)).To(Succeed())

			// Verify headless (ClusterIP: None)
			Expect(service.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))

			// Verify port
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(27017)))
			Expect(service.Spec.Ports[0].Name).To(Equal("mongodb"))

			// Verify owner reference
			Expect(service.OwnerReferences).To(HaveLen(1))
			Expect(service.OwnerReferences[0].Name).To(Equal(name))

			// Verify selector matches labels
			Expect(service.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/instance", name))
		})

		It("should create client Service with ClusterIP and port 27017", func() {
			Expect(reconciler.reconcileClientService(ctx, cr)).To(Succeed())

			service := &corev1.Service{}
			svcKey := types.NamespacedName{Name: fmt.Sprintf("%s-client", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, svcKey, service)).To(Succeed())

			// Verify NOT headless (regular ClusterIP)
			Expect(service.Spec.ClusterIP).NotTo(Equal(corev1.ClusterIPNone))

			// Verify port
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(27017)))
			Expect(service.Spec.Ports[0].Name).To(Equal("mongodb"))

			// Verify owner reference
			Expect(service.OwnerReferences).To(HaveLen(1))
			Expect(service.OwnerReferences[0].Name).To(Equal(name))

			// Verify selector matches labels
			Expect(service.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/instance", name))
		})
	})

	// ============================================================
	// Per-Method Tests: reconcileStatefulSet
	// ============================================================
	Context("When reconciling StatefulSet", func() {
		var (
			ctx        context.Context
			name       string
			namespace  string
			key        types.NamespacedName
			cr         *databasev1alpha1.MongoCluster
			reconciler *MongoClusterReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			name = fmt.Sprintf("test-%d", time.Now().UnixNano())
			namespace = "default"
			key = types.NamespacedName{Name: name, Namespace: namespace}

			cr = &databasev1alpha1.MongoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: databasev1alpha1.MongoClusterSpec{
					Replicas: 3,
					Version:  "7.0",
					Storage: databasev1alpha1.StorageSpec{
						Size: "10Gi",
					},
				},
			}

			reconciler = &MongoClusterReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, key, cr)).To(Succeed())
		})

		AfterEach(func() {
			resource := &databasev1alpha1.MongoCluster{}
			if err := k8sClient.Get(ctx, key, resource); err == nil {
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create StatefulSet with correct replicas and image", func() {
			Expect(reconciler.reconcileStatefulSet(ctx, cr)).To(Succeed())

			statefulSet := &appsv1.StatefulSet{}
			stsKey := types.NamespacedName{Name: name, Namespace: namespace}
			Expect(k8sClient.Get(ctx, stsKey, statefulSet)).To(Succeed())

			// Verify replicas
			Expect(*statefulSet.Spec.Replicas).To(Equal(int32(3)))

			// Verify image contains mongodb
			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring("mongodb"))

			// Verify headless service name
			Expect(statefulSet.Spec.ServiceName).To(Equal(fmt.Sprintf("%s-headless", name)))

			// Verify container port
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Ports).To(HaveLen(1))
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(27017)))

			// Verify volume mounts (data + config + keyfile)
			Expect(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(3))

			// Verify VolumeClaimTemplates
			Expect(statefulSet.Spec.VolumeClaimTemplates).To(HaveLen(1))
			Expect(statefulSet.Spec.VolumeClaimTemplates[0].Name).To(Equal("data"))

			// Verify owner reference
			Expect(statefulSet.OwnerReferences).To(HaveLen(1))
			Expect(statefulSet.OwnerReferences[0].Name).To(Equal(name))
		})

		It("should update replicas on spec change", func() {
			Expect(reconciler.reconcileStatefulSet(ctx, cr)).To(Succeed())

			statefulSet := &appsv1.StatefulSet{}
			stsKey := types.NamespacedName{Name: name, Namespace: namespace}
			Expect(k8sClient.Get(ctx, stsKey, statefulSet)).To(Succeed())
			originalVersion := statefulSet.ResourceVersion

			// Change replicas from 3 to 5
			cr.Spec.Replicas = 5
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())
			Expect(reconciler.reconcileStatefulSet(ctx, cr)).To(Succeed())

			Expect(k8sClient.Get(ctx, stsKey, statefulSet)).To(Succeed())
			Expect(statefulSet.ResourceVersion).NotTo(Equal(originalVersion))
			Expect(*statefulSet.Spec.Replicas).To(Equal(int32(5)))
		})
	})

	// ============================================================
	// Per-Method Tests: reconcileBackupJob
	// ============================================================
	Context("When reconciling Backup Job", func() {
		var (
			ctx        context.Context
			name       string
			namespace  string
			key        types.NamespacedName
			cr         *databasev1alpha1.MongoCluster
			reconciler *MongoClusterReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			name = fmt.Sprintf("test-%d", time.Now().UnixNano())
			namespace = "default"
			key = types.NamespacedName{Name: name, Namespace: namespace}

			cr = &databasev1alpha1.MongoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: databasev1alpha1.MongoClusterSpec{
					Replicas: 3,
					Version:  "7.0",
					Storage: databasev1alpha1.StorageSpec{
						Size: "10Gi",
					},
				},
			}

			reconciler = &MongoClusterReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, key, cr)).To(Succeed())
		})

		AfterEach(func() {
			resource := &databasev1alpha1.MongoCluster{}
			if err := k8sClient.Get(ctx, key, resource); err == nil {
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create backup Job when backup.enabled is true", func() {
			cr.Spec.Backup = &databasev1alpha1.BackupSpec{
				Enabled:       true,
				RetentionDays: 7,
			}
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			Expect(reconciler.reconcileBackupJob(ctx, cr)).To(Succeed())

			// List Jobs with backup label to find the created Job
			jobList := &batchv1.JobList{}
			Expect(k8sClient.List(ctx, jobList, &client.ListOptions{
				Namespace: namespace,
			})).To(Succeed())

			found := false
			for _, job := range jobList.Items {
				if lbls := job.Labels; lbls != nil {
					if lbls["app.kubernetes.io/component"] == "backup" && lbls["app.kubernetes.io/instance"] == name {
						found = true
						// Verify owner reference
						Expect(job.OwnerReferences).To(HaveLen(1))
						Expect(job.OwnerReferences[0].Name).To(Equal(name))

						// Verify container uses mongodump
						Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
						Expect(job.Spec.Template.Spec.Containers[0].Command).To(ContainElement("/bin/sh"))
					}
				}
			}
			Expect(found).To(BeTrue(), "Expected backup Job to be created")
		})

		It("should not create duplicate Job when one is active", func() {
			cr.Spec.Backup = &databasev1alpha1.BackupSpec{
				Enabled:       true,
				RetentionDays: 7,
			}
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			// First call creates a Job
			Expect(reconciler.reconcileBackupJob(ctx, cr)).To(Succeed())

			// Count Jobs after first call
			jobList1 := &batchv1.JobList{}
			Expect(k8sClient.List(ctx, jobList1, &client.ListOptions{
				Namespace: namespace,
			})).To(Succeed())
			initialCount := 0
			for _, job := range jobList1.Items {
				if lbls := job.Labels; lbls != nil {
					if lbls["app.kubernetes.io/component"] == "backup" && lbls["app.kubernetes.io/instance"] == name {
						initialCount++
					}
				}
			}

			// Second call should NOT create another Job (existing one has no status = pending)
			Expect(reconciler.reconcileBackupJob(ctx, cr)).To(Succeed())

			jobList2 := &batchv1.JobList{}
			Expect(k8sClient.List(ctx, jobList2, &client.ListOptions{
				Namespace: namespace,
			})).To(Succeed())
			finalCount := 0
			for _, job := range jobList2.Items {
				if lbls := job.Labels; lbls != nil {
					if lbls["app.kubernetes.io/component"] == "backup" && lbls["app.kubernetes.io/instance"] == name {
						finalCount++
					}
				}
			}
			Expect(finalCount).To(Equal(initialCount))
		})

		It("should not create Job when backup is disabled", func() {
			// Default: no backup spec (nil)
			Expect(reconciler.reconcileBackupJob(ctx, cr)).To(Succeed())

			jobList := &batchv1.JobList{}
			Expect(k8sClient.List(ctx, jobList, &client.ListOptions{
				Namespace: namespace,
			})).To(Succeed())

			for _, job := range jobList.Items {
				if lbls := job.Labels; lbls != nil {
					Expect(lbls["app.kubernetes.io/instance"]).NotTo(Equal(name),
						"No backup Job should be created when backup is disabled")
				}
			}
		})
	})

	// ============================================================
	// Per-Method Tests: reconcileArbiter
	// ============================================================
	Context("When reconciling Arbiter", func() {
		var (
			ctx        context.Context
			name       string
			namespace  string
			key        types.NamespacedName
			cr         *databasev1alpha1.MongoCluster
			reconciler *MongoClusterReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			name = fmt.Sprintf("test-%d", time.Now().UnixNano())
			namespace = "default"
			key = types.NamespacedName{Name: name, Namespace: namespace}

			cr = &databasev1alpha1.MongoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: databasev1alpha1.MongoClusterSpec{
					Replicas: 3,
					Version:  "7.0",
					Storage: databasev1alpha1.StorageSpec{
						Size: "10Gi",
					},
				},
			}

			reconciler = &MongoClusterReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, key, cr)).To(Succeed())
		})

		AfterEach(func() {
			resource := &databasev1alpha1.MongoCluster{}
			if err := k8sClient.Get(ctx, key, resource); err == nil {
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create arbiter Deployment when enabled", func() {
			cr.Spec.Arbiter = &databasev1alpha1.ArbiterSpec{
				Enabled: true,
			}
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			Expect(reconciler.reconcileArbiter(ctx, cr)).To(Succeed())

			deployment := &appsv1.Deployment{}
			depKey := types.NamespacedName{Name: fmt.Sprintf("%s-arbiter", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, depKey, deployment)).To(Succeed())

			// Verify replicas is 1
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))

			// Verify arbiter component label
			Expect(deployment.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "arbiter"))
			Expect(deployment.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", name))
			Expect(deployment.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "mongodb-operator"))

			// Verify container
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal("arbiter"))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring("mongodb"))

			// Verify container port
			Expect(deployment.Spec.Template.Spec.Containers[0].Ports).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(27017)))

			// Verify owner reference
			Expect(deployment.OwnerReferences).To(HaveLen(1))
			Expect(deployment.OwnerReferences[0].Name).To(Equal(name))
		})

		It("should not create arbiter when disabled", func() {
			// Default: no arbiter spec (nil)
			Expect(reconciler.reconcileArbiter(ctx, cr)).To(Succeed())

			deployment := &appsv1.Deployment{}
			depKey := types.NamespacedName{Name: fmt.Sprintf("%s-arbiter", name), Namespace: namespace}
			err := k8sClient.Get(ctx, depKey, deployment)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should be idempotent for arbiter", func() {
			cr.Spec.Arbiter = &databasev1alpha1.ArbiterSpec{
				Enabled: true,
			}
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			// First reconcile creates the Deployment
			Expect(reconciler.reconcileArbiter(ctx, cr)).To(Succeed())

			deployment := &appsv1.Deployment{}
			depKey := types.NamespacedName{Name: fmt.Sprintf("%s-arbiter", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, depKey, deployment)).To(Succeed())
			originalVersion := deployment.ResourceVersion

			// Second reconcile should not recreate
			Expect(reconciler.reconcileArbiter(ctx, cr)).To(Succeed())
			Expect(k8sClient.Get(ctx, depKey, deployment)).To(Succeed())
			Expect(deployment.ResourceVersion).To(Equal(originalVersion))
		})

		It("should delete arbiter when disabled", func() {
			// First enable arbiter and reconcile to create Deployment
			cr.Spec.Arbiter = &databasev1alpha1.ArbiterSpec{
				Enabled: true,
			}
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			Expect(reconciler.reconcileArbiter(ctx, cr)).To(Succeed())

			// Verify Deployment exists
			deployment := &appsv1.Deployment{}
			depKey := types.NamespacedName{Name: fmt.Sprintf("%s-arbiter", name), Namespace: namespace}
			Expect(k8sClient.Get(ctx, depKey, deployment)).To(Succeed())

			// Now disable arbiter
			cr.Spec.Arbiter.Enabled = false
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			// Reconcile should delete the Deployment
			Expect(reconciler.reconcileArbiter(ctx, cr)).To(Succeed())

			// Verify Deployment is deleted
			err := k8sClient.Get(ctx, depKey, deployment)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	// ============================================================
	// Helper Function Tests
	// ============================================================
	Context("When testing helper functions", func() {
		It("should return correct labels with expected keys and values", func() {
			cr := &databasev1alpha1.MongoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-mongo",
				},
				Spec: databasev1alpha1.MongoClusterSpec{
					Version: "7.0",
				},
			}

			labels := labelsForMongoCluster(cr)

			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", "mongodb"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "my-mongo"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "mongodb-operator"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/part-of", "my-mongo"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/version", "7.0"))
			Expect(labels).To(HaveLen(5))
		})

		It("should generate correct MongoDB image name", func() {
			cr := &databasev1alpha1.MongoCluster{
				Spec: databasev1alpha1.MongoClusterSpec{
					Version: "7.0",
				},
			}

			image := imageForMongoCluster(cr)
			Expect(image).To(Equal("registry.redhat.io/rhel9/mongodb-7"))

			cr.Spec.Version = "8.0"
			image = imageForMongoCluster(cr)
			Expect(image).To(Equal("registry.redhat.io/rhel9/mongodb-8"))
		})
	})
})

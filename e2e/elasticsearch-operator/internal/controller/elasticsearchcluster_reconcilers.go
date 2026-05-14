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
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	searchv1beta1 "github.com/example/elasticsearch-operator/api/v1beta1"
)

func (r *ElasticsearchClusterReconciler) reconcileSecret(ctx context.Context, cr *searchv1beta1.ElasticsearchCluster) error {
	if cr.Spec.Auth != nil && cr.Spec.Auth.ExistingSecret != "" {
		return nil
	}

	name := fmt.Sprintf("%s-auth", cr.Name)
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	password := generatePassword()
	if cr.Spec.Auth != nil && cr.Spec.Auth.AdminPassword != "" {
		password = cr.Spec.Auth.AdminPassword
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: cr.Namespace, Labels: labelsForElasticsearchCluster(cr),
		},
		StringData: map[string]string{
			"ELASTIC_USERNAME": "elastic",
			"ELASTIC_PASSWORD": password,
		},
	}

	if err := controllerutil.SetControllerReference(cr, secret, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, secret); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "SecretFailed", fmt.Sprintf("Failed to create Secret: %v", err))
		return err
	}
	r.Recorder.Event(cr, corev1.EventTypeNormal, "SecretCreated", fmt.Sprintf("Created Secret %s", name))
	return nil
}

func (r *ElasticsearchClusterReconciler) reconcileConfigMap(ctx context.Context, cr *searchv1beta1.ElasticsearchCluster) error {
	name := fmt.Sprintf("%s-config", cr.Name)
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	esYml := fmt.Sprintf(`cluster.name: %s
node.name: ${HOSTNAME}
network.host: 0.0.0.0
http.port: 9200
transport.port: 9300
discovery.seed_hosts:
  - %s-transport
`, cr.Name, cr.Name)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: cr.Namespace, Labels: labelsForElasticsearchCluster(cr),
		},
		Data: map[string]string{"elasticsearch.yml": esYml},
	}

	if err := controllerutil.SetControllerReference(cr, configMap, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, configMap); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "ConfigMapFailed", fmt.Sprintf("Failed to create ConfigMap: %v", err))
		return err
	}
	r.Recorder.Event(cr, corev1.EventTypeNormal, "ConfigMapCreated", fmt.Sprintf("Created ConfigMap %s", name))
	return nil
}

func (r *ElasticsearchClusterReconciler) reconcileHTTPService(ctx context.Context, cr *searchv1beta1.ElasticsearchCluster) error {
	name := fmt.Sprintf("%s-http", cr.Name)
	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: cr.Namespace, Labels: labelsForElasticsearchCluster(cr),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labelsForElasticsearchCluster(cr),
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 9200, TargetPort: intstr.FromInt32(9200), Protocol: corev1.ProtocolTCP},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, svc, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, svc); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "HTTPServiceFailed", fmt.Sprintf("Failed to create HTTP Service: %v", err))
		return err
	}
	r.Recorder.Event(cr, corev1.EventTypeNormal, "HTTPServiceCreated", fmt.Sprintf("Created HTTP Service %s", name))
	return nil
}

func (r *ElasticsearchClusterReconciler) reconcileTransportService(ctx context.Context, cr *searchv1beta1.ElasticsearchCluster) error {
	name := fmt.Sprintf("%s-transport", cr.Name)
	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: cr.Namespace, Labels: labelsForElasticsearchCluster(cr),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  labelsForElasticsearchCluster(cr),
			Ports: []corev1.ServicePort{
				{Name: "transport", Port: 9300, TargetPort: intstr.FromInt32(9300), Protocol: corev1.ProtocolTCP},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, svc, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, svc); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "TransportServiceFailed", fmt.Sprintf("Failed to create transport Service: %v", err))
		return err
	}
	r.Recorder.Event(cr, corev1.EventTypeNormal, "TransportServiceCreated", fmt.Sprintf("Created transport Service %s", name))
	return nil
}

func (r *ElasticsearchClusterReconciler) reconcileStatefulSet(ctx context.Context, cr *searchv1beta1.ElasticsearchCluster) error {
	name := cr.Name
	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		updated := false
		if *existing.Spec.Replicas != cr.Spec.Replicas {
			existing.Spec.Replicas = &cr.Spec.Replicas
			updated = true
		}
		if !reflect.DeepEqual(existing.Spec.Template.Spec.Containers[0].Resources, cr.Spec.Resources) {
			existing.Spec.Template.Spec.Containers[0].Resources = cr.Spec.Resources
			updated = true
		}
		if updated {
			if err := r.Update(ctx, existing); err != nil {
				r.Recorder.Event(cr, corev1.EventTypeWarning, "StatefulSetUpdateFailed", fmt.Sprintf("Failed to update StatefulSet: %v", err))
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "StatefulSetUpdated", fmt.Sprintf("Updated StatefulSet %s", name))
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	replicas := cr.Spec.Replicas
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: cr.Namespace, Labels: labelsForElasticsearchCluster(cr),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: fmt.Sprintf("%s-transport", cr.Name),
			Selector:    &metav1.LabelSelector{MatchLabels: labelsForElasticsearchCluster(cr)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labelsForElasticsearchCluster(cr)},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "elasticsearch",
							Image:   "registry.access.redhat.com/ubi9/ubi-micro:latest",
							Command: []string{"/bin/sleep", "infinity"},
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 9200, Protocol: corev1.ProtocolTCP},
								{Name: "transport", ContainerPort: 9300, Protocol: corev1.ProtocolTCP},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/usr/share/elasticsearch/data"},
								{Name: "config", MountPath: "/usr/share/elasticsearch/config"},
							},
							Resources: cr.Spec.Resources,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: fmt.Sprintf("%s-config", cr.Name)},
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data", Labels: labelsForElasticsearchCluster(cr)},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(cr.Spec.Storage.Size)},
						},
					},
				},
			},
		},
	}

	if cr.Spec.Storage.StorageClassName != nil {
		sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName = cr.Spec.Storage.StorageClassName
	}

	if err := controllerutil.SetControllerReference(cr, sts, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, sts); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "StatefulSetFailed", fmt.Sprintf("Failed to create StatefulSet: %v", err))
		return err
	}
	r.Recorder.Event(cr, corev1.EventTypeNormal, "StatefulSetCreated", fmt.Sprintf("Created StatefulSet %s", name))
	return nil
}

func (r *ElasticsearchClusterReconciler) reconcileMaster(ctx context.Context, cr *searchv1beta1.ElasticsearchCluster) error {
	name := fmt.Sprintf("%s-master", cr.Name)

	if cr.Spec.Master == nil || !cr.Spec.Master.Enabled {
		existing := &appsv1.Deployment{}
		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
		if err == nil {
			if err := r.Delete(ctx, existing); err != nil {
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "MasterDeleted", fmt.Sprintf("Deleted master Deployment %s", name))
		} else if !errors.IsNotFound(err) {
			return err
		}
		clearMasterReadyCondition(cr, "MasterDisabled", "Master nodes are not enabled")
		return nil
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		updated := false
		if *existing.Spec.Replicas != cr.Spec.Master.Replicas {
			existing.Spec.Replicas = &cr.Spec.Master.Replicas
			updated = true
		}
		if !reflect.DeepEqual(existing.Spec.Template.Spec.Containers[0].Resources, cr.Spec.Master.Resources) {
			existing.Spec.Template.Spec.Containers[0].Resources = cr.Spec.Master.Resources
			updated = true
		}
		if updated {
			if err := r.Update(ctx, existing); err != nil {
				r.Recorder.Event(cr, corev1.EventTypeWarning, "MasterUpdateFailed", fmt.Sprintf("Failed to update master: %v", err))
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "MasterUpdated", fmt.Sprintf("Updated master Deployment %s", name))
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	replicas := cr.Spec.Master.Replicas
	masterLabels := map[string]string{
		"app.kubernetes.io/name":       "elasticsearch",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/managed-by": "elasticsearch-operator",
		"app.kubernetes.io/part-of":    cr.Name,
		"app.kubernetes.io/component":  "master",
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: cr.Namespace, Labels: masterLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: masterLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: masterLabels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "master",
							Image:   "registry.access.redhat.com/ubi9/ubi-micro:latest",
							Command: []string{"/bin/sleep", "infinity"},
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 9200, Protocol: corev1.ProtocolTCP},
								{Name: "transport", ContainerPort: 9300, Protocol: corev1.ProtocolTCP},
							},
							Resources: cr.Spec.Master.Resources,
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, deployment, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, deployment); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "MasterFailed", fmt.Sprintf("Failed to create master Deployment: %v", err))
		return err
	}
	r.Recorder.Event(cr, corev1.EventTypeNormal, "MasterCreated", fmt.Sprintf("Created master Deployment %s", name))
	setMasterReadyCondition(cr, "MasterConfigured", "Master Deployment is running")
	return nil
}

func (r *ElasticsearchClusterReconciler) reconcileBackupCronJob(ctx context.Context, cr *searchv1beta1.ElasticsearchCluster) error {
	if cr.Spec.Backup == nil || !cr.Spec.Backup.Enabled {
		clearBackupReadyCondition(cr, "BackupDisabled", "Backup is not enabled")
		return nil
	}

	name := fmt.Sprintf("%s-backup", cr.Name)
	existing := &batchv1.CronJob{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		if existing.Spec.Schedule != cr.Spec.Backup.Schedule {
			existing.Spec.Schedule = cr.Spec.Backup.Schedule
			if err := r.Update(ctx, existing); err != nil {
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "BackupCronJobUpdated", fmt.Sprintf("Updated backup schedule to %s", cr.Spec.Backup.Schedule))
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: cr.Namespace, Labels: labelsForElasticsearchCluster(cr),
		},
		Spec: batchv1.CronJobSpec{
			Schedule: cr.Spec.Backup.Schedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labelsForElasticsearchCluster(cr)},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:    "snapshot",
									Image:   "registry.access.redhat.com/ubi9/ubi-micro:latest",
									Command: []string{"/bin/sleep", "5"},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, cronJob, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, cronJob); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "BackupCronJobFailed", fmt.Sprintf("Failed to create backup CronJob: %v", err))
		return err
	}
	r.Recorder.Event(cr, corev1.EventTypeNormal, "BackupCronJobCreated", fmt.Sprintf("Created backup CronJob %s with schedule %s", name, cr.Spec.Backup.Schedule))
	setBackupReadyCondition(cr, "BackupConfigured", "Backup CronJob is scheduled")
	return nil
}

func (r *ElasticsearchClusterReconciler) reconcileNetworkPolicy(ctx context.Context, cr *searchv1beta1.ElasticsearchCluster) error {
	name := fmt.Sprintf("%s-network-policy", cr.Name)
	existing := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		setNetworkSecuredCondition(cr, "NetworkSecured", "NetworkPolicy is configured")
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	httpPort := intstr.FromInt32(9200)
	transportPort := intstr.FromInt32(9300)
	dnsPort := intstr.FromInt32(53)
	protocolTCP := corev1.ProtocolTCP
	protocolUDP := corev1.ProtocolUDP

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labelsForElasticsearchCluster(cr),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: labelsForElasticsearchCluster(cr),
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Port:     &httpPort,
							Protocol: &protocolTCP,
						},
						{
							Port:     &transportPort,
							Protocol: &protocolTCP,
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Port:     &dnsPort,
							Protocol: &protocolTCP,
						},
						{
							Port:     &dnsPort,
							Protocol: &protocolUDP,
						},
					},
				},
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: labelsForElasticsearchCluster(cr),
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Port:     &httpPort,
							Protocol: &protocolTCP,
						},
						{
							Port:     &transportPort,
							Protocol: &protocolTCP,
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, np, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, np); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "NetworkPolicyFailed",
			fmt.Sprintf("Failed to create NetworkPolicy: %v", err))
		return err
	}

	r.Recorder.Event(cr, corev1.EventTypeNormal, "NetworkPolicyCreated",
		fmt.Sprintf("Created NetworkPolicy %s", name))
	setNetworkSecuredCondition(cr, "NetworkSecured", "NetworkPolicy is configured")
	return nil
}

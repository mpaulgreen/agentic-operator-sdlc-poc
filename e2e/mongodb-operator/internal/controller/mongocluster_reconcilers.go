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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	databasev1alpha1 "github.com/example/mongodb-operator/api/v1alpha1"
)

func (r *MongoClusterReconciler) reconcileAdminSecret(ctx context.Context, cr *databasev1alpha1.MongoCluster) error {
	if cr.Spec.Auth != nil && cr.Spec.Auth.ExistingSecret != "" {
		return nil
	}

	name := fmt.Sprintf("%s-admin", cr.Name)
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
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labelsForMongoCluster(cr),
		},
		StringData: map[string]string{
			"MONGO_INITDB_ROOT_USERNAME": "admin",
			"MONGO_INITDB_ROOT_PASSWORD": password,
		},
	}

	if err := controllerutil.SetControllerReference(cr, secret, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, secret); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "AdminSecretFailed",
			fmt.Sprintf("Failed to create admin Secret: %v", err))
		return err
	}

	r.Recorder.Event(cr, corev1.EventTypeNormal, "AdminSecretCreated",
		fmt.Sprintf("Created admin Secret %s", name))
	return nil
}

func (r *MongoClusterReconciler) reconcileKeyFileSecret(ctx context.Context, cr *databasev1alpha1.MongoCluster) error {
	if cr.Spec.Auth != nil && cr.Spec.Auth.KeyFile != "" {
		return nil
	}

	name := fmt.Sprintf("%s-keyfile", cr.Name)
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labelsForMongoCluster(cr),
		},
		StringData: map[string]string{
			"keyfile": generateKeyFile(),
		},
	}

	if err := controllerutil.SetControllerReference(cr, secret, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, secret); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "KeyFileSecretFailed",
			fmt.Sprintf("Failed to create keyFile Secret: %v", err))
		return err
	}

	r.Recorder.Event(cr, corev1.EventTypeNormal, "KeyFileSecretCreated",
		fmt.Sprintf("Created keyFile Secret %s", name))
	return nil
}

func (r *MongoClusterReconciler) reconcileConfigMap(ctx context.Context, cr *databasev1alpha1.MongoCluster) error {
	name := fmt.Sprintf("%s-config", cr.Name)
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	mongodConf := fmt.Sprintf(`storage:
  dbPath: /var/lib/mongodb/data
net:
  port: 27017
  bindIp: 0.0.0.0
replication:
  replSetName: %s
security:
  keyFile: /etc/mongodb/keyfile/keyfile
`, cr.Name)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labelsForMongoCluster(cr),
		},
		Data: map[string]string{
			"mongod.conf": mongodConf,
		},
	}

	if err := controllerutil.SetControllerReference(cr, configMap, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, configMap); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "ConfigMapFailed",
			fmt.Sprintf("Failed to create ConfigMap: %v", err))
		return err
	}

	r.Recorder.Event(cr, corev1.EventTypeNormal, "ConfigMapCreated",
		fmt.Sprintf("Created ConfigMap %s", name))
	return nil
}

func (r *MongoClusterReconciler) reconcileHeadlessService(ctx context.Context, cr *databasev1alpha1.MongoCluster) error {
	name := fmt.Sprintf("%s-headless", cr.Name)
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
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labelsForMongoCluster(cr),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  labelsForMongoCluster(cr),
			Ports: []corev1.ServicePort{
				{
					Name:       "mongodb",
					Port:       27017,
					TargetPort: intstr.FromInt32(27017),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, svc, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, svc); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "HeadlessServiceFailed",
			fmt.Sprintf("Failed to create headless Service: %v", err))
		return err
	}

	r.Recorder.Event(cr, corev1.EventTypeNormal, "HeadlessServiceCreated",
		fmt.Sprintf("Created headless Service %s", name))
	return nil
}

func (r *MongoClusterReconciler) reconcileClientService(ctx context.Context, cr *databasev1alpha1.MongoCluster) error {
	name := fmt.Sprintf("%s-client", cr.Name)
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
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labelsForMongoCluster(cr),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labelsForMongoCluster(cr),
			Ports: []corev1.ServicePort{
				{
					Name:       "mongodb",
					Port:       27017,
					TargetPort: intstr.FromInt32(27017),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, svc, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, svc); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "ClientServiceFailed",
			fmt.Sprintf("Failed to create client Service: %v", err))
		return err
	}

	r.Recorder.Event(cr, corev1.EventTypeNormal, "ClientServiceCreated",
		fmt.Sprintf("Created client Service %s", name))
	return nil
}

func (r *MongoClusterReconciler) reconcileStatefulSet(ctx context.Context, cr *databasev1alpha1.MongoCluster) error {
	name := cr.Name
	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		updated := false
		if *existing.Spec.Replicas != cr.Spec.Replicas {
			existing.Spec.Replicas = &cr.Spec.Replicas
			updated = true
		}
		desiredImage := imageForMongoCluster(cr)
		if existing.Spec.Template.Spec.Containers[0].Image != desiredImage {
			existing.Spec.Template.Spec.Containers[0].Image = desiredImage
			updated = true
		}
		if !reflect.DeepEqual(existing.Spec.Template.Spec.Containers[0].Resources, cr.Spec.Resources) {
			existing.Spec.Template.Spec.Containers[0].Resources = cr.Spec.Resources
			updated = true
		}
		if updated {
			if err := r.Update(ctx, existing); err != nil {
				r.Recorder.Event(cr, corev1.EventTypeWarning, "StatefulSetUpdateFailed",
					fmt.Sprintf("Failed to update StatefulSet: %v", err))
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "StatefulSetUpdated",
				fmt.Sprintf("Updated StatefulSet %s", name))
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	replicas := cr.Spec.Replicas
	keyFileSecretName := fmt.Sprintf("%s-keyfile", cr.Name)
	if cr.Spec.Auth != nil && cr.Spec.Auth.KeyFile != "" {
		keyFileSecretName = cr.Spec.Auth.KeyFile
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labelsForMongoCluster(cr),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: fmt.Sprintf("%s-headless", cr.Name),
			Selector: &metav1.LabelSelector{
				MatchLabels: labelsForMongoCluster(cr),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labelsForMongoCluster(cr),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "mongodb",
							Image: imageForMongoCluster(cr),
							Ports: []corev1.ContainerPort{
								{
									Name:          "mongodb",
									ContainerPort: 27017,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Command: []string{"/bin/sleep", "infinity"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/mongodb/data",
								},
								{
									Name:      "config",
									MountPath: "/etc/mongodb",
								},
								{
									Name:      "keyfile",
									MountPath: "/etc/mongodb/keyfile",
									ReadOnly:  true,
								},
							},
							Resources: cr.Spec.Resources,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config", cr.Name),
									},
								},
							},
						},
						{
							Name: "keyfile",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  keyFileSecretName,
									DefaultMode: int32Ptr(0400),
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "data",
						Labels: labelsForMongoCluster(cr),
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(cr.Spec.Storage.Size),
							},
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
		r.Recorder.Event(cr, corev1.EventTypeWarning, "StatefulSetFailed",
			fmt.Sprintf("Failed to create StatefulSet: %v", err))
		return err
	}

	r.Recorder.Event(cr, corev1.EventTypeNormal, "StatefulSetCreated",
		fmt.Sprintf("Created StatefulSet %s", name))
	return nil
}

func (r *MongoClusterReconciler) reconcileArbiter(ctx context.Context, cr *databasev1alpha1.MongoCluster) error {
	name := fmt.Sprintf("%s-arbiter", cr.Name)

	if cr.Spec.Arbiter == nil || !cr.Spec.Arbiter.Enabled {
		existing := &appsv1.Deployment{}
		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
		if err == nil {
			if err := r.Delete(ctx, existing); err != nil {
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "ArbiterDeleted",
				fmt.Sprintf("Deleted arbiter Deployment %s", name))
		} else if !errors.IsNotFound(err) {
			return err
		}
		clearArbiterReadyCondition(cr, "ArbiterDisabled", "Arbiter is not enabled")
		return nil
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		updated := false
		if !reflect.DeepEqual(existing.Spec.Template.Spec.Containers[0].Resources, cr.Spec.Arbiter.Resources) {
			existing.Spec.Template.Spec.Containers[0].Resources = cr.Spec.Arbiter.Resources
			updated = true
		}
		if updated {
			if err := r.Update(ctx, existing); err != nil {
				r.Recorder.Event(cr, corev1.EventTypeWarning, "ArbiterUpdateFailed",
					fmt.Sprintf("Failed to update arbiter Deployment: %v", err))
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "ArbiterUpdated",
				fmt.Sprintf("Updated arbiter Deployment %s", name))
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	replicas := int32(1)
	arbiterLabels := map[string]string{
		"app.kubernetes.io/name":       "mongodb",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/managed-by": "mongodb-operator",
		"app.kubernetes.io/part-of":    cr.Name,
		"app.kubernetes.io/component":  "arbiter",
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    arbiterLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: arbiterLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: arbiterLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "arbiter",
							Image:   imageForMongoCluster(cr),
							Command: []string{"/bin/sleep", "infinity"},
							Ports: []corev1.ContainerPort{
								{
									Name:          "mongodb",
									ContainerPort: 27017,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: cr.Spec.Arbiter.Resources,
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
		r.Recorder.Event(cr, corev1.EventTypeWarning, "ArbiterFailed",
			fmt.Sprintf("Failed to create arbiter Deployment: %v", err))
		return err
	}

	r.Recorder.Event(cr, corev1.EventTypeNormal, "ArbiterCreated",
		fmt.Sprintf("Created arbiter Deployment %s", name))
	setArbiterReadyCondition(cr, "ArbiterConfigured", "Arbiter Deployment is running")
	return nil
}

func (r *MongoClusterReconciler) reconcileBackupJob(ctx context.Context, cr *databasev1alpha1.MongoCluster) error {
	if cr.Spec.Backup == nil || !cr.Spec.Backup.Enabled {
		clearBackupReadyCondition(cr, "BackupDisabled", "Backup is not enabled")
		return nil
	}

	labelSelector := labels.SelectorFromSet(map[string]string{
		"app.kubernetes.io/instance":  cr.Name,
		"app.kubernetes.io/component": "backup",
	})

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList, &client.ListOptions{
		Namespace:     cr.Namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return err
	}

	for i := range jobList.Items {
		job := &jobList.Items[i]
		if job.Status.Active > 0 {
			return nil
		}
		if job.Status.Succeeded > 0 {
			return nil
		}
		if job.Status.Succeeded == 0 && job.Status.Failed == 0 {
			return nil
		}
	}

	timestamp := time.Now().Format("20060102-150405")
	jobName := fmt.Sprintf("%s-backup-%s", cr.Name, timestamp)

	backoff := int32(3)
	backupLabels := map[string]string{
		"app.kubernetes.io/name":       "mongodb",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/managed-by": "mongodb-operator",
		"app.kubernetes.io/component":  "backup",
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cr.Namespace,
			Labels:    backupLabels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: backupLabels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:    "mongodump",
							Image:   imageForMongoCluster(cr),
							Command: []string{"/bin/sleep", "5"},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, job, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, job); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "BackupJobFailed",
			fmt.Sprintf("Failed to create backup Job: %v", err))
		return err
	}

	r.Recorder.Event(cr, corev1.EventTypeNormal, "BackupJobCreated",
		fmt.Sprintf("Created backup Job %s", jobName))
	return nil
}

func int32Ptr(i int32) *int32 { return &i }

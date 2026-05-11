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

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	databasev1alpha1 "github.com/example/postgres-operator/api/v1alpha1"
)

// reconcileSecret ensures the PostgreSQL credentials Secret exists.
// Secret name: <name>-credentials
// Keys: POSTGRESQL_PASSWORD, POSTGRESQL_USER, POSTGRESQL_DATABASE (RHEL9 format)
func (r *PostgresClusterReconciler) reconcileSecret(ctx context.Context, cr *databasev1alpha1.PostgresCluster) error {
	name := fmt.Sprintf("%s-credentials", cr.Name)

	// 1. CHECK if exists
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		return nil // EXISTS -- idempotent, nothing to do
	}
	if !errors.IsNotFound(err) {
		return err // ACTUAL ERROR
	}

	// 2. BUILD desired state
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labelsForPostgresCluster(cr),
		},
		StringData: map[string]string{
			"POSTGRESQL_PASSWORD": generatePassword(),
			"POSTGRESQL_USER":     "postgres",
			"POSTGRESQL_DATABASE": "postgresdb",
		},
	}

	// 3. SET OWNER REFERENCE (for garbage collection)
	if err := controllerutil.SetControllerReference(cr, secret, r.Scheme); err != nil {
		return err
	}

	// 4. CREATE
	if err := r.Create(ctx, secret); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "SecretFailed", err.Error())
		return err
	}

	// 5. RECORD SUCCESS EVENT
	r.Recorder.Event(cr, corev1.EventTypeNormal, "SecretCreated", name)
	return nil
}

// reconcileConfigMap ensures the PostgreSQL configuration ConfigMap exists.
// ConfigMap name: <name>-config
// Key: postgresql.conf with shared_buffers, max_connections, wal_level settings
func (r *PostgresClusterReconciler) reconcileConfigMap(ctx context.Context, cr *databasev1alpha1.PostgresCluster) error {
	name := fmt.Sprintf("%s-config", cr.Name)

	// 1. CHECK if exists
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		return nil // EXISTS -- idempotent, nothing to do
	}
	if !errors.IsNotFound(err) {
		return err // ACTUAL ERROR
	}

	// 2. BUILD desired state
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labelsForPostgresCluster(cr),
		},
		Data: map[string]string{
			"postgresql.conf": "shared_buffers = 256MB\nmax_connections = 100\nwal_level = replica\n",
		},
	}

	// 3. SET OWNER REFERENCE
	if err := controllerutil.SetControllerReference(cr, configMap, r.Scheme); err != nil {
		return err
	}

	// 4. CREATE
	if err := r.Create(ctx, configMap); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "ConfigMapFailed", err.Error())
		return err
	}

	// 5. RECORD SUCCESS EVENT
	r.Recorder.Event(cr, corev1.EventTypeNormal, "ConfigMapCreated", name)
	return nil
}

// reconcileService ensures the headless Service exists for StatefulSet DNS.
// Service name: <name>-headless, ClusterIP None, port 5432
func (r *PostgresClusterReconciler) reconcileService(ctx context.Context, cr *databasev1alpha1.PostgresCluster) error {
	name := fmt.Sprintf("%s-headless", cr.Name)

	// 1. CHECK if exists
	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		return nil // EXISTS -- idempotent, nothing to do
	}
	if !errors.IsNotFound(err) {
		return err // ACTUAL ERROR
	}

	// 2. BUILD desired state
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labelsForPostgresCluster(cr),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  labelsForPostgresCluster(cr),
			Ports: []corev1.ServicePort{
				{
					Name:     "postgresql",
					Port:     5432,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	// 3. SET OWNER REFERENCE
	if err := controllerutil.SetControllerReference(cr, service, r.Scheme); err != nil {
		return err
	}

	// 4. CREATE
	if err := r.Create(ctx, service); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "ServiceFailed", err.Error())
		return err
	}

	// 5. RECORD SUCCESS EVENT
	r.Recorder.Event(cr, corev1.EventTypeNormal, "ServiceCreated", name)
	return nil
}

// reconcileStatefulSet ensures the PostgreSQL StatefulSet exists and is up to date.
// Image: registry.redhat.io/rhel9/postgresql-<version> (OpenShift-compatible)
// Data dir: /var/lib/pgsql/data, Config dir: /opt/app-root/src/postgresql-cfg
func (r *PostgresClusterReconciler) reconcileStatefulSet(ctx context.Context, cr *databasev1alpha1.PostgresCluster) error {
	name := cr.Name

	// 1. CHECK if exists
	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		// Check-update: reconcile replicas if changed
		if *existing.Spec.Replicas != cr.Spec.Replicas {
			existing.Spec.Replicas = &cr.Spec.Replicas
			if err := r.Update(ctx, existing); err != nil {
				r.Recorder.Event(cr, corev1.EventTypeWarning, "StatefulSetUpdateFailed", err.Error())
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "StatefulSetUpdated",
				fmt.Sprintf("Updated replicas to %d", cr.Spec.Replicas))
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err // ACTUAL ERROR
	}

	// 2. BUILD desired state
	labels := labelsForPostgresCluster(cr)
	replicas := cr.Spec.Replicas
	image := fmt.Sprintf("registry.redhat.io/rhel9/postgresql-%s", cr.Spec.Version)
	credentialSecretName := fmt.Sprintf("%s-credentials", cr.Name)
	configMapName := fmt.Sprintf("%s-config", cr.Name)
	headlessServiceName := fmt.Sprintf("%s-headless", cr.Name)

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: headlessServiceName,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "postgresql",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "postgresql",
									ContainerPort: 5432,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: credentialSecretName,
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/pgsql/data",
								},
								{
									Name:      "config",
									MountPath: "/opt/app-root/src/postgresql-cfg",
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
										Name: configMapName,
									},
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
						Labels: labels,
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
						StorageClassName: cr.Spec.Storage.StorageClassName,
					},
				},
			},
		},
	}

	// 3. SET OWNER REFERENCE
	if err := controllerutil.SetControllerReference(cr, statefulSet, r.Scheme); err != nil {
		return err
	}

	// 4. CREATE
	if err := r.Create(ctx, statefulSet); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "StatefulSetFailed", err.Error())
		return err
	}

	// 5. RECORD SUCCESS EVENT
	r.Recorder.Event(cr, corev1.EventTypeNormal, "StatefulSetCreated", name)
	return nil
}

// reconcileCronJob ensures the backup CronJob exists if backup is enabled.
// CronJob name: <name>-backup
// Only created when spec.backup is non-nil and spec.backup.enabled is true.
func (r *PostgresClusterReconciler) reconcileCronJob(ctx context.Context, cr *databasev1alpha1.PostgresCluster) error {
	name := fmt.Sprintf("%s-backup", cr.Name)

	// If backup is not enabled, ensure the CronJob does not exist
	if cr.Spec.Backup == nil || !cr.Spec.Backup.Enabled {
		existing := &batchv1.CronJob{}
		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
		if err == nil {
			// Backup was disabled, delete the CronJob
			if err := r.Delete(ctx, existing); err != nil {
				r.Recorder.Event(cr, corev1.EventTypeWarning, "CronJobDeleteFailed", err.Error())
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "CronJobDeleted", name)
			clearBackupReadyCondition(cr, "BackupDisabled", "Backup is not enabled")
		}
		return nil
	}

	// 1. CHECK if exists
	existing := &batchv1.CronJob{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		return nil // EXISTS -- idempotent, nothing to do
	}
	if !errors.IsNotFound(err) {
		return err // ACTUAL ERROR
	}

	// 2. BUILD desired state
	labels := labelsForPostgresCluster(cr)
	credentialSecretName := fmt.Sprintf("%s-credentials", cr.Name)
	image := fmt.Sprintf("registry.redhat.io/rhel9/postgresql-%s", cr.Spec.Version)

	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: cr.Spec.Backup.Schedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:    "backup",
									Image:   image,
									Command: []string{"/bin/bash", "-c", "pg_dumpall -h $POSTGRESQL_HOST -U $POSTGRESQL_USER > /backup/dump-$(date +%Y%m%d%H%M%S).sql"},
									EnvFrom: []corev1.EnvFromSource{
										{
											SecretRef: &corev1.SecretEnvSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: credentialSecretName,
												},
											},
										},
									},
									Env: []corev1.EnvVar{
										{
											Name:  "POSTGRESQL_HOST",
											Value: fmt.Sprintf("%s-headless.%s.svc.cluster.local", cr.Name, cr.Namespace),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// 3. SET OWNER REFERENCE
	if err := controllerutil.SetControllerReference(cr, cronJob, r.Scheme); err != nil {
		return err
	}

	// 4. CREATE
	if err := r.Create(ctx, cronJob); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "CronJobFailed", err.Error())
		return err
	}

	// 5. RECORD SUCCESS EVENT
	r.Recorder.Event(cr, corev1.EventTypeNormal, "CronJobCreated", name)
	setBackupReadyCondition(cr, "BackupConfigured", fmt.Sprintf("Backup CronJob created with schedule %s", cr.Spec.Backup.Schedule))
	return nil
}

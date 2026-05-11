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
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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
		// Check-update: reconcile all mutable spec fields
		updated := false
		if *existing.Spec.Replicas != cr.Spec.Replicas {
			existing.Spec.Replicas = &cr.Spec.Replicas
			updated = true
		}
		desiredAffinity := podAffinityForPostgresCluster(cr)
		if !reflect.DeepEqual(existing.Spec.Template.Spec.Affinity, desiredAffinity) {
			existing.Spec.Template.Spec.Affinity = desiredAffinity
			updated = true
		}
		desiredImage := fmt.Sprintf("registry.redhat.io/rhel9/postgresql-%s", cr.Spec.Version)
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
				r.Recorder.Event(cr, corev1.EventTypeWarning, "StatefulSetUpdateFailed", err.Error())
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "StatefulSetUpdated",
				fmt.Sprintf("Updated StatefulSet %s", cr.Name))
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
					Affinity: podAffinityForPostgresCluster(cr),
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

// reconcilePodDisruptionBudget ensures the PDB exists when HA is configured.
// PDB name: <name>-pdb
// Only created when spec.ha is non-nil.
func (r *PostgresClusterReconciler) reconcilePodDisruptionBudget(ctx context.Context, cr *databasev1alpha1.PostgresCluster) error {
	name := fmt.Sprintf("%s-pdb", cr.Name)

	if cr.Spec.HA == nil {
		existing := &policyv1.PodDisruptionBudget{}
		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
		if err == nil {
			if err := r.Delete(ctx, existing); err != nil {
				r.Recorder.Event(cr, corev1.EventTypeWarning, "PDBDeleteFailed", err.Error())
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "PDBDeleted", name)
			clearHAReadyCondition(cr, "HANotConfigured", "High availability is not configured")
		}
		return nil
	}

	// 1. CHECK if exists
	existing := &policyv1.PodDisruptionBudget{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
	if err == nil {
		// Check-update: reconcile PDB spec if changed
		updated := false
		desiredMinAvail, desiredMaxUnavail := computePDBValues(cr)
		if desiredMinAvail != nil {
			val := intstr.FromInt32(*desiredMinAvail)
			if existing.Spec.MinAvailable == nil || *existing.Spec.MinAvailable != val {
				existing.Spec.MinAvailable = &val
				existing.Spec.MaxUnavailable = nil
				updated = true
			}
		} else if desiredMaxUnavail != nil {
			val := intstr.FromInt32(*desiredMaxUnavail)
			if existing.Spec.MaxUnavailable == nil || *existing.Spec.MaxUnavailable != val {
				existing.Spec.MaxUnavailable = &val
				existing.Spec.MinAvailable = nil
				updated = true
			}
		}
		if updated {
			if err := r.Update(ctx, existing); err != nil {
				r.Recorder.Event(cr, corev1.EventTypeWarning, "PDBUpdateFailed", err.Error())
				return err
			}
			r.Recorder.Event(cr, corev1.EventTypeNormal, "PDBUpdated", name)
		}
		setHAReadyCondition(cr, "HAConfigured", "PodDisruptionBudget is configured")
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	// 2. BUILD desired state
	labels := labelsForPostgresCluster(cr)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}

	minAvail, maxUnavail := computePDBValues(cr)
	if minAvail != nil {
		val := intstr.FromInt32(*minAvail)
		pdb.Spec.MinAvailable = &val
	} else if maxUnavail != nil {
		val := intstr.FromInt32(*maxUnavail)
		pdb.Spec.MaxUnavailable = &val
	}

	// 3. SET OWNER REFERENCE
	if err := controllerutil.SetControllerReference(cr, pdb, r.Scheme); err != nil {
		return err
	}

	// 4. CREATE
	if err := r.Create(ctx, pdb); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "PDBFailed", err.Error())
		return err
	}

	// 5. RECORD SUCCESS EVENT
	r.Recorder.Event(cr, corev1.EventTypeNormal, "PDBCreated", name)
	setHAReadyCondition(cr, "HAConfigured", "PodDisruptionBudget is configured")
	return nil
}

func computePDBValues(cr *databasev1alpha1.PostgresCluster) (minAvailable *int32, maxUnavailable *int32) {
	if cr.Spec.HA.MinAvailable != nil {
		return cr.Spec.HA.MinAvailable, nil
	}
	if cr.Spec.HA.MaxUnavailable != nil {
		return nil, cr.Spec.HA.MaxUnavailable
	}
	defaultMin := cr.Spec.Replicas - 1
	if defaultMin < 1 {
		defaultMin = 1
	}
	return &defaultMin, nil
}

// podAffinityForPostgresCluster returns pod anti-affinity based on HA settings. Returns nil when HA is nil.
func podAffinityForPostgresCluster(cr *databasev1alpha1.PostgresCluster) *corev1.Affinity {
	if cr.Spec.HA == nil {
		return nil
	}

	labelSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/instance": cr.Name,
		},
	}

	if cr.Spec.HA.AntiAffinityMode == "required" {
		return &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{
						LabelSelector: labelSelector,
						TopologyKey:   "kubernetes.io/hostname",
					},
				},
			},
		}
	}

	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: labelSelector,
						TopologyKey:   "kubernetes.io/hostname",
					},
				},
			},
		},
	}
}

func (r *PostgresClusterReconciler) reconcileNetworkPolicy(ctx context.Context, cr *databasev1alpha1.PostgresCluster) error {
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

	labels := labelsForPostgresCluster(cr)
	protocolTCP := corev1.ProtocolTCP
	protocolUDP := corev1.ProtocolUDP
	port5432 := intstr.FromInt32(5432)
	port53 := intstr.FromInt32(53)

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: labels,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{}},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &protocolTCP, Port: &port5432},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &protocolTCP, Port: &port53},
						{Protocol: &protocolUDP, Port: &port53},
					},
				},
				{
					To: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{MatchLabels: labels}},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &protocolTCP, Port: &port5432},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, networkPolicy, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, networkPolicy); err != nil {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "NetworkPolicyFailed", err.Error())
		return err
	}

	r.Recorder.Event(cr, corev1.EventTypeNormal, "NetworkPolicyCreated", name)
	setNetworkSecuredCondition(cr, "NetworkSecured", "NetworkPolicy is configured")
	return nil
}

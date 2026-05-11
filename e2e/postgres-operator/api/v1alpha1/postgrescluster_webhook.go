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

package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var postgresclusterlog = logf.Log.WithName("postgrescluster-resource")

func (r *PostgresCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-database-postgres-example-com-v1alpha1-postgrescluster,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.postgres.example.com,resources=postgresclusters,verbs=create;update,versions=v1alpha1,name=mpostgrescluster.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &PostgresCluster{}

func (r *PostgresCluster) Default() {
	postgresclusterlog.Info("default", "name", r.Name)

	if r.Spec.Replicas == 0 {
		r.Spec.Replicas = 3
	}
	if r.Spec.Version == "" {
		r.Spec.Version = "16"
	}
	if r.Spec.HA != nil {
		if r.Spec.HA.AntiAffinityMode == "" {
			r.Spec.HA.AntiAffinityMode = "preferred"
		}
		if r.Spec.HA.MinAvailable == nil && r.Spec.HA.MaxUnavailable == nil {
			defaultMin := r.Spec.Replicas - 1
			if defaultMin < 1 {
				defaultMin = 1
			}
			r.Spec.HA.MinAvailable = &defaultMin
		}
	}
	if r.Spec.Backup != nil && r.Spec.Backup.RetentionDays == 0 {
		r.Spec.Backup.RetentionDays = 7
	}
}

//+kubebuilder:webhook:path=/validate-database-postgres-example-com-v1alpha1-postgrescluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.postgres.example.com,resources=postgresclusters,verbs=create;update,versions=v1alpha1,name=vpostgrescluster.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &PostgresCluster{}

func (r *PostgresCluster) ValidateCreate() (admission.Warnings, error) {
	postgresclusterlog.Info("validate create", "name", r.Name)
	return nil, r.validatePostgresCluster()
}

func (r *PostgresCluster) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	postgresclusterlog.Info("validate update", "name", r.Name)

	oldCluster, ok := old.(*PostgresCluster)
	if ok {
		oldSize, err := resource.ParseQuantity(oldCluster.Spec.Storage.Size)
		if err != nil {
			return nil, fmt.Errorf("invalid old storage size %q: %v", oldCluster.Spec.Storage.Size, err)
		}
		newSize, err := resource.ParseQuantity(r.Spec.Storage.Size)
		if err != nil {
			return nil, fmt.Errorf("invalid new storage size %q: %v", r.Spec.Storage.Size, err)
		}
		if newSize.Cmp(oldSize) < 0 {
			return nil, fmt.Errorf("storage size cannot be reduced from %s to %s", oldCluster.Spec.Storage.Size, r.Spec.Storage.Size)
		}
	}

	return nil, r.validatePostgresCluster()
}

func (r *PostgresCluster) ValidateDelete() (admission.Warnings, error) {
	postgresclusterlog.Info("validate delete", "name", r.Name)
	return nil, nil
}

func (r *PostgresCluster) validatePostgresCluster() error {
	if r.Spec.HA != nil {
		if r.Spec.HA.MinAvailable != nil && *r.Spec.HA.MinAvailable >= r.Spec.Replicas {
			return fmt.Errorf("ha.minAvailable (%d) must be less than replicas (%d)", *r.Spec.HA.MinAvailable, r.Spec.Replicas)
		}
		if r.Spec.HA.MinAvailable != nil && r.Spec.HA.MaxUnavailable != nil {
			return fmt.Errorf("ha.minAvailable and ha.maxUnavailable are mutually exclusive")
		}
	}
	if r.Spec.Backup != nil && r.Spec.Backup.Enabled && r.Spec.Backup.Schedule == "" {
		return fmt.Errorf("backup.schedule is required when backup.enabled is true")
	}
	return nil
}

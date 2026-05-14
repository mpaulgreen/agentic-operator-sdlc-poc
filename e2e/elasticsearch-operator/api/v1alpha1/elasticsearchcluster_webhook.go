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

var elasticsearchclusterlog = logf.Log.WithName("elasticsearchcluster-resource")

func (r *ElasticsearchCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-search-elasticsearch-example-com-v1alpha1-elasticsearchcluster,mutating=true,failurePolicy=fail,sideEffects=None,groups=search.elasticsearch.example.com,resources=elasticsearchclusters,verbs=create;update,versions=v1alpha1,name=melasticsearchcluster.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &ElasticsearchCluster{}

func (r *ElasticsearchCluster) Default() {
	elasticsearchclusterlog.Info("default", "name", r.Name)

	if r.Spec.Replicas == 0 {
		r.Spec.Replicas = 3
	}
	if r.Spec.Version == "" {
		r.Spec.Version = "8.14"
	}
	if r.Spec.Backup != nil && r.Spec.Backup.Enabled && r.Spec.Backup.RetentionDays == 0 {
		r.Spec.Backup.RetentionDays = 7
	}
	if r.Spec.Master != nil && r.Spec.Master.Enabled && r.Spec.Master.Replicas == 0 {
		r.Spec.Master.Replicas = 3
	}
}

//+kubebuilder:webhook:path=/validate-search-elasticsearch-example-com-v1alpha1-elasticsearchcluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=search.elasticsearch.example.com,resources=elasticsearchclusters,verbs=create;update,versions=v1alpha1,name=velasticsearchcluster.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &ElasticsearchCluster{}

func (r *ElasticsearchCluster) ValidateCreate() (admission.Warnings, error) {
	elasticsearchclusterlog.Info("validate create", "name", r.Name)
	return nil, r.validateElasticsearchCluster()
}

func (r *ElasticsearchCluster) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	elasticsearchclusterlog.Info("validate update", "name", r.Name)

	oldCluster, ok := old.(*ElasticsearchCluster)
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

	return nil, r.validateElasticsearchCluster()
}

func (r *ElasticsearchCluster) ValidateDelete() (admission.Warnings, error) {
	elasticsearchclusterlog.Info("validate delete", "name", r.Name)
	return nil, nil
}

func (r *ElasticsearchCluster) validateElasticsearchCluster() error {
	if r.Spec.Replicas < 1 {
		return fmt.Errorf("replicas must be at least 1, got %d", r.Spec.Replicas)
	}
	if r.Spec.Master != nil && r.Spec.Master.Enabled && r.Spec.Master.Replicas%2 == 0 {
		return fmt.Errorf("master.replicas must be odd for quorum, got %d", r.Spec.Master.Replicas)
	}
	if r.Spec.Auth != nil && r.Spec.Auth.AdminPassword != "" && r.Spec.Auth.ExistingSecret != "" {
		return fmt.Errorf("auth.adminPassword and auth.existingSecret are mutually exclusive")
	}
	if r.Spec.Backup != nil && r.Spec.Backup.Enabled && r.Spec.Backup.Schedule == "" {
		return fmt.Errorf("backup.schedule is required when backup.enabled is true")
	}
	return nil
}

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
	"k8s.io/apimachinery/pkg/types"

	searchv1beta1 "github.com/example/elasticsearch-operator/api/v1beta1"
)

func (r *ElasticsearchClusterReconciler) updateStatus(ctx context.Context, cr *searchv1beta1.ElasticsearchCluster) error {
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, sts); err != nil {
		return err
	}

	cr.Status.ReadyReplicas = sts.Status.ReadyReplicas
	cr.Status.CurrentVersion = cr.Spec.Version
	cr.Status.HttpEndpoint = fmt.Sprintf("%s-http.%s.svc.cluster.local:9200", cr.Name, cr.Namespace)

	if sts.Status.ReadyReplicas == *sts.Spec.Replicas {
		cr.Status.Phase = "Running"
		setAvailableCondition(cr, "AllReplicasReady", "All Elasticsearch nodes are ready")
		clearProgressingCondition(cr, "RolloutComplete", "Rollout completed")
		clearDegradedCondition(cr, "AllHealthy", "No issues detected")
	} else if sts.Status.ReadyReplicas > 0 {
		cr.Status.Phase = "Degraded"
		setUnavailableCondition(cr, "PartiallyReady",
			fmt.Sprintf("%d/%d replicas ready", sts.Status.ReadyReplicas, *sts.Spec.Replicas))
		setDegradedCondition(cr, "PartiallyReady", "Not all replicas are ready")
	} else {
		cr.Status.Phase = "Initializing"
		setUnavailableCondition(cr, "NotReady", "No replicas ready yet")
		setProgressingCondition(cr, "Initializing", "Waiting for replicas to start")
	}

	cr.Status.ILMEnabled = cr.Spec.ILM != nil && cr.Spec.ILM.Enabled

	if cr.Spec.Backup != nil && cr.Spec.Backup.Enabled {
		setBackupReadyCondition(cr, "BackupConfigured", "Backup CronJob is scheduled")
	} else {
		clearBackupReadyCondition(cr, "BackupDisabled", "Backup is not enabled")
	}

	return r.Status().Update(ctx, cr)
}

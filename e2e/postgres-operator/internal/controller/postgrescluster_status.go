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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	databasev1alpha1 "github.com/example/postgres-operator/api/v1alpha1"
)

// updateStatus fetches the StatefulSet and updates the PostgresCluster status accordingly.
func (r *PostgresClusterReconciler) updateStatus(ctx context.Context, cr *databasev1alpha1.PostgresCluster) error {
	// Fetch the StatefulSet to read its status
	sts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, sts)
	if err != nil {
		if errors.IsNotFound(err) {
			// StatefulSet not yet created, set Initializing
			cr.Status.Phase = "Initializing"
			cr.Status.ReadyReplicas = 0
			setProgressingCondition(cr, "StatefulSetPending", "Waiting for StatefulSet to be created")
			setUnavailableCondition(cr, "StatefulSetPending", "StatefulSet does not exist yet")
			return r.Status().Update(ctx, cr)
		}
		return err
	}

	// Update status from StatefulSet
	cr.Status.ReadyReplicas = sts.Status.ReadyReplicas
	cr.Status.CurrentVersion = cr.Spec.Version
	cr.Status.Endpoint = fmt.Sprintf("%s-headless.%s.svc.cluster.local:5432", cr.Name, cr.Namespace)

	// Determine phase and conditions based on replica counts
	r.updatePhase(cr, sts)

	return r.Status().Update(ctx, cr)
}

// updatePhase sets the phase and conditions based on StatefulSet readiness.
func (r *PostgresClusterReconciler) updatePhase(cr *databasev1alpha1.PostgresCluster, sts *appsv1.StatefulSet) {
	desired := cr.Spec.Replicas
	ready := sts.Status.ReadyReplicas

	switch {
	case ready == 0:
		// No pods ready yet
		cr.Status.Phase = "Initializing"
		setProgressingCondition(cr, "PodsStarting", "Waiting for pods to become ready")
		setUnavailableCondition(cr, "NoPodsReady", "No PostgreSQL pods are ready")
		clearDegradedCondition(cr, "Initializing", "Cluster is initializing")

	case ready < desired:
		// Some pods ready but not all
		cr.Status.Phase = "Degraded"
		setProgressingCondition(cr, "ScalingUp",
			fmt.Sprintf("Ready %d/%d replicas", ready, desired))
		setAvailableCondition(cr, "PartiallyAvailable",
			fmt.Sprintf("PostgresCluster has %d/%d ready replicas", ready, desired))
		setDegradedCondition(cr, "InsufficientReplicas",
			fmt.Sprintf("Only %d of %d replicas are ready", ready, desired))

	case ready >= desired:
		// All pods ready
		cr.Status.Phase = "Running"
		clearProgressingCondition(cr, "AllReplicasReady", "All replicas are ready")
		setAvailableCondition(cr, "AllReplicasReady",
			fmt.Sprintf("All %d replicas are ready", desired))
		clearDegradedCondition(cr, "FullyAvailable", "All replicas are running")
	}
}

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	databasev1alpha1 "github.com/example/postgres-operator/api/v1alpha1"
)

// Condition type constants for PostgresCluster.
const (
	// ConditionAvailable indicates the PostgresCluster has minimum availability.
	ConditionAvailable = "Available"

	// ConditionProgressing indicates the PostgresCluster is being rolled out.
	ConditionProgressing = "Progressing"

	// ConditionDegraded indicates the PostgresCluster has reduced capacity or errors.
	ConditionDegraded = "Degraded"

	// ConditionBackupReady indicates the backup CronJob is configured and active.
	ConditionBackupReady = "BackupReady"
)

// setCondition adds or updates a condition on the PostgresCluster status.
func setCondition(cr *databasev1alpha1.PostgresCluster, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()

	// Search for existing condition
	for i, c := range cr.Status.Conditions {
		if c.Type == conditionType {
			// Only update LastTransitionTime if status actually changed
			if c.Status != status {
				cr.Status.Conditions[i].LastTransitionTime = now
			}
			cr.Status.Conditions[i].Status = status
			cr.Status.Conditions[i].Reason = reason
			cr.Status.Conditions[i].Message = message
			cr.Status.Conditions[i].ObservedGeneration = cr.Generation
			return
		}
	}

	// Condition not found, add new one
	cr.Status.Conditions = append(cr.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cr.Generation,
	})
}

// setAvailableCondition sets the Available condition to True.
func setAvailableCondition(cr *databasev1alpha1.PostgresCluster, reason, message string) {
	setCondition(cr, ConditionAvailable, metav1.ConditionTrue, reason, message)
}

// setUnavailableCondition sets the Available condition to False.
func setUnavailableCondition(cr *databasev1alpha1.PostgresCluster, reason, message string) {
	setCondition(cr, ConditionAvailable, metav1.ConditionFalse, reason, message)
}

// setProgressingCondition sets the Progressing condition to True.
func setProgressingCondition(cr *databasev1alpha1.PostgresCluster, reason, message string) {
	setCondition(cr, ConditionProgressing, metav1.ConditionTrue, reason, message)
}

// clearProgressingCondition sets the Progressing condition to False.
func clearProgressingCondition(cr *databasev1alpha1.PostgresCluster, reason, message string) {
	setCondition(cr, ConditionProgressing, metav1.ConditionFalse, reason, message)
}

// setDegradedCondition sets the Degraded condition to True.
func setDegradedCondition(cr *databasev1alpha1.PostgresCluster, reason, message string) {
	setCondition(cr, ConditionDegraded, metav1.ConditionTrue, reason, message)
}

// clearDegradedCondition sets the Degraded condition to False.
func clearDegradedCondition(cr *databasev1alpha1.PostgresCluster, reason, message string) {
	setCondition(cr, ConditionDegraded, metav1.ConditionFalse, reason, message)
}

// setBackupReadyCondition sets the BackupReady condition to True.
func setBackupReadyCondition(cr *databasev1alpha1.PostgresCluster, reason, message string) {
	setCondition(cr, ConditionBackupReady, metav1.ConditionTrue, reason, message)
}

// clearBackupReadyCondition sets the BackupReady condition to False.
func clearBackupReadyCondition(cr *databasev1alpha1.PostgresCluster, reason, message string) {
	setCondition(cr, ConditionBackupReady, metav1.ConditionFalse, reason, message)
}

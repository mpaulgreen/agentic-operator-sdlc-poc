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

	databasev1alpha1 "github.com/example/mongodb-operator/api/v1alpha1"
)

const (
	ConditionAvailable   = "Available"
	ConditionProgressing = "Progressing"
	ConditionDegraded    = "Degraded"
	ConditionBackupReady  = "BackupReady"
	ConditionArbiterReady = "ArbiterReady"
)

func setCondition(cr *databasev1alpha1.MongoCluster, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()

	for i, c := range cr.Status.Conditions {
		if c.Type == conditionType {
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

	cr.Status.Conditions = append(cr.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cr.Generation,
	})
}

func setAvailableCondition(cr *databasev1alpha1.MongoCluster, reason, message string) {
	setCondition(cr, ConditionAvailable, metav1.ConditionTrue, reason, message)
}

func setUnavailableCondition(cr *databasev1alpha1.MongoCluster, reason, message string) {
	setCondition(cr, ConditionAvailable, metav1.ConditionFalse, reason, message)
}

func setProgressingCondition(cr *databasev1alpha1.MongoCluster, reason, message string) {
	setCondition(cr, ConditionProgressing, metav1.ConditionTrue, reason, message)
}

func clearProgressingCondition(cr *databasev1alpha1.MongoCluster, reason, message string) {
	setCondition(cr, ConditionProgressing, metav1.ConditionFalse, reason, message)
}

func setDegradedCondition(cr *databasev1alpha1.MongoCluster, reason, message string) {
	setCondition(cr, ConditionDegraded, metav1.ConditionTrue, reason, message)
}

func clearDegradedCondition(cr *databasev1alpha1.MongoCluster, reason, message string) {
	setCondition(cr, ConditionDegraded, metav1.ConditionFalse, reason, message)
}

func setBackupReadyCondition(cr *databasev1alpha1.MongoCluster, reason, message string) {
	setCondition(cr, ConditionBackupReady, metav1.ConditionTrue, reason, message)
}

func clearBackupReadyCondition(cr *databasev1alpha1.MongoCluster, reason, message string) {
	setCondition(cr, ConditionBackupReady, metav1.ConditionFalse, reason, message)
}

func setArbiterReadyCondition(cr *databasev1alpha1.MongoCluster, reason, message string) {
	setCondition(cr, ConditionArbiterReady, metav1.ConditionTrue, reason, message)
}

func clearArbiterReadyCondition(cr *databasev1alpha1.MongoCluster, reason, message string) {
	setCondition(cr, ConditionArbiterReady, metav1.ConditionFalse, reason, message)
}

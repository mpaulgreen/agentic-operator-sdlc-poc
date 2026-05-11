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
	"testing"
)

func TestDefault_ReplicasZero(t *testing.T) {
	cr := &PostgresCluster{Spec: PostgresClusterSpec{Storage: StorageSpec{Size: "1Gi"}}}
	cr.Default()
	if cr.Spec.Replicas != 3 {
		t.Errorf("expected replicas=3, got %d", cr.Spec.Replicas)
	}
}

func TestDefault_VersionEmpty(t *testing.T) {
	cr := &PostgresCluster{Spec: PostgresClusterSpec{Replicas: 3, Storage: StorageSpec{Size: "1Gi"}}}
	cr.Default()
	if cr.Spec.Version != "16" {
		t.Errorf("expected version=16, got %s", cr.Spec.Version)
	}
}

func TestDefault_HAAntiAffinityMode(t *testing.T) {
	cr := &PostgresCluster{Spec: PostgresClusterSpec{Replicas: 3, Version: "16", Storage: StorageSpec{Size: "1Gi"}, HA: &HASpec{}}}
	cr.Default()
	if cr.Spec.HA.AntiAffinityMode != "preferred" {
		t.Errorf("expected antiAffinityMode=preferred, got %s", cr.Spec.HA.AntiAffinityMode)
	}
	if cr.Spec.HA.MinAvailable == nil || *cr.Spec.HA.MinAvailable != 2 {
		t.Errorf("expected minAvailable=2, got %v", cr.Spec.HA.MinAvailable)
	}
}

func TestValidate_MinAvailableGteReplicas(t *testing.T) {
	minAvail := int32(3)
	cr := &PostgresCluster{Spec: PostgresClusterSpec{Replicas: 3, Version: "16", Storage: StorageSpec{Size: "1Gi"}, HA: &HASpec{MinAvailable: &minAvail}}}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Error("expected error for minAvailable >= replicas")
	}
}

func TestValidate_BothMinAndMaxSet(t *testing.T) {
	minAvail, maxUnavail := int32(2), int32(1)
	cr := &PostgresCluster{Spec: PostgresClusterSpec{Replicas: 3, Version: "16", Storage: StorageSpec{Size: "1Gi"}, HA: &HASpec{MinAvailable: &minAvail, MaxUnavailable: &maxUnavail}}}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Error("expected error for both minAvailable and maxUnavailable set")
	}
}

func TestValidate_BackupEnabledNoSchedule(t *testing.T) {
	cr := &PostgresCluster{Spec: PostgresClusterSpec{Replicas: 3, Version: "16", Storage: StorageSpec{Size: "1Gi"}, Backup: &BackupSpec{Enabled: true}}}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Error("expected error for backup enabled without schedule")
	}
}

func TestValidate_StorageSizeReduction(t *testing.T) {
	cr := &PostgresCluster{Spec: PostgresClusterSpec{Replicas: 3, Version: "16", Storage: StorageSpec{Size: "5Gi"}}}
	old := &PostgresCluster{Spec: PostgresClusterSpec{Replicas: 3, Version: "16", Storage: StorageSpec{Size: "10Gi"}}}
	_, err := cr.ValidateUpdate(old)
	if err == nil {
		t.Error("expected error for storage size reduction")
	}
}

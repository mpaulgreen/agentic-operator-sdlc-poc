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

package v1beta1

import (
	"testing"
)

// --- Defaulting tests ---

func TestDefault_ReplicasZero(t *testing.T) {
	cr := &ElasticsearchCluster{Spec: ElasticsearchClusterSpec{Storage: StorageSpec{Size: "1Gi"}}}
	cr.Default()
	if cr.Spec.Replicas != 3 {
		t.Errorf("expected replicas=3, got %d", cr.Spec.Replicas)
	}
}

func TestDefault_VersionEmpty(t *testing.T) {
	cr := &ElasticsearchCluster{Spec: ElasticsearchClusterSpec{Replicas: 3, Storage: StorageSpec{Size: "1Gi"}}}
	cr.Default()
	if cr.Spec.Version != "8.14" {
		t.Errorf("expected version=8.14, got %s", cr.Spec.Version)
	}
}

func TestDefault_BackupRetentionDays(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "1Gi"},
			Backup:  &BackupSpec{Enabled: true, Schedule: "0 2 * * *"},
		},
	}
	cr.Default()
	if cr.Spec.Backup.RetentionDays != 7 {
		t.Errorf("expected retentionDays=7, got %d", cr.Spec.Backup.RetentionDays)
	}
}

func TestDefault_MasterReplicasWhenEnabled(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "1Gi"},
			Master:  &MasterSpec{Enabled: true},
		},
	}
	cr.Default()
	if cr.Spec.Master.Replicas != 3 {
		t.Errorf("expected master.replicas=3, got %d", cr.Spec.Master.Replicas)
	}
}

// --- Validation rejection tests ---

func TestValidate_ReplicasLessThanOne(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 0, Version: "8.14",
			Storage: StorageSpec{Size: "1Gi"},
		},
	}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Error("expected error for replicas < 1")
	}
}

func TestValidate_MasterReplicasEven(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "1Gi"},
			Master:  &MasterSpec{Enabled: true, Replicas: 4},
		},
	}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Error("expected error for even master replicas")
	}
}

func TestValidate_BothPasswordAndExistingSecret(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "1Gi"},
			Auth:    &AuthSpec{AdminPassword: "secret", ExistingSecret: "my-secret"},
		},
	}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Error("expected error for both auth.adminPassword and auth.existingSecret set")
	}
}

func TestValidate_BackupEnabledNoSchedule(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "1Gi"},
			Backup:  &BackupSpec{Enabled: true},
		},
	}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Error("expected error for backup enabled without schedule")
	}
}

func TestValidate_StorageSizeReduction(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "5Gi"},
		},
	}
	old := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "10Gi"},
		},
	}
	_, err := cr.ValidateUpdate(old)
	if err == nil {
		t.Error("expected error for storage size reduction")
	}
}

// --- Validation acceptance tests ---

func TestValidate_ValidCreateMinimal(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "10Gi"},
		},
	}
	_, err := cr.ValidateCreate()
	if err != nil {
		t.Errorf("expected no error for valid minimal create, got: %v", err)
	}
}

func TestValidate_ValidUpdateSameSize(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "10Gi"},
		},
	}
	old := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "10Gi"},
		},
	}
	_, err := cr.ValidateUpdate(old)
	if err != nil {
		t.Errorf("expected no error for update with same storage size, got: %v", err)
	}
}

func TestValidate_ValidUpdateLargerSize(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "20Gi"},
		},
	}
	old := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "10Gi"},
		},
	}
	_, err := cr.ValidateUpdate(old)
	if err != nil {
		t.Errorf("expected no error for update with larger storage size, got: %v", err)
	}
}

func TestValidate_ValidWithNilOptionals(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 1, Version: "8.12",
			Storage: StorageSpec{Size: "5Gi"},
		},
	}
	_, err := cr.ValidateCreate()
	if err != nil {
		t.Errorf("expected no error for valid create with nil optionals, got: %v", err)
	}
}

func TestValidate_ValidBackupWithSchedule(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "10Gi"},
			Backup:  &BackupSpec{Enabled: true, Schedule: "0 2 * * *", RetentionDays: 7},
		},
	}
	_, err := cr.ValidateCreate()
	if err != nil {
		t.Errorf("expected no error for backup with schedule, got: %v", err)
	}
}

func TestValidate_ValidMasterOddReplicas(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "10Gi"},
			Master:  &MasterSpec{Enabled: true, Replicas: 3},
		},
	}
	_, err := cr.ValidateCreate()
	if err != nil {
		t.Errorf("expected no error for master with odd replicas, got: %v", err)
	}
}

// --- ILM tests (new for v1beta1) ---

func TestValidate_ILMEnabledWithoutHotPhase(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "10Gi"},
			ILM:     &ILMSpec{Enabled: true},
		},
	}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Error("expected error for ilm.enabled without hotPhase")
	}
}

func TestValidate_ILMEnabledWithHotPhase(t *testing.T) {
	cr := &ElasticsearchCluster{
		Spec: ElasticsearchClusterSpec{
			Replicas: 3, Version: "8.14",
			Storage: StorageSpec{Size: "10Gi"},
			ILM:     &ILMSpec{Enabled: true, HotPhase: "30d", WarmPhase: "90d", DeletePhase: "365d"},
		},
	}
	_, err := cr.ValidateCreate()
	if err != nil {
		t.Errorf("expected no error for ilm.enabled with hotPhase, got: %v", err)
	}
}

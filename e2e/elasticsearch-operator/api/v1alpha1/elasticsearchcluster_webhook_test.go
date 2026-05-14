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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ---------------------------------------------------------------------------
// Defaulting tests
// ---------------------------------------------------------------------------

func TestDefault_ReplicasZeroDefaultsToThree(t *testing.T) {
	cr := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 0,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
		},
	}
	cr.Default()
	if cr.Spec.Replicas != 3 {
		t.Errorf("expected replicas=3, got %d", cr.Spec.Replicas)
	}
}

func TestDefault_VersionEmptyDefaultsTo814(t *testing.T) {
	cr := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "",
			Storage:  StorageSpec{Size: "10Gi"},
		},
	}
	cr.Default()
	if cr.Spec.Version != "8.14" {
		t.Errorf("expected version=8.14, got %s", cr.Spec.Version)
	}
}

func TestDefault_BackupRetentionDaysDefaultsToSeven(t *testing.T) {
	cr := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
			Backup: &BackupSpec{
				Enabled:       true,
				Schedule:      "0 2 * * *",
				RetentionDays: 0,
			},
		},
	}
	cr.Default()
	if cr.Spec.Backup.RetentionDays != 7 {
		t.Errorf("expected retentionDays=7, got %d", cr.Spec.Backup.RetentionDays)
	}
}

func TestDefault_MasterReplicasDefaultsToThree(t *testing.T) {
	cr := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
			Master: &MasterSpec{
				Enabled:  true,
				Replicas: 0,
			},
		},
	}
	cr.Default()
	if cr.Spec.Master.Replicas != 3 {
		t.Errorf("expected master replicas=3, got %d", cr.Spec.Master.Replicas)
	}
}

func TestDefault_DoesNotOverrideExplicitValues(t *testing.T) {
	cr := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 5,
			Version:  "8.12",
			Storage:  StorageSpec{Size: "10Gi"},
		},
	}
	cr.Default()
	if cr.Spec.Replicas != 5 {
		t.Errorf("expected replicas=5 (unchanged), got %d", cr.Spec.Replicas)
	}
	if cr.Spec.Version != "8.12" {
		t.Errorf("expected version=8.12 (unchanged), got %s", cr.Spec.Version)
	}
}

// ---------------------------------------------------------------------------
// ValidateCreate tests
// ---------------------------------------------------------------------------

func TestValidateCreate_ReplicasLessThanOneRejected(t *testing.T) {
	cr := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 0,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
		},
	}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Fatal("expected error for replicas < 1, got nil")
	}
}

func TestValidateCreate_EvenMasterReplicasRejected(t *testing.T) {
	cr := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
			Master: &MasterSpec{
				Enabled:  true,
				Replicas: 2,
			},
		},
	}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Fatal("expected error for even master replicas, got nil")
	}
}

func TestValidateCreate_MutualExclusionAuthRejected(t *testing.T) {
	cr := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
			Auth: &AuthSpec{
				AdminPassword:  "secret123",
				ExistingSecret: "my-secret",
			},
		},
	}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Fatal("expected error for mutual exclusion auth, got nil")
	}
}

func TestValidateCreate_BackupWithoutScheduleRejected(t *testing.T) {
	cr := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
			Backup: &BackupSpec{
				Enabled:  true,
				Schedule: "",
			},
		},
	}
	_, err := cr.ValidateCreate()
	if err == nil {
		t.Fatal("expected error for backup without schedule, got nil")
	}
}

func TestValidateCreate_ValidSpecAccepted(t *testing.T) {
	cr := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		},
	}
	_, err := cr.ValidateCreate()
	if err != nil {
		t.Fatalf("expected no error for valid spec, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ValidateUpdate tests
// ---------------------------------------------------------------------------

func TestValidateUpdate_StorageReductionRejected(t *testing.T) {
	old := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "20Gi"},
		},
	}
	newCR := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
		},
	}
	_, err := newCR.ValidateUpdate(old)
	if err == nil {
		t.Fatal("expected error for storage reduction, got nil")
	}
}

func TestValidateUpdate_StorageIncreaseAccepted(t *testing.T) {
	old := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
		},
	}
	newCR := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "20Gi"},
		},
	}
	_, err := newCR.ValidateUpdate(old)
	if err != nil {
		t.Fatalf("expected no error for storage increase, got %v", err)
	}
}

func TestValidateUpdate_SameStorageSizeAccepted(t *testing.T) {
	old := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
		},
	}
	newCR := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
		},
	}
	_, err := newCR.ValidateUpdate(old)
	if err != nil {
		t.Fatalf("expected no error for same storage size, got %v", err)
	}
}

func TestValidateUpdate_ReplicasLessThanOneRejected(t *testing.T) {
	old := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
		},
	}
	newCR := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 0,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
		},
	}
	_, err := newCR.ValidateUpdate(old)
	if err == nil {
		t.Fatal("expected error for replicas < 1 on update, got nil")
	}
}

// ---------------------------------------------------------------------------
// ValidateDelete tests
// ---------------------------------------------------------------------------

func TestValidateDelete_AlwaysAccepted(t *testing.T) {
	cr := &ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: ElasticsearchClusterSpec{
			Replicas: 3,
			Version:  "8.14",
			Storage:  StorageSpec{Size: "10Gi"},
		},
	}
	_, err := cr.ValidateDelete()
	if err != nil {
		t.Fatalf("expected no error for delete validation, got %v", err)
	}
}

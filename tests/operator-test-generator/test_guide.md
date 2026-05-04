# Sprint 7 Test Guide: `operator-test-generator` Subagent

## Prerequisites

- Sprints 1-6 complete (all 5 skills + operator-reviewer built)
- The agent is at `.claude/agents/operator-test-generator.md`
- A working redis-operator project at `/tmp/redis-operator-test/` with controller files but NO test files

## Test Order

1. **7.1**: Generate full test suite and validate
2. **7.2**: Incremental test generation for new method
3. **I-7**: Generate tests for a freshly built operator

---

## Test 7.1 — Generate Full Test Suite (Workflow A)

### Step 1: Ensure controller exists but test files don't

```bash
# Verify controller exists
test -f /tmp/redis-operator-test/internal/controller/rediscluster_reconcilers.go && echo "PASS: reconcilers" || echo "FAIL"

# Remove any existing test files
rm -f /tmp/redis-operator-test/internal/controller/*_test.go

# Verify no test files
test -z "$(find /tmp/redis-operator-test/internal/controller -name '*_test.go')" && echo "PASS: no test files" || echo "FAIL"
```

### Step 2: Prompt

```
Using the operator-test-generator agent, generate and validate tests for the 
RedisCluster controller at /tmp/redis-operator-test/internal/controller/. 
Identify all reconciler methods, generate test cases, and validate them. 
Report results.
```

### Step 3: Verify

```bash
# Files exist
test -f /tmp/redis-operator-test/internal/controller/suite_test.go && echo "PASS: suite_test.go" || echo "FAIL"
test -f /tmp/redis-operator-test/internal/controller/rediscluster_controller_test.go && echo "PASS: controller_test.go" || echo "FAIL"

# Tests compile
cd /tmp/redis-operator-test && go vet ./internal/controller/... && echo "GO VET: PASS" || echo "GO VET: FAIL"

# Test matrix
python3 .claude/skills/testing-operator/scripts/generate-test-matrix.py /tmp/redis-operator-test/internal/controller/

# Test coverage
bash .claude/skills/testing-operator/scripts/check-test-coverage.sh /tmp/redis-operator-test/

# Test case count
grep -c 'It(' /tmp/redis-operator-test/internal/controller/rediscluster_controller_test.go | xargs -I{} echo "Test cases: {}"
```

### Acceptance Criteria

- [ ] All 4 reconciler methods discovered (reconcileSecret, reconcileConfigMap, reconcileService, reconcileStatefulSet)
- [ ] suite_test.go generated with envtest setup
- [ ] controller_test.go generated with lifecycle + per-method tests
- [ ] `go vet` passes (tests compile)
- [ ] generate-test-matrix.py shows 4/4 methods covered (100%)
- [ ] At least 10 test cases (4 lifecycle + 4×2 per-method)
- [ ] Structured report output with discovery, files, and validation sections

---

## Test 7.2 — Incremental Test Generation (Workflow B)

### Step 1: Add PodDisruptionBudget method stub

Add this stub to `/tmp/redis-operator-test/internal/controller/rediscluster_reconcilers.go`:

```go
func (r *RedisClusterReconciler) reconcilePodDisruptionBudget(ctx context.Context, rc *cachev1alpha1.RedisCluster) error {
	logger := log.FromContext(ctx)
	name := fmt.Sprintf("%s-pdb", rc.Name)

	existing := &policyv1.PodDisruptionBudget{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: rc.Namespace}, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	minAvailable := intstr.FromInt(1)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rc.Namespace,
			Labels:    labelsForRedisCluster(rc),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: labelsForRedisCluster(rc),
			},
		},
	}

	if err := ctrl.SetControllerReference(rc, pdb, r.Scheme); err != nil {
		return err
	}

	logger.Info("Creating PodDisruptionBudget", "PDB.Name", name)
	if err := r.Create(ctx, pdb); err != nil {
		return err
	}

	r.Recorder.Eventf(rc, corev1.EventTypeNormal, "PDBCreated", "Created PodDisruptionBudget %s", name)
	return nil
}
```

Also add the import `policyv1 "k8s.io/api/policy/v1"` and add the RBAC marker:
```go
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
```

Verify it compiles:
```bash
cd /tmp/redis-operator-test && go build -o bin/manager ./cmd/main.go
```

### Step 2: Prompt

```
Using the operator-test-generator agent, a new reconcilePodDisruptionBudget() 
method was added to the RedisCluster controller at /tmp/redis-operator-test/. 
Generate tests for ONLY this new method and add them to the existing test file 
at internal/controller/rediscluster_controller_test.go. Do not modify existing 
tests. Validate afterward.
```

### Step 3: Verify

```bash
# PDB tests present
grep 'PodDisruptionBudget\|reconcilePodDisruptionBudget\|pdb' /tmp/redis-operator-test/internal/controller/rediscluster_controller_test.go > /dev/null && echo "PASS: PDB tests present" || echo "FAIL"

# Existing tests unchanged (should still have original lifecycle + per-method tests)
grep 'reconcileSecret\|reconcileService\|reconcileStatefulSet' /tmp/redis-operator-test/internal/controller/rediscluster_controller_test.go > /dev/null && echo "PASS: existing tests preserved" || echo "FAIL"

# Tests compile
cd /tmp/redis-operator-test && go vet ./internal/controller/... && echo "GO VET: PASS" || echo "GO VET: FAIL"

# Test matrix now 5/5
python3 .claude/skills/testing-operator/scripts/generate-test-matrix.py /tmp/redis-operator-test/internal/controller/
```

### Acceptance Criteria

- [ ] PDB test cases added to existing file
- [ ] Existing tests unchanged (Secret, ConfigMap, Service, StatefulSet tests still present)
- [ ] New tests follow same Ginkgo pattern (Context, It, BeforeEach)
- [ ] `go vet` passes
- [ ] Test matrix shows 5/5 methods covered

---

## Test I-7 — Generate Tests for Freshly Built Operator

### Step 1: Ensure database-operator exists

```bash
test -d ../go-operator/operators/database-operator/internal/controller && echo "PASS" || echo "FAIL"
```

### Step 2: Prompt

```
Using the operator-test-generator agent, generate and validate tests for 
the database-operator controller at 
go-operator/operators/database-operator/internal/controller/. Discover all 
reconciler methods and report what tests would be generated.
```

### Step 3: Verify

```bash
# Should discover 4 reconciler methods
# Should list resource types
# Report should be structured
```

### Acceptance Criteria

- [ ] All reconciler methods discovered
- [ ] Tests reference correct types from API package
- [ ] Report is structured with discovery + generated files + validation

---

## Cleanup

```bash
# Remove PDB stub if added
# Restore original reconcilers.go
```

## Troubleshooting

| Issue | Cause | Fix |
|-------|-------|-----|
| `go vet` fails after generation | Import missing | Add required imports (ginkgo, gomega, record, types) |
| Test matrix < 100% | New method not in test file | Check method name matches exactly |
| Existing tests broken | File overwritten instead of appended | Incremental should only add, never replace |
| FakeRecorder not available | Wrong import | Use `"k8s.io/client-go/tools/record"` |

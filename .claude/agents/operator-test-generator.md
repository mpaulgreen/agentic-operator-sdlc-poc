---
name: operator-test-generator
description: >
  Generates and validates test suites for operator controllers. Use when user asks
  to generate tests, add test coverage, create test suite, test a controller,
  or verify test completeness for an operator.
tools: Bash, Read, Edit, Write
---

# Operator Test Generator

Discover reconciler methods in an operator controller, generate comprehensive test suites using envtest and Ginkgo/Gomega, validate compilation and coverage, and report results. Uses the testing-operator skill patterns.

## Workflow

### Step 1: Discovery

Identify what to test by reading the controller files:

```bash
# Find controller files
find <project>/internal/controller -name '*.go' ! -name '*_test.go' | sort

# Parse reconcileX methods
grep -n 'func.*reconcile\w+\(' <reconcilers-file>

# Identify resource types
grep 'corev1\.\|appsv1\.\|metav1\.' <reconcilers-file> | grep -o '\w*{}' | sort -u
```

Read these files:
- `*_reconcilers.go` — all reconcileX() methods, what resources each creates
- `*_controller.go` — Reconcile() structure, RBAC markers, finalizer name
- `*_helpers.go` — utility functions (labels, password generation)
- `*_status.go` — status update logic
- `api/<version>/*_types.go` — CR Spec/Status fields for test setup

Produce a discovery summary:
```
Reconciler methods: reconcileSecret, reconcileConfigMap, reconcileService, reconcileStatefulSet
Resource types: Secret, ConfigMap, Service, StatefulSet
Finalizer: yes (<finalizer-name>)
Status updates: yes (readyReplicas, phase, conditions)
Helper functions: labelsForX, generatePassword
```

### Step 2: Generate Test Files

**Workflow A — Full Suite** (no existing tests):

Generate two files:

1. **`suite_test.go`** — envtest setup:
   - Global vars: `cfg`, `k8sClient`, `testEnv`
   - `TestControllers(t)` Ginkgo entry point
   - `BeforeSuite`: envtest.Environment with CRD paths, start, register scheme
   - `AfterSuite`: stop envtest
   - Follow template: `.claude/skills/testing-operator/assets/templates/suite_test.go.tmpl`

2. **`<kind>_controller_test.go`** — all test cases:
   - **Lifecycle tests** (Describe "When reconciling a <Kind>"):
     - "should add finalizer on first reconciliation"
     - "should create all managed resources"
     - "should be idempotent on repeated reconciliation"
     - "should handle deletion with finalizer cleanup"
   - **Per-method tests** (Context "When reconciling <Resource>"):
     - "should create <Resource> when absent" — verify data content, not just existence
     - "should not recreate existing <Resource> (idempotent)" — verify ResourceVersion unchanged
   - **Helper tests** (Context "When testing helper functions"):
     - Label keys and values
     - Password length and randomness

   Key patterns:
   - `BeforeEach`: create test CR with unique name (`fmt.Sprintf("test-%d", time.Now().UnixNano())`)
   - Reconciler with `record.NewFakeRecorder(100)`
   - `AfterEach`: remove finalizer then delete CR
   - Call reconcileX directly for per-method tests
   - Verify owner references: `Expect(resource.OwnerReferences).To(HaveLen(1))`

**Workflow B — Incremental** (tests already exist):

1. Read existing test file to understand structure
2. Add new Context block for the new method following the same pattern
3. Add create + idempotency test cases
4. Do NOT modify existing tests

### Step 3: Validate

Run these checks after generating tests:

```bash
# Tests compile
cd <project> && go vet ./internal/controller/...

# Test coverage matrix — all methods covered
python3 .claude/skills/testing-operator/scripts/generate-test-matrix.py <controller-dir>/

# Test case count
bash .claude/skills/testing-operator/scripts/check-test-coverage.sh <project>/
```

### Step 4: Report

Output a structured report:

```markdown
## Test Generation Report: <operator-name>

### Discovery
- Reconciler methods found: N
  - reconcileSecret, reconcileConfigMap, reconcileService, reconcileStatefulSet
- Resource types: Secret, ConfigMap, Service, StatefulSet
- Finalizer: <name>
- Helper functions: labelsForX, generatePassword

### Generated Files
- internal/controller/suite_test.go (envtest setup)
- internal/controller/<kind>_controller_test.go (N test cases)

### Validation
- go vet: PASS/FAIL
- Test matrix: N/N methods covered (100%)
- Test cases: N total (lifecycle: 4, per-method: N×2, helpers: 2)

### Issues (if any)
- [compilation errors, coverage gaps, or missing dependencies]
```

## envtest Limitations

Tests run against a real API server + etcd but with **no kubelet**:
- Pods won't actually run
- Deployments won't create ReplicaSets
- StatefulSets won't create Pods
- `ReadyReplicas` will stay 0

Tests verify the **reconciler creates the right objects** with correct specs, not that they become "Ready".

## Test Patterns Reference

### FakeRecorder
```go
reconciler := &MyReconciler{
    Client:   k8sClient,
    Scheme:   k8sClient.Scheme(),
    Recorder: record.NewFakeRecorder(100),
}
```

### Idempotency Test
```go
It("should not recreate existing Secret", func() {
    Expect(reconciler.reconcileSecret(ctx, cr)).To(Succeed())
    secret := &corev1.Secret{}
    Expect(k8sClient.Get(ctx, key, secret)).To(Succeed())
    originalVersion := secret.ResourceVersion

    Expect(reconciler.reconcileSecret(ctx, cr)).To(Succeed())
    Expect(k8sClient.Get(ctx, key, secret)).To(Succeed())
    Expect(secret.ResourceVersion).To(Equal(originalVersion))
})
```

### Content Verification
```go
It("should create ConfigMap with redis.conf", func() {
    Expect(reconciler.reconcileConfigMap(ctx, cr)).To(Succeed())
    cm := &corev1.ConfigMap{}
    Expect(k8sClient.Get(ctx, key, cm)).To(Succeed())
    Expect(cm.Data).To(HaveKey("redis.conf"))
    Expect(cm.Data["redis.conf"]).To(ContainSubstring("maxmemory"))
})
```

# Development Plan: Build & Test OpenShift Operator Skills

## Context

We have a validated architecture of 5 skills + 3 subagents for building OpenShift operators with Claude Agentic Skills (see `architecture.md`). This plan defines the progressive build order, testing methodology at each stage, and sample prompts that exercise each skill across the four composition scenarios.

## Testing Methodology

Testing a skill means invoking it with a prompt and verifying the output. Three layers:

| Test Layer | What It Validates | When Run |
|-----------|-------------------|----------|
| **Unit Test** | Skill in isolation produces correct artifacts | After each skill is built |
| **Integration Test** | Skills compose correctly — output of one is valid input for the next | After each skill pair/chain is ready |
| **E2E Scenario Test** | Full scenario (A/B/C/D) produces a working operator | After all skills + subagents are ready |

Verification methods for each layer:
- **Structural**: Are the right files produced in the right locations?
- **Compilable**: Does the generated Go code compile? (`go build ./...`)
- **Pattern-correct**: Does the code follow idempotency patterns? (validation scripts)
- **Testable**: Do generated tests run? (`make test`)
- **Bundleable**: Does the OLM bundle pass validation? (validation scripts)

---

## Build Order & Dependencies

```
Sprint 1: scaffolding-operator         (no dependencies)
Sprint 2: designing-operator-api       (references scaffolding output)
Sprint 3: implementing-reconciliation  (references API types)
Sprint 4: testing-operator             (references controller patterns)
Sprint 5: bundling-operator            (references API + controller)
Sprint 6: operator-reviewer            (subagent, uses skills 2+3)
Sprint 7: operator-test-generator      (subagent, uses skill 4)
Sprint 8: operator-bundle-validator    (subagent, uses skill 5)
```

Each sprint follows: **Build skill → Unit test → Integration test with prior skills**

---

## Sprint 1: `scaffolding-operator`

### Patterns Covered

| Pattern | Workflow | Description |
|---------|----------|-------------|
| A | New project | Scaffold complete operator project from scratch |
| B | Same-group kind | Add kind to existing API group (flat layout) |
| C | Different-group kind | Add kind in new API group (multi-group layout, aliased imports) |
| D | Cluster-scoped resource | Scaffold with `namespaced: false` and `scope=Cluster` marker |

### Build

29 files in `.claude/skills/scaffolding-operator/` (1 SKILL.md, 3 references, 1 script with 48 checks, 25 templates). Validated against `operator-sdk` v1.37.0.

See `tests/scaffolding-operator/test_guide.md` for full test prompts, verification commands, and acceptance criteria across all 4 patterns.

See `tests/scaffolding-operator/gap_analysis.md` for detailed comparison against `operator-sdk` output.

---

## Sprint 2: `designing-operator-api`

### Patterns Covered (from scaffolding research)

In addition to the core CRD design workflows, Sprint 2 covers these scaffolding-adjacent patterns that relate to API design:

| Pattern | Description |
|---------|-------------|
| E | Resource-only (no controller) — define a CRD type consumed by another controller |
| F | Controller-only for external types — watch core K8s types without defining a CRD |
| G | Multiple API versions (v1alpha1 → v1beta1) — version progression with `+kubebuilder:storageversion` |
| H | Validating/defaulting webhooks — `Default()`, `ValidateCreate/Update/Delete()` |
| I | Conversion webhooks — hub-and-spoke pattern for multi-version CRD conversion |

### Build
Create the skill directory and contents:
```
.claude/skills/designing-operator-api/
├── SKILL.md
├── references/
│   ├── type-design-patterns.md
│   ├── validation-markers.md
│   ├── cel-validation-rules.md
│   ├── status-conventions.md
│   ├── api-versioning.md
│   ├── webhook-patterns.md          # NEW — defaulting, validating, conversion webhooks
│   └── cluster-scoped-patterns.md   # NEW — cluster vs. namespaced design considerations
├── scripts/
│   └── validate-api-types.py
└── assets/
    ├── templates/
    │   ├── types.go.tmpl
    │   └── webhook.go.tmpl          # NEW — webhook handler template
    └── examples/
        ├── simple-spec.go
        ├── complex-spec.go
        ├── status-conditions.go
        └── cluster-scoped-types.go  # NEW — cluster-scoped type example
```

Source material: Extract from `go-operator/operators/database-operator/api/v1alpha1/databasecluster_types.go` and `model-registry-operator/api/v1alpha1/`.

### Unit Test

**Test 2.1 — Simple CRD design**
```
Prompt: "Using the designing-operator-api skill, design a CRD for RedisCluster 
with these requirements:
- Spec: replicas (3-7, default 3), version (enum: 6.0/7.0/7.2, default 7.2), 
  storage size, sentinel enabled (default true)
- Status: phase (Pending/Initializing/Running/Failed), readyReplicas, 
  conditions (Available/Progressing/Degraded), endpoint string
Generate api/v1alpha1/rediscluster_types.go with proper kubebuilder markers."
```

Expected output:
- `api/v1alpha1/rediscluster_types.go` with:
  - Spec struct with all fields and validation markers
  - Status struct with Conditions using `metav1.Condition`
  - Root type with `+kubebuilder:object:root=true`, `+kubebuilder:subresource:status`
  - PrintColumn markers for `kubectl get` output
  - List type
  - `groupversion_info.go` with SchemeBuilder

Verification:
```bash
# Script validation
python3 .claude/skills/designing-operator-api/scripts/validate-api-types.py \
  /tmp/redis-operator-test/api/v1alpha1/rediscluster_types.go

# Compilation check
cd /tmp/redis-operator-test && go build ./api/...
```

Acceptance criteria:
- [ ] All Spec fields have json tags
- [ ] Validation markers present (Minimum, Maximum, Enum, Default)
- [ ] Status has `[]metav1.Condition`
- [ ] Root type has `+kubebuilder:subresource:status`
- [ ] At least 2 PrintColumn markers (Phase, ReadyReplicas)
- [ ] validate-api-types.py passes
- [ ] Code compiles

**Test 2.2 — Complex CRD with nested types**
```
Prompt: "Using the designing-operator-api skill, extend the RedisCluster CRD 
to add a nested StorageSpec type (size string, storageClassName string, 
accessModes []string) and a ResourceSpec type (requests/limits for cpu and memory). 
Also add a BackupSpec (schedule cron string, retentionDays int, destination string)."
```

Acceptance criteria:
- [ ] Nested types defined as separate structs
- [ ] Each nested type has proper json tags and markers
- [ ] BackupSpec has cron validation pattern
- [ ] Code compiles with nested types

### Integration Test (Sprint 1 + 2)

**Test I-1.2 — Scaffold then Design**
```
Prompt: "First, scaffold a new operator project called 'message-queue-operator' 
with domain 'messaging.example.com'. Then design a CRD for MessageQueue with 
spec fields: queueType (kafka/rabbitmq/redis), replicas (1-10), 
persistentStorage (bool), and retentionHours (1-720). Status should include 
phase, readyReplicas, and standard conditions."
```

Verification:
```bash
# Full chain validation
bash .claude/skills/scaffolding-operator/scripts/validate-project-structure.sh /tmp/mq-operator-test/
python3 .claude/skills/designing-operator-api/scripts/validate-api-types.py /tmp/mq-operator-test/api/v1alpha1/messagequeue_types.go
cd /tmp/mq-operator-test && go mod tidy && go build ./...
```

Acceptance criteria:
- [ ] Project structure valid
- [ ] Types file valid
- [ ] Whole project compiles end-to-end
- [ ] PROJECT file matches types file (same group, version, kind)

---

## Sprint 3: `implementing-reconciliation`

### Build
Create the skill directory and contents:
```
.claude/skills/implementing-reconciliation/
├── SKILL.md
├── references/
│   ├── reconciliation-architecture.md
│   ├── idempotency-patterns.md
│   ├── resource-orchestration.md
│   ├── error-handling-patterns.md
│   ├── finalizer-lifecycle.md
│   ├── rbac-annotations.md
│   └── event-recording.md
├── scripts/
│   ├── validate-rbac-annotations.py
│   └── check-idempotency.py
└── assets/
    ├── templates/
    │   ├── controller.go.tmpl
    │   ├── reconciler-method.go.tmpl
    │   ├── resource-builder.go.tmpl
    │   ├── status-updater.go.tmpl
    │   ├── conditions.go.tmpl
    │   └── helpers.go.tmpl
    └── examples/
        ├── simple-reconciler.go
        ├── complex-reconciler.go
        └── ssa-reconciler.go
```

Source material: Extract from `go-operator/operators/database-operator/internal/controller/` (controller, reconcilers, status, conditions, helpers files).

### Unit Test

**Test 3.1 — Simple controller (3 resource types)**
```
Prompt: "Using the implementing-reconciliation skill, implement a controller 
for RedisCluster that reconciles these resources in order:
1. Secret (redis-credentials with generated password)
2. Service (headless service on port 6379)
3. StatefulSet (redis containers with spec.version image, spec.replicas count, 
   volume mounts for persistent storage)

Use the check-create idempotency pattern for each resource. Add finalizer 
for cleanup. Set owner references on all created resources. Update status 
conditions (Available, Progressing, Degraded) and status.readyReplicas 
from StatefulSet.

Generate these files:
- internal/controller/rediscluster_controller.go
- internal/controller/rediscluster_reconcilers.go  
- internal/controller/rediscluster_status.go
- internal/controller/rediscluster_conditions.go
- internal/controller/rediscluster_helpers.go"
```

Expected output:
- Controller with Reconcile() following three-phase pattern (fetch → orchestrate → status)
- Each reconcileX method using check-create: `Get → IsNotFound? → Create → SetOwnerRef → RecordEvent`
- Finalizer add on create, cleanup + remove on delete
- RBAC markers for all managed resource types
- Status updater reading StatefulSet readyReplicas

Verification:
```bash
# Idempotency check
python3 .claude/skills/implementing-reconciliation/scripts/check-idempotency.py \
  /tmp/redis-operator-test/internal/controller/rediscluster_reconcilers.go

# RBAC check
python3 .claude/skills/implementing-reconciliation/scripts/validate-rbac-annotations.py \
  /tmp/redis-operator-test/internal/controller/rediscluster_controller.go

# Compilation
cd /tmp/redis-operator-test && go build ./internal/controller/...
```

Acceptance criteria:
- [ ] All reconciler methods follow check-create pattern
- [ ] Owner references set on every created resource
- [ ] Finalizer implemented (add + cleanup + remove)
- [ ] RBAC annotations cover all managed resource types
- [ ] Events recorded for create and error
- [ ] Status conditions updated with proper types/reasons
- [ ] check-idempotency.py passes (no event-type-dependent logic)
- [ ] validate-rbac-annotations.py passes (no over/under-granting)
- [ ] Code compiles

**Test 3.2 — Add new resource to existing controller**
```
Prompt: "Using the implementing-reconciliation skill, add a new reconcileConfigMap() 
method to the existing RedisCluster controller. The ConfigMap should contain 
redis.conf with settings: maxmemory 256mb, maxmemory-policy allkeys-lru, 
timeout 300, tcp-keepalive 60. Follow the same check-create pattern as existing 
reconciler methods. Add the appropriate RBAC marker and call it in the correct 
dependency order (after Secret, before StatefulSet)."
```

Acceptance criteria:
- [ ] New reconcileConfigMap() follows same pattern as existing methods
- [ ] RBAC marker added for configmaps
- [ ] Called in correct position in Reconcile() chain
- [ ] ConfigMap data contains redis.conf settings
- [ ] Owner reference set
- [ ] Code compiles

### Integration Test (Sprint 1 + 2 + 3)

**Test I-1.2.3 — Scaffold → Design → Implement**
```
Prompt: "Build a complete operator from scratch:
1. Scaffold a project called 'cache-operator' with domain 'cache.example.com'
2. Design a CRD for CacheCluster with spec: engine (redis/memcached), 
   replicas (1-5), maxMemory string, evictionPolicy string. 
   Status: phase, readyReplicas, conditions.
3. Implement the controller that reconciles: Secret, ConfigMap, Service, 
   Deployment (not StatefulSet since cache is ephemeral). Use check-create 
   pattern, finalizers, owner refs, status updates."
```

Verification:
```bash
# Full chain
bash .claude/skills/scaffolding-operator/scripts/validate-project-structure.sh /tmp/cache-operator-test/
python3 .claude/skills/designing-operator-api/scripts/validate-api-types.py /tmp/cache-operator-test/api/v1alpha1/cachecluster_types.go
python3 .claude/skills/implementing-reconciliation/scripts/check-idempotency.py /tmp/cache-operator-test/internal/controller/cachecluster_reconcilers.go
cd /tmp/cache-operator-test && go mod tidy && go build ./...
```

Acceptance criteria:
- [ ] All three validation scripts pass
- [ ] Complete project compiles
- [ ] Controller references types from API package correctly
- [ ] RBAC markers match resources actually created in reconcilers

---

## Sprint 4: `testing-operator`

### Build
Create the skill directory and contents:
```
.claude/skills/testing-operator/
├── SKILL.md
├── references/
│   ├── envtest-setup.md
│   ├── ginkgo-patterns.md
│   ├── test-scenarios.md
│   └── e2e-patterns.md
├── scripts/
│   ├── check-test-coverage.sh
│   └── generate-test-matrix.py
└── assets/
    ├── templates/
    │   ├── suite_test.go.tmpl
    │   ├── controller_test.go.tmpl
    │   ├── reconciler_test.go.tmpl
    │   └── e2e_test.go.tmpl
    └── examples/
        └── database-controller-test.go
```

Source material: Extract from `go-operator/operators/database-operator/internal/controller/databasecluster_controller_test.go` and `suite_test.go`.

### Unit Test

**Test 4.1 — Generate controller test suite**
```
Prompt: "Using the testing-operator skill, generate a complete test suite for 
the RedisCluster controller at internal/controller/rediscluster_controller.go. 
The controller has these reconciler methods: reconcileSecret, reconcileConfigMap, 
reconcileService, reconcileStatefulSet, and uses finalizers.

Generate:
- internal/controller/suite_test.go (envtest setup)
- internal/controller/rediscluster_controller_test.go (all test cases)

For each reconciler method, test:
1. Creates resource when absent
2. Is idempotent when resource already exists (no error, no duplicate)
3. Handles errors gracefully
4. Updates status on spec change

Also test:
- Finalizer is added on first reconciliation
- Deletion triggers cleanup
- Status conditions are set correctly"
```

Expected output:
- `suite_test.go` with envtest Environment, CRD paths, k8sClient, testEnv
- `controller_test.go` with Ginkgo Describe/Context/It structure
- Per-reconciler test blocks (4 tests × 4 methods = 16 test cases)
- Finalizer test (add + cleanup)
- Status test (phase transitions)

Verification:
```bash
# Test matrix check
python3 .claude/skills/testing-operator/scripts/generate-test-matrix.py \
  /tmp/redis-operator-test/internal/controller/rediscluster_controller.go \
  /tmp/redis-operator-test/internal/controller/rediscluster_controller_test.go

# Run tests (requires envtest binaries)
cd /tmp/redis-operator-test && make test
```

Acceptance criteria:
- [ ] suite_test.go compiles and sets up envtest
- [ ] Every reconciler method has at least 3 test cases (create, idempotent, error)
- [ ] Finalizer lifecycle tested (add, cleanup, remove)
- [ ] Status updates tested
- [ ] generate-test-matrix.py shows full coverage
- [ ] `make test` passes (or fails only due to envtest binary availability, not code errors)

**Test 4.2 — Generate tests for single new method**
```
Prompt: "Using the testing-operator skill, generate tests for ONLY the new 
reconcileConfigMap() method that was added to the RedisCluster controller. 
Add tests to the existing test file. Test create, idempotent, error, and 
verify the ConfigMap data contains the expected redis.conf settings."
```

Acceptance criteria:
- [ ] Tests added to existing file (not a new file)
- [ ] Tests verify ConfigMap data content, not just existence
- [ ] Tests follow same Ginkgo pattern as existing tests

### Integration Test (Sprint 1 + 2 + 3 + 4)

**Test I-1.2.3.4 — Full dev cycle through testing**
```
Prompt: "Build and test a complete operator:
1. Scaffold 'notification-operator' with domain 'notify.example.com'
2. Design CRD for NotificationChannel: spec has type (email/slack/pagerduty), 
   endpoint string, retryCount (1-5), retryDelay string. 
   Status: phase, lastDelivery timestamp, conditions.
3. Implement controller reconciling: Secret (API credentials), ConfigMap (channel config), 
   Deployment (notification worker)
4. Generate complete test suite and run it."
```

Verification:
```bash
cd /tmp/notification-operator-test && go mod tidy && make test
```

Acceptance criteria:
- [ ] Project scaffolds cleanly
- [ ] Types compile with markers
- [ ] Controller compiles with reconcilers
- [ ] Tests compile and cover all reconciler methods
- [ ] `make test` runs (pass or fail with clear diagnostics)

---

## Sprint 5: `bundling-operator`

### Build
Create the skill directory and contents:
```
.claude/skills/bundling-operator/
├── SKILL.md
├── references/
│   ├── csv-anatomy.md
│   ├── bundle-structure.md
│   ├── olm-v0-vs-v1.md
│   ├── scorecard-tests.md
│   ├── certification-checklist.md
│   └── catalog-management.md
├── scripts/
│   ├── validate-csv.py
│   ├── validate-bundle-structure.sh
│   └── check-scorecard-readiness.py
└── assets/
    ├── templates/
    │   ├── csv.yaml.tmpl
    │   ├── annotations.yaml.tmpl
    │   ├── bundle.dockerfile.tmpl
    │   └── scorecard-config.yaml.tmpl
    └── examples/
        └── database-operator-csv.yaml
```

Source material: Extract from `go-operator/operators/database-operator/bundle/` (CSV, annotations, Dockerfile).

### Unit Test

**Test 5.1 — Generate initial bundle**
```
Prompt: "Using the bundling-operator skill, create an OLM bundle for the 
redis-operator v0.1.0. The operator:
- Manages RedisCluster CRD (redis.example.com/v1alpha1)
- CRD spec fields: replicas, version, storage, sentinel, persistencePolicy
- CRD status fields: phase, readyReplicas, conditions, endpoint
- Needs RBAC for: statefulsets, services, secrets, configmaps, pods
- Install mode: OwnNamespace, SingleNamespace
- Channel: alpha
- Category: Database
- Display name: Redis Operator
- Description: Manages Redis clusters on OpenShift with HA via Sentinel

Generate:
- bundle/manifests/redis-operator.clusterserviceversion.yaml
- bundle/manifests/redis.example.com_redisclusters.yaml (CRD)
- bundle/metadata/annotations.yaml
- bundle/tests/scorecard/config.yaml
- bundle.Dockerfile"
```

Expected output:
- CSV with all required sections (metadata, spec.customresourcedefinitions, spec.install, spec.installModes)
- specDescriptors and statusDescriptors matching CRD fields
- RBAC permissions in CSV matching controller RBAC markers
- alm-examples with a valid sample CR
- annotations.yaml with correct mediatype, package, channel
- scorecard config with basic and olm test suites

Verification:
```bash
# Bundle structure
bash .claude/skills/bundling-operator/scripts/validate-bundle-structure.sh /tmp/redis-operator-test/bundle/

# CSV validation
python3 .claude/skills/bundling-operator/scripts/validate-csv.py \
  /tmp/redis-operator-test/bundle/manifests/redis-operator.clusterserviceversion.yaml

# Scorecard readiness
python3 .claude/skills/bundling-operator/scripts/check-scorecard-readiness.py /tmp/redis-operator-test/bundle/
```

Acceptance criteria:
- [ ] Bundle structure valid (manifests/, metadata/, tests/)
- [ ] CSV has all required sections
- [ ] specDescriptors match CRD spec fields
- [ ] statusDescriptors match CRD status fields
- [ ] RBAC in CSV matches controller RBAC markers
- [ ] alm-examples contains valid sample CR YAML
- [ ] All three validation scripts pass

**Test 5.2 — Update bundle for new version**
```
Prompt: "Using the bundling-operator skill, update the redis-operator bundle 
from v0.1.0 to v0.2.0. Changes: added BackupSpec to CRD (schedule, retentionDays, 
destination fields) and new reconcileCronJob method. Update the CSV with:
- New version 0.2.0
- replaces: redis-operator.v0.1.0
- New RBAC for batch/cronjobs
- New specDescriptors for backup fields
- Updated alm-examples showing backup configuration"
```

Acceptance criteria:
- [ ] CSV version updated to 0.2.0
- [ ] `spec.replaces` set to redis-operator.v0.1.0
- [ ] New RBAC entry for batch/cronjobs
- [ ] New specDescriptors for backup.schedule, backup.retentionDays, backup.destination
- [ ] alm-examples includes backup configuration

### Integration Test (All 5 Skills)

**Test I-1.2.3.4.5 — Full skill chain**
```
Prompt: "Build a complete operator end-to-end:
1. Scaffold 'metrics-collector-operator' with domain 'observability.example.com'
2. Design CRD for MetricsCollector: spec has scrapeInterval (string), 
   targets ([]string), retentionDays (1-90), storageSize (string). 
   Status: phase, targetsDiscovered (int), lastScrape (timestamp), conditions.
3. Implement controller reconciling: ConfigMap (scrape config), 
   Service (metrics endpoint), Deployment (collector), ServiceMonitor (Prometheus)
4. Generate test suite
5. Create OLM bundle v0.1.0 with channel 'stable', category 'Monitoring'"
```

Verification:
```bash
# All validations
bash .claude/skills/scaffolding-operator/scripts/validate-project-structure.sh /tmp/metrics-operator-test/
python3 .claude/skills/designing-operator-api/scripts/validate-api-types.py /tmp/metrics-operator-test/api/v1alpha1/metricscollector_types.go
python3 .claude/skills/implementing-reconciliation/scripts/check-idempotency.py /tmp/metrics-operator-test/internal/controller/metricscollector_reconcilers.go
python3 .claude/skills/bundling-operator/scripts/validate-csv.py /tmp/metrics-operator-test/bundle/manifests/metrics-collector-operator.clusterserviceversion.yaml
cd /tmp/metrics-operator-test && go mod tidy && go build ./... && make test
```

Acceptance criteria:
- [ ] All 5 validation scripts pass
- [ ] Project compiles end-to-end
- [ ] Tests compile and run
- [ ] Bundle is structurally valid
- [ ] CSV descriptors match CRD fields
- [ ] RBAC in CSV matches controller annotations

---

## Sprint 6: `operator-reviewer` (Subagent)

### Build
Create the subagent definition:
```
.claude/agents/operator-reviewer.md
```

Assigns skills: `implementing-reconciliation`, `designing-operator-api`

### Unit Test

**Test 6.1 — Review operator with known issues**

First, create a deliberately flawed operator for the reviewer to catch.

```
Prompt: "Review the operator code at /tmp/redis-operator-test/ for best practices 
and common mistakes. Check the controller, API types, and RBAC."
```

Plant these issues in the test operator before running:
1. A reconcileService() that does `r.Create()` without checking if resource exists (non-idempotent)
2. A reconcileConfigMap() missing `controllerutil.SetOwnerReference()` 
3. An RBAC marker granting `*` verbs instead of least privilege
4. Missing status condition update in error path
5. Finalizer that doesn't re-fetch CR before removing

Expected output: Structured review identifying all 5 planted issues with severity, line numbers, and fix suggestions.

Acceptance criteria:
- [ ] All 5 planted issues detected
- [ ] Each issue has severity (Critical/Warning)
- [ ] Each issue has line number reference
- [ ] Each issue has a concrete fix suggestion
- [ ] No false positives on correct code

**Test 6.2 — Review clean operator**
```
Prompt: "Review the database-operator at go-operator/operators/database-operator/ 
for best practices."
```

Acceptance criteria:
- [ ] No false Critical findings on production code
- [ ] Warnings are genuine improvement suggestions
- [ ] Review completes without errors

### Integration Test (Subagent + Skills 2+3)

**Test I-6 — Review then fix**
```
Prompt: "Review the operator at /tmp/redis-operator-test/. For any Critical 
findings, fix them using the implementing-reconciliation skill patterns."
```

Acceptance criteria:
- [ ] Review identifies issues
- [ ] Fixes follow patterns from implementing-reconciliation references
- [ ] Re-review shows Critical issues resolved

---

## Sprint 7: `operator-test-generator` (Subagent)

### Build
Create the subagent definition:
```
.claude/agents/operator-test-generator.md
```

Assigns skills: `testing-operator`

### Unit Test

**Test 7.1 — Generate and run tests**
```
Prompt: "Generate and run tests for the RedisCluster controller at 
/tmp/redis-operator-test/internal/controller/. Identify all reconciler 
methods, generate test cases, and run them. Report results."
```

Expected output:
- List of discovered reconciler methods
- Generated test file (or updated existing)
- Test execution results with pass/fail counts
- For any failures: specific diagnosis and fix suggestions

Acceptance criteria:
- [ ] All reconciler methods discovered
- [ ] Tests generated for each method
- [ ] Tests actually executed (not just generated)
- [ ] Results reported with pass/fail breakdown

**Test 7.2 — Incremental test generation**
```
Prompt: "A new reconcilePodDisruptionBudget() method was added to the 
RedisCluster controller. Generate tests for ONLY this new method and 
add them to the existing test file. Run the full test suite afterward."
```

Acceptance criteria:
- [ ] Only new tests added (existing tests unchanged)
- [ ] New tests follow same pattern as existing ones
- [ ] Full suite runs after addition

### Integration Test (Subagent + Skill 4 + Prior Skills)

**Test I-7 — Generate tests for freshly built operator**
```
Prompt: "I just built the cache-operator. Generate and run the complete 
test suite for it."
```

Acceptance criteria:
- [ ] Tests generated match the controller structure
- [ ] Tests reference correct types from API package
- [ ] Tests compile and run

---

## Sprint 8: `operator-bundle-validator` (Subagent)

### Build
Create the subagent definition:
```
.claude/agents/operator-bundle-validator.md
```

Assigns skills: `bundling-operator`

### Unit Test

**Test 8.1 — Validate correct bundle**
```
Prompt: "Validate the OLM bundle at /tmp/redis-operator-test/bundle/ 
for correctness and certification readiness."
```

Expected output: 
- Bundle structure: PASS
- CSV validation: PASS with details on each section
- Scorecard readiness: list of tests that would pass/fail
- Certification checklist: items met / not met

Acceptance criteria:
- [ ] Clean bundle passes validation
- [ ] Each validation category reported separately
- [ ] Certification gaps identified (e.g., missing icon, no tests)

**Test 8.2 — Validate bundle with deliberate issues**

Plant these issues:
1. Missing annotations.yaml
2. CSV missing spec.installModes
3. alm-examples with invalid YAML
4. specDescriptor referencing non-existent field

```
Prompt: "Validate the bundle at /tmp/redis-operator-test/bundle/ and 
report all issues found."
```

Acceptance criteria:
- [ ] All 4 planted issues detected
- [ ] Each issue has actionable fix suggestion
- [ ] Issues categorized by severity

### Integration Test (Subagent + Skill 5)

**Test I-8 — Validate then fix**
```
Prompt: "Validate the bundle. For any issues found, fix them using 
the bundling-operator skill and re-validate."
```

Acceptance criteria:
- [ ] Issues identified
- [ ] Fixes applied
- [ ] Re-validation passes

---

## E2E Scenario Tests (After All Sprints)

These test complete workflows across all skills and subagents.

### Scenario A: New Operator from Scratch

```
Prompt: "Build me a complete OpenShift operator that manages PostgreSQL 
clusters. Requirements:

Spec:
- replicas: 1-5, default 3
- version: enum 14/15/16, default 16
- storage: size (string), storageClassName (string)
- resources: cpu/memory requests and limits
- backup: enabled (bool), schedule (cron string), retentionDays (1-30)

Status:
- phase: Pending/Initializing/Running/Failed/Degraded
- readyReplicas, currentVersion, endpoint
- conditions: Available, Progressing, Degraded, BackupReady

Controller should reconcile:
- Secret (superuser credentials)
- ConfigMap (postgresql.conf)
- Service (headless, port 5432)
- StatefulSet (postgres containers with PVCs)
- CronJob (if backup.enabled, pg_dump on schedule)

Generate the complete project, tests, and OLM bundle v0.1.0 on alpha channel.
Review the code for best practices before finalizing."
```

Verification — run ALL validation scripts, compile, test, bundle validate.

Acceptance criteria:
- [ ] Complete project structure valid
- [ ] Types compile with all markers
- [ ] Controller compiles with 5 reconciler methods
- [ ] Tests compile and cover all methods
- [ ] Bundle valid with matching descriptors
- [ ] Code review shows no Critical issues
- [ ] Total files created: ~25-30

---

### Scenario B: Add Feature to Existing Operator

Uses the database-operator from the knowledgebase as the target.

```
Prompt: "Add backup support to the existing database operator at 
go-operator/operators/database-operator/. 

1. Add to the CRD: BackupSpec with schedule (cron string), 
   retentionDays (int, 1-30, default 7), destination (s3/local, default local)
2. Add to Status: BackupStatus with lastBackup (timestamp), 
   lastBackupResult (Success/Failed), add BackupReady condition
3. Add reconcileCronJob() to the controller for scheduled pg_dump
4. Generate tests for the new reconciler method
5. Update the OLM bundle from current version to next version with 
   'replaces' set correctly"
```

Verification:
```bash
# Types still compile with additions
cd go-operator/operators/database-operator && go build ./api/...

# Controller still compiles with new reconciler
go build ./internal/controller/...

# New tests compile and existing tests still pass
make test

# Bundle valid
python3 .claude/skills/bundling-operator/scripts/validate-csv.py bundle/manifests/*.clusterserviceversion.yaml
```

Acceptance criteria:
- [ ] Existing code unchanged except for additions
- [ ] New types integrate with existing Spec/Status
- [ ] New reconciler follows same pattern as existing ones
- [ ] Tests added for new method only
- [ ] CSV version incremented with `replaces` set
- [ ] New descriptors added without removing existing ones

---

### Scenario C: Review Existing Operator

```
Prompt: "Perform a comprehensive review of the operator at 
go-operator/operators/database-operator/. Check:
1. API types for missing markers, incorrect validation
2. Controller for idempotency violations, missing owner refs, RBAC issues
3. Tests for coverage gaps
4. Bundle for CSV completeness and certification readiness

Produce a structured report with findings and recommended fixes."
```

Acceptance criteria:
- [ ] Review covers all 4 areas (types, controller, tests, bundle)
- [ ] Findings categorized by severity
- [ ] Each finding has file path and line number
- [ ] Fix recommendations reference specific skill patterns
- [ ] No false Critical findings on working production code

---

### Scenario D: Prepare for Certification

```
Prompt: "Prepare the redis-operator for Red Hat certification. 
1. Validate the bundle structure and CSV
2. Check certification prerequisites (icon, description, examples, test config)
3. Check security requirements (non-root, read-only fs, minimal capabilities)
4. Check RBAC for least privilege
5. Generate a certification readiness report with pass/fail for each requirement"
```

Acceptance criteria:
- [ ] Each certification requirement checked individually
- [ ] Clear pass/fail per requirement
- [ ] Actionable remediation steps for each failure
- [ ] Security posture verified (Dockerfile, RBAC, SCC)
- [ ] Bundle passes all validation scripts

---

## Summary: Test Matrix

| Sprint | Component | Unit Tests | Integration Tests | Scenario Coverage |
|--------|-----------|-----------|-------------------|-------------------|
| 1 | scaffolding-operator | 1.1, 1.2 | — | A |
| 2 | designing-operator-api | 2.1, 2.2 | I-1.2 | A, B |
| 3 | implementing-reconciliation | 3.1, 3.2 | I-1.2.3 | A, B |
| 4 | testing-operator | 4.1, 4.2 | I-1.2.3.4 | A, B |
| 5 | bundling-operator | 5.1, 5.2 | I-1.2.3.4.5 | A, B, D |
| 6 | operator-reviewer | 6.1, 6.2 | I-6 | C |
| 7 | operator-test-generator | 7.1, 7.2 | I-7 | A, B |
| 8 | operator-bundle-validator | 8.1, 8.2 | I-8 | D |
| Final | All components | — | — | A, B, C, D |

**Total**: 16 unit tests + 7 integration tests + 4 E2E scenario tests = **27 test points**

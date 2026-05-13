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

27 files in `.claude/skills/scaffolding-operator/` (1 SKILL.md, 3 references, 1 script with 49 checks, 22 templates). Validated against `operator-sdk` v1.37.0. Updated: removed kube-rbac-proxy sidecar templates (deprecated in operator-sdk v1.33+), added metrics-service and metrics-reader-clusterrole. Dockerfile uses `FROM --platform=$BUILDPLATFORM` for cross-compilation on Apple Silicon.

See `tests/scaffolding-operator/test_guide.md` for full test prompts, verification commands, and acceptance criteria across all 4 patterns.

See `tests/scaffolding-operator/gap_analysis.md` for detailed comparison against `operator-sdk` output.

---

## Sprint 2: `designing-operator-api`

### Patterns Covered

| Pattern | Description | Workflow |
|---------|-------------|----------|
| E | Resource-only (no controller) — documented in references, scaffolded by Sprint 1 | Ref only |
| F | Controller-only for external types — documented in references, scaffolded by Sprint 1 | Ref only |
| G | Multiple API versions (v1alpha1 → v1beta1) with `+kubebuilder:storageversion` | Workflow D |
| H | Validating/defaulting webhooks — Default(), ValidateCreate/Update/Delete() | Workflow C |
| I | Conversion webhooks — hub-and-spoke pattern | Workflow C (with conversion) |

### Build

24 files in `.claude/skills/designing-operator-api/` (1 SKILL.md, 7 references, 1 script with 14 checks, 11 templates, 4 examples). Validated against `operator-sdk`.

See `tests/designing-operator-api/test_guide.md` for full test prompts, verification commands, and acceptance criteria across all workflows.

See `tests/designing-operator-api/gap_analysis.md` for detailed comparison against `operator-sdk` output.

---

## Sprint 3: `implementing-reconciliation`

### Build

19 files in `.claude/skills/implementing-reconciliation/` (1 SKILL.md, 7 references, 2 scripts, 6 templates, 3 examples). Scripts validated against real database-operator (10 RBAC markers, all idempotency checks pass).

See `tests/implementing-reconciliation/test_guide.md` for full test prompts, verification commands, and acceptance criteria.

See `tests/implementing-reconciliation/gap_analysis.md` for detailed comparison against `operator-sdk` output.

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

12 files in `.claude/skills/testing-operator/` (1 SKILL.md, 4 references, 2 scripts, 4 templates, 1 example). Scripts validated: check-test-coverage finds test files/cases, generate-test-matrix verifies 100% method coverage.

See `tests/testing-operator/test_guide.md` for full test prompts, verification commands, and acceptance criteria.

See `tests/testing-operator/gap_analysis.md` for detailed comparison against `operator-sdk` test output.

---

## Sprint 5: `bundling-operator`

### Build

15 files in `.claude/skills/bundling-operator/` (1 SKILL.md, 6 references, 3 scripts, 4 templates, 1 example). Scripts validated against database-operator bundle (0 errors each).

See `tests/bundling-operator/test_guide.md` for full test prompts, verification commands, and acceptance criteria.

See `tests/bundling-operator/gap_analysis.md` for detailed comparison against `make bundle` output.

### Unit Test

**Test 5.1 — Generate initial bundle**
```
Prompt: see tests/bundling-operator/test_guide.md Test 5.1
```

**Test 5.2 — Update bundle for new version**
```
Prompt: see tests/bundling-operator/test_guide.md Test 5.2
```

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

1 agent definition at `.claude/agents/operator-reviewer.md`. Composes skills 2+3 (designing-operator-api + implementing-reconciliation). Runs 3 validation scripts + manual checklist. Produces structured review with severity, line numbers, and fix suggestions.

See `tests/operator-reviewer/test_guide.md` for full test prompts (including flawed code to plant), verification commands, and acceptance criteria.

See `tests/operator-reviewer/gap_analysis.md` for comparison of automated vs manual review coverage.

---

## Sprint 7: `operator-test-generator` (Subagent)

### Build

1 agent definition at `.claude/agents/operator-test-generator.md`. Uses skill 4 (testing-operator). Discovers reconciler methods, generates suite_test.go + controller_test.go, validates with go vet + test matrix.

See `tests/operator-test-generator/test_guide.md` for full test prompts, verification commands, and acceptance criteria.

See `tests/operator-test-generator/gap_analysis.md` for comparison against manual test writing and operator-sdk.

---

## Sprint 8: `operator-bundle-validator` (Subagent)

### Build

1 agent definition at `.claude/agents/operator-bundle-validator.md`. Uses skill 5 (bundling-operator). Runs 3 validation scripts + certification checklist inspection.

See `tests/operator-bundle-validator/test_guide.md` for full test prompts (including 4 issues to plant), verification commands, and acceptance criteria.

See `tests/operator-bundle-validator/gap_analysis.md` for comparison against `operator-sdk bundle validate`.

---

## E2E Validation by Operator Category

Operator projects are complex and diverse. E2E validation is organized by operator category, with each category testing the skills against different workload patterns, resource types, and operational concerns.

> **Mandatory workflow (from CLAUDE.md):** E2E tests MUST follow the skill/subagent workflow: Skills for generation (Steps 1-3, 5), Subagents for verification (Steps 4a, 4b, 6). Do NOT write operator code from training knowledge.

### Validation Categories

| Category | Examples | Patterns Tested | Status |
|----------|---------|-----------------|--------|
| **Stateful Workloads** | PostgreSQL, Redis, Kafka, MongoDB, Elasticsearch | StatefulSet, PVC, backup CronJob/Job, connection pooling, HA (PDB/anti-affinity/arbiter) | **PostgreSQL DONE** (111/111), **Redis A-E DONE** (139/139, 0 fixes), **MongoDB A-B DONE** (59/59, 1 fix) |
| **Application Platform** | RHOAI, Tekton, ArgoCD, ServiceMesh | Deployment, multi-component, cross-namespace | Planned |
| **Infrastructure / Cloud** | Cluster autoscaler, node management | Cluster-scoped CRDs, node selectors, taints | Planned |
| **Network / Security** | cert-manager, Kuadrant, External DNS | Webhooks, NetworkPolicy, TLS certificates, ingress | Partial (tested within PostgreSQL C) |
| **Observability** | Prometheus, Grafana, Loki, Jaeger | ServiceMonitor, PrometheusRule, dashboards | Planned |
| **Storage / Data** | Rook-Ceph, MinIO, NFS provisioner | StorageClass, PV management, CSI | Planned |
| **ML / AI** | KServe, Ray, Training Operator | GPU scheduling, model serving, batch jobs | Planned |
| **CI/CD & GitOps** | Tekton, ArgoCD, Flux | Pipeline runs, GitRepository, Application sync | Planned |

### Stateful Workloads

#### PostgreSQL Operator (COMPLETE)

Progressive enhancement across 4 scenarios testing all 4 designing-operator-api workflows:

- **Prompts**: [`e2e/docs/statefulsets/postgres-prompts.md`](../e2e/docs/statefulsets/postgres-prompts.md)
- **Validation guide**: [`e2e/docs/statefulsets/postgres-e2e-validation.md`](../e2e/docs/statefulsets/postgres-e2e-validation.md)
- **Operator code**: `e2e/postgres-operator/`
- **Results**: 4 scenarios, 111 test conditions, 17 skill bugs found and fixed, all pass on OpenShift

| Scenario | Feature | Version | Tests | Skills Exercised |
|----------|---------|---------|-------|-----------------|
| A | Core (from scratch) | v0.1.0 | 31 | All 5 skills (Workflow A) + 3 subagents |
| B | High Availability | v0.2.0 | 25 | 4 skills (Workflow B) + 3 subagents |
| C | Webhooks + NetworkPolicy | v0.3.0 | 27 | 4 skills (Workflow C) + 3 subagents |
| D | API versioning + Connection Pooling | v0.4.0 | 28 | 4 skills (Workflow D) + 3 subagents |

#### Redis Operator (COMPLETE)

Different stateful workload to validate skill generality. Tests 5 scenarios (A-E) covering all 4 skill workflows plus the multi-CRD expansion workflow (scaffolding Workflow B). Tests different operand patterns (2 Services, Sentinel Deployment, TLS support, RedisUser second CRD). Designed as a regression test for all 17 PostgreSQL bug fixes plus the multi-CRD gap.

- **Prompts**: [`e2e/docs/statefulsets/redis-prompts.md`](../e2e/docs/statefulsets/redis-prompts.md)
- **Validation guide**: [`e2e/docs/statefulsets/redis-e2e-validation.md`](../e2e/docs/statefulsets/redis-e2e-validation.md)
- **Operator code**: `e2e/redis-operator/`
- **Results**: 5 scenarios, 139 test conditions, zero skill modifications needed, all pass on OpenShift (both deploy paths)

| Scenario | Feature | Version | Tests | Skills Exercised |
|----------|---------|---------|-------|-----------------|
| A | Core (from scratch) | v0.1.0 | 34 | All 5 skills (Workflow A) + 3 subagents |
| B | Redis Sentinel HA | v0.2.0 | 22 | 4 skills (Workflow B) + 3 subagents |
| C | Webhooks + NetworkPolicy | v0.3.0 | 24 | 4 skills (Workflow C) + 3 subagents |
| D | API Maturity + TLS | v0.4.0 | 25 | 4 skills (Workflow D) + 3 subagents |
| E | Add Second CRD (RedisUser) | v0.5.0 | 34 | scaffolding B + all |

#### MongoDB Operator (In Progress)

Gap-coverage test targeting untested skill patterns: Job (batch/v1) reconciliation and different-group multi-CRD (scaffolding Workflow C). Tests MongoDB-specific patterns (2 Secrets, YAML ConfigMap, arbiter Deployment, backup Job).

- **Prompts**: [`e2e/docs/statefulsets/mongodb-prompts.md`](../e2e/docs/statefulsets/mongodb-prompts.md)
- **Validation guide**: [`e2e/docs/statefulsets/mongodb-e2e-validation.md`](../e2e/docs/statefulsets/mongodb-e2e-validation.md)
- **Operator code**: `e2e/mongodb-operator/`
- **Results**: Scenarios A-B complete — 59/59 test conditions pass on OpenShift (both deploy paths), 1 skill fix (Bug #18: check-idempotency.py List() support). Scenarios C-E pending.

| Scenario | Feature | Version | Tests | Skills Exercised |
|----------|---------|---------|-------|-----------------|
| A | Core with Job backup | v0.1.0 | 41 | All 5 skills (Workflow A) + 3 subagents |
| B | Arbiter node | v0.2.0 | 18 | 4 skills (Workflow B) + 3 subagents |
| C | Webhooks + NetworkPolicy | v0.3.0 | — | Pending |
| D | API Maturity + Sharding | v0.4.0 | — | Pending |
| E | Different-group CRD | v0.5.0 | — | Pending |

#### Kafka Operator (Planned)

Multi-component stateful workload (ZooKeeper + Kafka brokers + topic management).

### Application Platform

*(Planned — tests skill handling of Deployment-based, multi-component operators)*

### Infrastructure / Cloud

*(Planned — tests cluster-scoped CRDs, different-group layout)*

### Network / Security

*(Partially tested within PostgreSQL Scenario C — webhooks, NetworkPolicy, cert-manager integration)*

### Observability

*(Planned — tests ServiceMonitor, PrometheusRule reconciliation)*

### Storage / Data

*(Planned — tests PV/PVC management, CSI integration)*

### ML / AI

*(Planned — tests GPU scheduling, batch Job patterns)*

### CI/CD & GitOps

*(Planned — tests Pipeline/Task CRDs, GitOps sync patterns)*

---

## Summary: Test Matrix

| Sprint | Component | Unit Tests | Integration Tests | Scenario Coverage |
|--------|-----------|-----------|-------------------|-------------------|
| 1 | scaffolding-operator | 1.1, 1.2 | — | A |
| 2 | designing-operator-api | 2.1, 2.2 | I-1.2 | A, B, C, D |
| 3 | implementing-reconciliation | 3.1, 3.2 | I-1.2.3 | A, B, C, D |
| 4 | testing-operator | 4.1, 4.2 | I-1.2.3.4 | A, B, C, D |
| 5 | bundling-operator | 5.1, 5.2 | I-1.2.3.4.5 | A, B, C, D |
| 6 | operator-reviewer | 6.1, 6.2 | I-6 | A, B, C, D |
| 7 | operator-test-generator | 7.1, 7.2 | I-7 | A, B, C, D |
| 8 | operator-bundle-validator | 8.1, 8.2 | I-8 | A, B, C, D |
| Final | All components | — | — | A, B, C, D |

**Total**: 16 unit tests + 7 integration tests + E2E scenarios (PostgreSQL 111 + Redis 56 = 167 OpenShift test conditions) = **190 test points**

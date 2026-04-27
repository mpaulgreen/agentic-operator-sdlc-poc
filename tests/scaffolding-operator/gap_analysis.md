# Sprint 1 Gap Analysis: Skill Scaffold vs operator-sdk Scaffold

Compared skill output against `operator-sdk` v1.37.0 across four patterns in the correct order:
- **Pattern A**: New project (`cache/RedisCluster`)
- **Pattern B**: Same-group kind (`cache/RedisSentinel`)
- **Pattern D**: Same-group cluster-scoped (`cache/ClusterRedisConfig`)
- **Pattern C**: Different-group kind (`monitoring/AlertPolicy`)

SDK created with correct ordering: `init` → `create api cache/RedisCluster` → `create api cache/RedisSentinel` → `create api cache/ClusterRedisConfig --namespaced=false` → `edit --multigroup` → `create api monitoring/AlertPolicy`.

All same-group APIs (namespaced + cluster-scoped) created before enabling multigroup.

---

## Pattern A: New Project Scaffold

All initial gaps (P1/P2/P3) have been resolved. See below for summary.

### Resolved Gaps

| Priority | What | Status |
|----------|------|--------|
| P1 | config/manifests, config/scorecard, config/crd/kustomizeconfig | Fixed |
| P2 | auth proxy RBAC, editor/viewer roles, default patches, prometheus, license headers | Fixed |
| P3 | .dockerignore, .golangci.yml, README.md | Fixed |
| By design | Test files (8) — Sprint 4's `testing-operator` skill | Deferred |

### Where Skill Exceeds SDK

| Aspect | SDK | Skill |
|--------|-----|-------|
| Event recorder | Not set up | Set up with `GetEventRecorderFor` |
| Secure metrics | `false` | `true` (newer pattern) |
| `metrics/filters` import | Absent | Present (needed for secure metrics) |
| `doc.go` | Absent | Present (good practice) |
| `Foo string` example field | Present | Absent (cleaner stubs) |

---

## Pattern B: Same-Group Kind (cache/RedisSentinel)

| Aspect | SDK | Skill | Match? |
|--------|-----|-------|--------|
| Types in `api/v1alpha1/` | YES | YES | MATCH |
| Controller in `internal/controller/` | YES | YES | MATCH |
| Shared package, no aliases | YES | YES | MATCH |
| Editor/viewer roles | YES | YES | MATCH |
| Kustomizations updated | YES | YES | MATCH |

---

## Pattern D: Cluster-Scoped (cache/ClusterRedisConfig)

| Aspect | SDK | Skill | Match? |
|--------|-----|-------|--------|
| `scope=Cluster` marker | YES | YES | MATCH |
| `namespaced: true` absent from PROJECT | YES | YES | MATCH |
| Types in `api/v1alpha1/` (flat) | YES | YES | MATCH |
| Controller in `internal/controller/` (flat) | YES | YES | MATCH |
| Compiles | YES | YES | MATCH |

---

## Pattern C: Different-Group (monitoring/AlertPolicy)

| Aspect | SDK | Skill | Match? |
|--------|-----|-------|--------|
| `api/monitoring/v1alpha1/` directory | YES | YES | MATCH |
| `internal/controller/monitoring/` directory | YES | YES | MATCH |
| `multigroup: true` in PROJECT | YES | YES | MATCH |
| `monitoringv1alpha1` aliased import | YES | YES | MATCH |
| `monitoringcontroller` aliased import | YES | YES | MATCH |
| `monitoringcontroller.AlertPolicyReconciler` registration | YES | YES | MATCH |
| Previous controllers preserved (flat) | YES | YES | MATCH |

---

## Cross-Pattern: Pluralization

Kubernetes pluralization rules (not simple `+s`):
- Ends in `s`: no change (`redis` → `redis`)
- Ends in `y` + consonant: `ies` (`policy` → `policies`)
- Otherwise: `+s` (`rediscluster` → `redisclusters`)

Documented in SKILL.md with user override via `--plural`.

---

## Full Comparison Summary

| Pattern | SDK Compiles | Skill Compiles | Layout Match |
|---------|-------------|----------------|-------------|
| **A** (New project) | YES | YES | YES |
| **B** (Same-group) | YES | YES | YES |
| **D** (Cluster-scoped) | YES | YES | YES |
| **C** (Different-group) | YES | YES | YES |

### Remaining Differences (all acceptable)

| # | Difference | Notes |
|---|-----------|-------|
| 1 | Test files (8) | Sprint 4 |
| 2 | `doc.go` | Skill extra — good practice |
| 3 | Editor/viewer naming | SDK: `redissentinel_editor`. Skill: `cache_redissentinel_editor`. Skill more consistent. |
| 4 | Kustomization entry ordering | Cosmetic |

### SDK Bug: Wrong API Creation Order

If multigroup is enabled BEFORE creating same-group kinds, the SDK produces duplicate import aliases that don't compile. Correct practice: create all same-group APIs first, enable multigroup only when needed. Our skill handles this correctly.

**All gaps resolved. Skill output matches SDK across all four patterns when APIs are created in the correct order.**

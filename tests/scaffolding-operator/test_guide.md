# Sprint 1 Test Guide: `scaffolding-operator` Skill

## Prerequisites

- Go 1.22+ installed (`go version`)
- operator-sdk v1.37.0+ installed (`operator-sdk version`) — required for Test 1.5
- Claude Code with access to this project
- The skill is at `.claude/skills/scaffolding-operator/`

## Test Order

Tests follow operator-sdk best practice — all same-group APIs (namespaced + cluster-scoped) before enabling multigroup:

1. **1.1 (A)**: New project — cache/RedisCluster (namespaced)
2. **1.2 (B)**: Same-group — cache/RedisSentinel (namespaced)
3. **1.3 (D)**: Same-group cluster-scoped — cache/ClusterRedisConfig
4. **1.4 (C)**: Different-group — monitoring/AlertPolicy (triggers multigroup)
5. **1.5**: SDK comparison (same order, both compilable)

---

## Test 1.1 — New Operator Project (Workflow A)

### Step 1: Clean up
```bash
rm -rf /tmp/redis-operator-test
```

### Step 2: Prompt
```
Using the scaffolding-operator skill, create a new OpenShift operator project 
called 'redis-operator' with domain 'redis.example.com' and group 'cache'. 
The operator will manage RedisCluster resources. Generate the full project 
structure under /tmp/redis-operator-test/
```

### Step 3: Verify
```bash
# Structural validation
bash .claude/skills/scaffolding-operator/scripts/validate-project-structure.sh /tmp/redis-operator-test/

# Compilation
cd /tmp/redis-operator-test && go mod tidy && go build -o bin/manager ./cmd/main.go

# Spot checks
grep "projectName: redis-operator" /tmp/redis-operator-test/PROJECT
grep 'cachev1alpha1' /tmp/redis-operator-test/cmd/main.go
grep 'DeepCopyObject' /tmp/redis-operator-test/api/v1alpha1/zz_generated.deepcopy.go
grep 'Copyright' /tmp/redis-operator-test/cmd/main.go > /dev/null && echo "PASS: license" || echo "FAIL"
test -f /tmp/redis-operator-test/hack/boilerplate.go.txt && echo "PASS" || echo "FAIL"
test -f /tmp/redis-operator-test/README.md && echo "PASS" || echo "FAIL"
test -f /tmp/redis-operator-test/.dockerignore && echo "PASS" || echo "FAIL"
test -f /tmp/redis-operator-test/.golangci.yml && echo "PASS" || echo "FAIL"
```

### Acceptance Criteria
- [ ] 48/48 structural checks pass
- [ ] Compiles without errors
- [ ] PROJECT, main.go, Makefile, Dockerfile, config/ all correct
- [ ] hack/boilerplate.go.txt, zz_generated.deepcopy.go, README, .dockerignore, .golangci.yml exist

---

## Test 1.2 — Same-Group Kind (Workflow B Flat)

### Step 1: Ensure Test 1.1 exists

### Step 2: Prompt
```
Using the scaffolding-operator skill, add a new kind 'RedisSentinel' in the 
existing 'cache' API group with version v1alpha1 to the operator project at 
/tmp/redis-operator-test/
```

### Step 3: Verify
```bash
# Flat layout
test -f /tmp/redis-operator-test/api/v1alpha1/redissentinel_types.go && echo "PASS: flat types" || echo "FAIL"
test -f /tmp/redis-operator-test/internal/controller/redissentinel_controller.go && echo "PASS: flat controller" || echo "FAIL"
test ! -d /tmp/redis-operator-test/api/cache && echo "PASS: no api/cache/" || echo "FAIL"

# PROJECT (no multigroup)
grep -c "kind:" /tmp/redis-operator-test/PROJECT  # Expected: 2
! grep "multigroup" /tmp/redis-operator-test/PROJECT && echo "PASS: no multigroup" || echo "INFO"

# main.go uses shared package
grep 'controller.RedisSentinelReconciler' /tmp/redis-operator-test/cmd/main.go && echo "PASS" || echo "FAIL"

# Kustomizations updated
grep "cache_redissentinel_editor" /tmp/redis-operator-test/config/rbac/kustomization.yaml && echo "PASS" || echo "FAIL"
grep "cache_v1alpha1_redissentinel" /tmp/redis-operator-test/config/samples/kustomization.yaml && echo "PASS" || echo "FAIL"
grep "cache.redis.example.com_redissentinels" /tmp/redis-operator-test/config/crd/kustomization.yaml && echo "PASS" || echo "FAIL"

# Compiles
cd /tmp/redis-operator-test && go build -o bin/manager ./cmd/main.go && echo "PASS" || echo "FAIL"
```

### Acceptance Criteria
- [ ] Flat layout — no api/cache/ or controller/cache/ directories
- [ ] No `multigroup` in PROJECT, 2 resources
- [ ] `controller.RedisSentinelReconciler` (shared package, no alias)
- [ ] Editor/viewer roles, sample CR, kustomizations updated
- [ ] Compiles with both controllers

---

## Test 1.3 — Cluster-Scoped Kind (Pattern D)

### Step 1: Ensure Test 1.2 exists

### Step 2: Prompt
```
Using the scaffolding-operator skill, add a new cluster-scoped kind 
'ClusterRedisConfig' in the 'cache' API group with version v1alpha1 
to the operator project at /tmp/redis-operator-test/. This resource 
should be cluster-scoped (not namespaced).
```

### Step 3: Verify
```bash
# scope=Cluster marker
grep 'scope=Cluster' /tmp/redis-operator-test/api/v1alpha1/clusterredisconfig_types.go && echo "PASS" || echo "FAIL"

# PROJECT: namespaced absent for this resource
awk '/kind: ClusterRedisConfig/{found=1} found{print; if(/version:/)exit}' /tmp/redis-operator-test/PROJECT | grep -q "namespaced" && echo "FAIL: namespaced present" || echo "PASS: namespaced absent"

# Flat layout (same cache group)
test -f /tmp/redis-operator-test/api/v1alpha1/clusterredisconfig_types.go && echo "PASS: flat" || echo "FAIL"
grep -c "kind:" /tmp/redis-operator-test/PROJECT  # Expected: 3

# Compiles
cd /tmp/redis-operator-test && go build -o bin/manager ./cmd/main.go && echo "PASS" || echo "FAIL"
```

### Acceptance Criteria
- [ ] `//+kubebuilder:resource:scope=Cluster` marker on root type
- [ ] PROJECT: no `namespaced: true` for ClusterRedisConfig
- [ ] Flat layout (same cache group), 3 resources
- [ ] Editor/viewer roles, sample CR, kustomizations updated
- [ ] Compiles with all 3 controllers

---

## Test 1.4 — Different-Group Kind (Workflow C Multi-Group)

### Step 1: Ensure Test 1.3 exists

### Step 2: Prompt
```
Using the scaffolding-operator skill, add a new API group 'monitoring' with 
version v1alpha1 and kind 'AlertPolicy' to the existing operator project at 
/tmp/redis-operator-test/
```

### Step 3: Verify
```bash
# Multi-group layout
test -d /tmp/redis-operator-test/api/monitoring/v1alpha1 && echo "PASS" || echo "FAIL"
test -d /tmp/redis-operator-test/internal/controller/monitoring && echo "PASS" || echo "FAIL"
test -f /tmp/redis-operator-test/api/monitoring/v1alpha1/groupversion_info.go && echo "PASS" || echo "FAIL"
test -f /tmp/redis-operator-test/api/monitoring/v1alpha1/zz_generated.deepcopy.go && echo "PASS" || echo "FAIL"

# PROJECT
grep "multigroup: true" /tmp/redis-operator-test/PROJECT && echo "PASS" || echo "FAIL"
grep -c "kind:" /tmp/redis-operator-test/PROJECT  # Expected: 4

# Aliased imports
grep 'monitoringv1alpha1' /tmp/redis-operator-test/cmd/main.go && echo "PASS" || echo "FAIL"
grep 'monitoringcontroller' /tmp/redis-operator-test/cmd/main.go && echo "PASS" || echo "FAIL"

# Kustomizations
grep "monitoring_alertpolicy_editor" /tmp/redis-operator-test/config/rbac/kustomization.yaml && echo "PASS" || echo "FAIL"
grep "monitoring.redis.example.com_alertpolicies" /tmp/redis-operator-test/config/crd/kustomization.yaml && echo "PASS" || echo "FAIL"

# Previous code preserved
grep 'controller.RedisClusterReconciler' /tmp/redis-operator-test/cmd/main.go && echo "PASS" || echo "FAIL"
grep 'controller.RedisSentinelReconciler' /tmp/redis-operator-test/cmd/main.go && echo "PASS" || echo "FAIL"
grep 'controller.ClusterRedisConfigReconciler' /tmp/redis-operator-test/cmd/main.go && echo "PASS" || echo "FAIL"

# Compiles
cd /tmp/redis-operator-test && go build -o bin/manager ./cmd/main.go && echo "PASS" || echo "FAIL"
```

### Acceptance Criteria
- [ ] Multi-group layout — api/monitoring/v1alpha1/ and controller/monitoring/
- [ ] `multigroup: true` in PROJECT, 4 resources
- [ ] Aliased imports (monitoringv1alpha1, monitoringcontroller)
- [ ] All 3 previous controllers preserved (flat in shared package)
- [ ] Compiles with all 4 controllers

---

## Test 1.5 — SDK Comparison

### Step 1: Scaffold with operator-sdk (same order)
```bash
rm -rf /tmp/redis-operator-sdk
mkdir -p /tmp/redis-operator-sdk && cd /tmp/redis-operator-sdk

# 1.1: init + RedisCluster
operator-sdk init --domain redis.example.com --repo github.com/example/redis-operator --plugins=go/v4
operator-sdk create api --group cache --version v1alpha1 --kind RedisCluster --resource --controller

# 1.2: RedisSentinel (same group, before multigroup)
operator-sdk create api --group cache --version v1alpha1 --kind RedisSentinel --resource --controller

# 1.3: ClusterRedisConfig (cluster-scoped, same group, before multigroup)
operator-sdk create api --group cache --version v1alpha1 --kind ClusterRedisConfig --resource --controller --namespaced=false

# 1.4: enable multigroup, then AlertPolicy
operator-sdk edit --multigroup
operator-sdk create api --group monitoring --version v1alpha1 --kind AlertPolicy --resource --controller
```

### Step 2: Both compile
```bash
cd /tmp/redis-operator-sdk && go build -o bin/manager ./cmd/main.go && echo "SDK: PASS" || echo "SDK: FAIL"
cd /tmp/redis-operator-test && go build -o bin/manager ./cmd/main.go && echo "SKILL: PASS" || echo "SKILL: FAIL"
```

### Step 3: Compare layouts
```bash
echo "=== API layout ==="
echo "SDK:" && find /tmp/redis-operator-sdk/api -type f -name '*.go' | sed 's|/tmp/redis-operator-sdk/||' | sort
echo "SKILL:" && find /tmp/redis-operator-test/api -type f -name '*.go' | grep -v '.DS_Store' | sed 's|/tmp/redis-operator-test/||' | sort

echo "=== Controller layout (excl tests) ==="
echo "SDK:" && find /tmp/redis-operator-sdk/internal/controller -type f -name '*.go' ! -name '*_test.go' ! -name 'suite_test.go' | sed 's|/tmp/redis-operator-sdk/||' | sort
echo "SKILL:" && find /tmp/redis-operator-test/internal/controller -type f -name '*.go' | sed 's|/tmp/redis-operator-test/||' | sort
```

### Step 4: Compare imports and registrations
```bash
echo "=== Imports ==="
echo "SDK:" && grep -E 'cachev1alpha1|monitoringv1alpha1|controller"|monitoringcontroller' /tmp/redis-operator-sdk/cmd/main.go | sed 's/^[ \t]*/  /'
echo "SKILL:" && grep -E 'cachev1alpha1|monitoringv1alpha1|controller"|monitoringcontroller' /tmp/redis-operator-test/cmd/main.go | sed 's/^[ \t]*/  /'

echo "=== Registrations ==="
echo "SDK:" && grep 'Reconciler{' /tmp/redis-operator-sdk/cmd/main.go | sed 's/^[ \t]*/  /'
echo "SKILL:" && grep 'Reconciler{' /tmp/redis-operator-test/cmd/main.go | sed 's/^[ \t]*/  /'
```

### Step 5: Compare config files
```bash
echo "RBAC roles: SDK=$(grep -cE 'editor|viewer' /tmp/redis-operator-sdk/config/rbac/kustomization.yaml) SKILL=$(grep -cE 'editor|viewer' /tmp/redis-operator-test/config/rbac/kustomization.yaml)"
echo "CRD bases: SDK=$(grep -c 'bases/' /tmp/redis-operator-sdk/config/crd/kustomization.yaml) SKILL=$(grep -c 'bases/' /tmp/redis-operator-test/config/crd/kustomization.yaml)"
echo "Samples: SDK=$(grep -cE '\.yaml$' /tmp/redis-operator-sdk/config/samples/kustomization.yaml) SKILL=$(grep -cE '\.yaml$' /tmp/redis-operator-test/config/samples/kustomization.yaml)"

for f in config/manifests/kustomization.yaml config/scorecard/bases/config.yaml config/crd/kustomizeconfig.yaml config/default/manager_auth_proxy_patch.yaml config/rbac/auth_proxy_service.yaml config/prometheus/monitor.yaml .dockerignore .golangci.yml README.md; do
  sdk=$(test -f /tmp/redis-operator-sdk/$f && echo "Y" || echo "N")
  skill=$(test -f /tmp/redis-operator-test/$f && echo "Y" || echo "N")
  printf "  %-50s SDK:%-1s SKILL:%-1s\n" "$f" "$sdk" "$skill"
done
```

### Expected Differences

| Aspect | SDK | Skill | Why |
|--------|-----|-------|-----|
| Test files (8) | Present | Absent | Sprint 4 |
| `doc.go` | Absent | Present | Skill extra — good practice |
| Editor/viewer naming | `redissentinel_editor` | `cache_redissentinel_editor` | Skill always prefixes group |
| Event recorder | Not set up | Set up | Skill is better |
| Secure metrics | `false` | `true` | Skill uses newer pattern |
| CRD base YAMLs | Empty (`config/crd/bases/`) | Generated during scaffold | SDK needs `make manifests` to populate; skill generates them at scaffold time so the project is self-contained immediately |

### Acceptance Criteria
- [ ] Both compile
- [ ] API and controller layouts match (excl test files and doc.go)
- [ ] Import aliases match
- [ ] Controller registrations match (4 each)
- [ ] Config files match (RBAC=8, CRD=4, samples=4)
- [ ] Only expected differences remain

---

## Cleanup
```bash
rm -rf /tmp/redis-operator-test /tmp/redis-operator-sdk
```

## Troubleshooting

| Issue | Cause | Fix |
|-------|-------|-----|
| `go build` fails: "missing DeepCopyObject" | zz_generated.deepcopy.go missing | Generate DeepCopy methods |
| `make generate` fails: "headerFile not found" | hack/boilerplate.go.txt missing | Create license header file |
| SDK compile error: duplicate import | APIs created in wrong order | Create same-group APIs before `operator-sdk edit --multigroup` |
| `scope=Cluster` missing | NAMESPACED not set to false | Add `//+kubebuilder:resource:scope=Cluster` marker |

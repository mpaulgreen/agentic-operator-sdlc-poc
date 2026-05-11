# E2E Validation Guide: PostgreSQL Operator on OpenShift

End-to-end validation of the PostgreSQL operator on a live OpenShift cluster. This guide covers multiple scenarios that build progressively on the same operator.

| Scenario | Feature | Bundle Version | Skills Tested | New Resource |
|----------|---------|----------------|---------------|-------------|
| A | Core operator (from scratch) | v0.1.0 | All 5 + all 3 subagents | StatefulSet, Service, Secret, ConfigMap, CronJob |
| B | High Availability | v0.2.0 | 4 (Workflow B) + 3 subagents | PodDisruptionBudget (policy/v1) |
| C | Webhooks + Network Security | v0.3.0 | 4 (Workflow C) + 3 subagents | NetworkPolicy |
| D | API Maturity + Connection Pooling | v0.4.0 | 4 (Workflow D) + 3 subagents | Deployment |

**Run scenarios in order** — each builds on the previous. Complete all phases of Scenario A before starting Scenario B.

---

# Scenario A: Core Operator (v0.1.0)

The operator was built using the mandatory workflow from CLAUDE.md:
- **Steps 1-3** (Generate): scaffolding-operator, designing-operator-api, implementing-reconciliation **SKILLS**
- **Step 4a** (Test): operator-test-generator **SUBAGENT** (Discovery → Generate → Validate → Report)
- **Step 4b** (Review): operator-reviewer **SUBAGENT** (Read → Scripts → Inspect → Report)
- **Step 5** (Generate): bundling-operator **SKILL**
- **Step 6** (Validate): operator-bundle-validator **SUBAGENT** (Validate → Certification → Report)

**Project stats**: 55 files, 5 reconciler methods, 16 test cases, 9 RBAC markers, all 9 validation scripts pass.

## Prerequisites

- OpenShift 4.14+ cluster with cluster-admin access
- `oc` CLI logged in (`oc whoami` returns a user)
- `podman` or `docker` for building images
- Access to a container registry (quay.io, or internal registry)
- A default StorageClass available (`oc get sc`)

## Environment Setup

```bash
# Set your registry and image
export IMG=quay.io/mpaulgreen/postgres-operator:v0.1.0
export BUNDLE_IMG=quay.io/mpaulgreen/postgres-operator-bundle:v0.1.0
export NAMESPACE=postgres-operator-system

# Navigate to the operator project
cd e2e/postgres-operator
```

---

## Phase 1: Build and Deploy

### 1.1 Build the Operator Image

```bash
# Build for linux/amd64 (required — OpenShift nodes run linux/amd64,
# building without --platform on macOS produces a darwin binary
# that fails with "Exec format error" on the cluster)
podman build --platform linux/amd64 -t $IMG .

# Push
podman push $IMG

# Verify
podman inspect $IMG | grep -i architecture
```

**Expected**: Image builds and pushes successfully.

### 1.2 Deploy the Operator

Choose **one** of the two deployment paths below. Both handle CRD installation, namespace creation, RBAC (ServiceAccount, ClusterRole, ClusterRoleBinding), and Deployment automatically — you do NOT create these manually.

#### Option A: `make deploy` (Development)

This uses kustomize to generate and apply all manifests from the `config/` directory. The RBAC is auto-generated from the `//+kubebuilder:rbac` markers in the controller code.

```bash
# Generate manifests (CRDs, RBAC roles from markers)
make manifests

# Deploy (creates namespace, CRD, RBAC, ServiceAccount, Deployment)
make deploy IMG=$IMG

# Verify
oc get pods -n $NAMESPACE -l control-plane=controller-manager
oc logs -n $NAMESPACE -l control-plane=controller-manager --tail=20
```


#### Option B: OLM (Production / OperatorHub)

This uses the OLM bundle we already generated. OLM reads the CSV's `spec.install.spec.clusterPermissions` and creates the RBAC automatically.

**Important**:
1. Before building the bundle image, update the CSV's container image to match your registry: `sed -i '' "s|quay.io/example/postgres-operator|$IMG|g" bundle/manifests/postgres-operator.clusterserviceversion.yaml`
2. A CatalogSource requires a **catalog/index image** (built with `opm`), NOT a raw bundle image. The bundle image only contains YAML — OLM can't query it as a catalog. Use one of these approaches:

**Option B1: `operator-sdk run bundle` (simplest)**

This handles catalog creation, OperatorGroup, and Subscription automatically:

```bash
# Build and push the bundle image
podman build -t $BUNDLE_IMG -f bundle.Dockerfile .
podman push $BUNDLE_IMG

# Deploy via OLM (creates catalog, subscription, and installs the operator)
oc new-project $NAMESPACE
operator-sdk run bundle $BUNDLE_IMG --namespace $NAMESPACE --timeout 5m
```

**Option B2: Using Makefile + `opm` (production)**

Requires `opm` CLI (auto-downloaded by `make opm`).

```bash
# Build and push the bundle image
make bundle-build bundle-push BUNDLE_IMG=$BUNDLE_IMG

# Generate FBC catalog, build and push catalog image
export CATALOG_IMG=quay.io/mpaulgreen/postgres-operator-catalog:latest
make catalog-render BUNDLE_IMG=$BUNDLE_IMG
make catalog-build catalog-push CATALOG_IMG=$CATALOG_IMG

# Create CatalogSource pointing to the CATALOG image (not the bundle image)
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: postgres-operator-catalog
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: $CATALOG_IMG
  displayName: PostgreSQL Operator
  publisher: Example Inc
EOF

oc wait --for=condition=ready catalogsource/postgres-operator-catalog -n openshift-marketplace --timeout=120s

# Create OperatorGroup + Subscription
oc new-project $NAMESPACE

cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: postgres-operator-group
  namespace: $NAMESPACE
spec:
  targetNamespaces:
  - $NAMESPACE
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: postgres-operator
  namespace: $NAMESPACE
spec:
  channel: alpha
  name: postgres-operator
  source: postgres-operator-catalog
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
EOF

oc wait --for=condition=available deployment/postgres-operator-controller-manager -n $NAMESPACE --timeout=120s
```

#### Verify Deployment (either path)

```bash
# Operator pod running
oc get pods -n $NAMESPACE -l control-plane=controller-manager
oc logs -n $NAMESPACE -l control-plane=controller-manager --tail=20

# RBAC was created automatically
oc get clusterrole | grep postgres-operator
oc get serviceaccount -n $NAMESPACE | grep postgres-operator

# CRD installed
oc get crd postgresclusters.database.postgres.example.com
```

**Expected**: Operator pod is Running, logs show "starting manager", RBAC and CRD created automatically.

---

## Phase 2: Basic CR Lifecycle

### 2.1 Create a Minimal PostgresCluster

```bash
cat <<EOF | oc apply -f -
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-test
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "16"
  storage:
    size: 1Gi
EOF

# Watch the operator reconcile
oc logs -n $NAMESPACE -l control-plane=controller-manager -f &
sleep 10
```

**Verify** — all 4 managed resources created:

```bash
echo "=== Managed Resources ==="
oc get secret pg-test-credentials -n $NAMESPACE && echo "PASS: Secret" || echo "FAIL: Secret"
oc get configmap pg-test-config -n $NAMESPACE && echo "PASS: ConfigMap" || echo "FAIL: ConfigMap"
oc get service pg-test-headless -n $NAMESPACE && echo "PASS: Service" || echo "FAIL: Service"
oc get statefulset pg-test -n $NAMESPACE && echo "PASS: StatefulSet" || echo "FAIL: StatefulSet"

echo ""
echo "=== Owner References ==="
oc get secret pg-test-credentials -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be PostgresCluster)"
oc get configmap pg-test-config -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be PostgresCluster)"
oc get service pg-test-headless -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be PostgresCluster)"
oc get statefulset pg-test -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be PostgresCluster)"

echo ""
echo "=== CR Status ==="
oc get postgrescluster pg-test -n $NAMESPACE -o wide
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.status.phase}' && echo ""
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.status.endpoint}' && echo ""
```

**Expected**:
- [ ] Secret `pg-test-credentials` created with `POSTGRESQL_PASSWORD` key
- [ ] ConfigMap `pg-test-config` created with `postgresql.conf`
- [ ] Service `pg-test-headless` created (ClusterIP: None, port 5432)
- [ ] StatefulSet `pg-test` created with 3 replicas, image `postgres:16`
- [ ] All resources have `ownerReferences` pointing to `PostgresCluster`
- [ ] Status shows phase (Initializing or Running depending on pod readiness)

### 2.2 Verify StatefulSet Details

```bash
echo "=== StatefulSet Spec ==="
oc get statefulset pg-test -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas"
oc get statefulset pg-test -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].image}' && echo " image"
oc get statefulset pg-test -n $NAMESPACE -o jsonpath='{.spec.volumeClaimTemplates[0].spec.resources.requests.storage}' && echo " storage"

echo ""
echo "=== Pod Status ==="
oc get pods -n $NAMESPACE -l app.kubernetes.io/instance=pg-test
```

**Expected**:
- [ ] 3 replicas
- [ ] Image is `postgres:16`
- [ ] PVC requests 1Gi storage
- [ ] Pods eventually reach Running state (may take 1-2 min)

### 2.3 Verify ConfigMap Content

```bash
oc get configmap pg-test-config -n $NAMESPACE -o jsonpath='{.data.postgresql\.conf}'
```

**Expected**: Contains `shared_buffers = 256MB`, `max_connections = 100`, `wal_level = replica`.

### 2.4 Verify Secret Content

```bash
# Secret should have POSTGRESQL_PASSWORD (base64-encoded)
oc get secret pg-test-credentials -n $NAMESPACE -o jsonpath='{.data.POSTGRESQL_PASSWORD}' | base64 -d && echo ""
oc get secret pg-test-credentials -n $NAMESPACE -o jsonpath='{.data.POSTGRESQL_USER}' | base64 -d && echo ""
```

**Expected**: 16-character random password, user is `postgres`.

### 2.5 Verify Service

```bash
oc get service pg-test-headless -n $NAMESPACE -o yaml | grep -A5 'spec:'
```

**Expected**: `clusterIP: None`, port 5432/TCP.

### 2.6 Wait for Running Phase

```bash
# Wait for pods to become ready (may take 2-5 min depending on storage)
oc wait --for=condition=ready pod -l app.kubernetes.io/instance=pg-test -n $NAMESPACE --timeout=300s

# Check status
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.status}' | python3 -m json.tool
```

**Expected**:
- [ ] `phase: Running` (once all replicas ready)
- [ ] `readyReplicas: 3`
- [ ] `currentVersion: "16"`
- [ ] `endpoint: pg-test-headless.$NAMESPACE.svc.cluster.local:5432`

### 2.7 Verify Conditions

```bash
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -m json.tool
```

**Expected conditions**:
- [ ] `Available: True` (reason: ResourcesReady)
- [ ] `Progressing: False` (reason: ReconcileSuccess)
- [ ] `Degraded: False` (reason: ReconcileSuccess)
- [ ] `BackupReady: False` (reason: BackupDisabled — no backup configured)

### 2.8 Verify Print Columns

```bash
oc get postgrescluster -n $NAMESPACE
```

**Expected**: Table with columns Phase, Ready, Version, Age.

### 2.9 Verify Events

```bash
oc get events -n $NAMESPACE --field-selector involvedObject.name=pg-test --sort-by='.lastTimestamp'
```

**Expected**: Events for SecretCreated, ConfigMapCreated, ServiceCreated, StatefulSetCreated.

---

## Phase 3: Idempotency

### 3.1 Verify No Duplicate Resources

```bash
# Delete the operator pod to force re-reconciliation
oc delete pod -n $NAMESPACE -l control-plane=controller-manager
oc wait --for=condition=available deployment/postgres-operator-controller-manager -n $NAMESPACE --timeout=60s

# Wait for reconciliation
sleep 15

# Verify resources are unchanged (no duplicates, no errors)
oc get secret -n $NAMESPACE | grep pg-test
oc get configmap -n $NAMESPACE | grep pg-test
oc get service -n $NAMESPACE | grep pg-test
oc get statefulset -n $NAMESPACE | grep pg-test
```

**Expected**: Exactly 1 of each resource, no duplicates, no errors in operator logs.

### 3.2 Verify Password Unchanged After Reconciliation

```bash
# Save current password
PASS_BEFORE=$(oc get secret pg-test-credentials -n $NAMESPACE -o jsonpath='{.data.POSTGRESQL_PASSWORD}')

# Trigger reconciliation by updating a label
oc label postgrescluster pg-test -n $NAMESPACE test-reconcile=true

sleep 10

# Check password unchanged
PASS_AFTER=$(oc get secret pg-test-credentials -n $NAMESPACE -o jsonpath='{.data.POSTGRESQL_PASSWORD}')
[ "$PASS_BEFORE" = "$PASS_AFTER" ] && echo "PASS: Password unchanged (idempotent)" || echo "FAIL: Password changed!"
```

---

## Phase 4: Scaling

### 4.1 Scale Up

```bash
oc patch postgrescluster pg-test -n $NAMESPACE --type merge -p '{"spec":{"replicas":5}}'
sleep 15

oc get statefulset pg-test -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas (should be 5)"
oc get pods -n $NAMESPACE -l app.kubernetes.io/instance=pg-test
```

**Expected**:
- [ ] StatefulSet replicas updated to 5
- [ ] 5 pods eventually Running
- [ ] Status `readyReplicas` reaches 5

### 4.2 Scale Down

```bash
oc patch postgrescluster pg-test -n $NAMESPACE --type merge -p '{"spec":{"replicas":1}}'
sleep 15

oc get statefulset pg-test -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas (should be 1)"
oc get pods -n $NAMESPACE -l app.kubernetes.io/instance=pg-test
```

**Expected**:
- [ ] StatefulSet replicas updated to 1
- [ ] Extra pods terminated

---

## Phase 5: Backup (CronJob)

### 5.1 Enable Backup

```bash
oc patch postgrescluster pg-test -n $NAMESPACE --type merge -p '{"spec":{"backup":{"enabled":true,"schedule":"*/5 * * * *","retentionDays":7}}}'
sleep 15

oc get cronjob -n $NAMESPACE | grep pg-test
oc get cronjob pg-test-backup -n $NAMESPACE -o yaml | grep schedule
```

**Expected**:
- [ ] CronJob `pg-test-backup` created
- [ ] Schedule is `*/5 * * * *`
- [ ] CronJob has ownerReferences to PostgresCluster
- [ ] BackupReady condition is True

### 5.2 Verify BackupReady Condition

```bash
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'BackupReady':
        print(f\"BackupReady: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**: `BackupReady: True (reason: BackupConfigured)`

### 5.3 Disable Backup

```bash
oc patch postgrescluster pg-test -n $NAMESPACE --type merge -p '{"spec":{"backup":{"enabled":false}}}'
sleep 15

oc get cronjob -n $NAMESPACE | grep pg-test || echo "CronJob removed (expected if using owner refs)"
```

**Expected**: CronJob behavior depends on whether the reconciler deletes it when disabled (current implementation keeps it if it exists). BackupReady condition should be False.

---

## Phase 6: Finalizer and Deletion

### 6.1 Verify Finalizer Exists

```bash
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.metadata.finalizers}' && echo ""
```

**Expected**: `["database.postgres.example.com/finalizer"]`

### 6.2 Delete the CR

```bash
oc delete postgrescluster pg-test -n $NAMESPACE

# Wait a moment for cleanup
sleep 10

# Verify all managed resources are garbage collected
oc get secret pg-test-credentials -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Secret cleaned up"
oc get configmap pg-test-config -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: ConfigMap cleaned up"
oc get service pg-test-headless -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Service cleaned up"
oc get statefulset pg-test -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: StatefulSet cleaned up"
oc get cronjob pg-test-backup -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: CronJob cleaned up"
```

**Expected**:
- [ ] CR deletion succeeds (not stuck on finalizer)
- [ ] All managed resources garbage collected via owner references
- [ ] Events show "Deleting" and "Deleted"
- [ ] No orphaned resources remain

---

## Phase 7: Validation Markers

### 7.1 Reject Invalid Replicas

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-invalid
  namespace: $NAMESPACE
spec:
  replicas: 10
  version: "16"
  storage:
    size: 1Gi
EOF
```

**Expected**: Rejected with validation error — `replicas` max is 5.

### 7.2 Reject Invalid Version

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-invalid
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "13"
  storage:
    size: 1Gi
EOF
```

**Expected**: Rejected — version enum only allows 14, 15, 16.

### 7.3 Reject Invalid Storage Size Pattern

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-invalid
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "16"
  storage:
    size: "100MB"
EOF
```

**Expected**: Rejected — storage size must match pattern `^[0-9]+[KMGT]i$`.

### 7.4 Verify Defaults Applied

```bash
cat <<EOF | oc apply -f -
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-defaults
  namespace: $NAMESPACE
spec:
  storage:
    size: 5Gi
EOF

sleep 5
oc get postgrescluster pg-defaults -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " (should be 3)"
oc get postgrescluster pg-defaults -n $NAMESPACE -o jsonpath='{.spec.version}' && echo " (should be 16)"

# Cleanup
oc delete postgrescluster pg-defaults -n $NAMESPACE
```

**Expected**: Defaults applied — replicas=3, version="16".

---

## Phase 8: RBAC Verification

### 8.1 Verify Operator RBAC Works

```bash
# Check the operator SA can access all needed resources
oc auth can-i get secrets --as=system:serviceaccount:$NAMESPACE:postgres-operator-controller-manager && echo "PASS" || echo "FAIL"
oc auth can-i create statefulsets --as=system:serviceaccount:$NAMESPACE:postgres-operator-controller-manager && echo "PASS" || echo "FAIL"
oc auth can-i create cronjobs --as=system:serviceaccount:$NAMESPACE:postgres-operator-controller-manager && echo "PASS" || echo "FAIL"
oc auth can-i update postgresclusters/status --as=system:serviceaccount:$NAMESPACE:postgres-operator-controller-manager && echo "PASS" || echo "FAIL"
```

### 8.2 Verify No Excess Permissions

```bash
# Should NOT have cluster-admin-level permissions
oc auth can-i create namespaces --as=system:serviceaccount:$NAMESPACE:postgres-operator-controller-manager && echo "FAIL: excess perms" || echo "PASS: no excess"
oc auth can-i delete nodes --as=system:serviceaccount:$NAMESPACE:postgres-operator-controller-manager && echo "FAIL: excess perms" || echo "PASS: no excess"
```

---

## Phase 9: Security Posture

### 9.1 Verify Non-Root Container

```bash
oc get deployment postgres-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.securityContext.runAsNonRoot}' && echo " (should be true)"
```

### 9.2 Verify Capabilities Dropped

```bash
oc get deployment postgres-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].securityContext.capabilities.drop}' && echo ""
```

**Expected**: `["ALL"]`

### 9.3 Verify No Privileged Containers

```bash
oc get deployment postgres-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].securityContext.privileged}' || echo "not set (good)"
```

---

## Phase 10: Health Probes

### 10.1 Verify Liveness Probe

```bash
oc get deployment postgres-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].livenessProbe}' | python3 -m json.tool
```

**Expected**: httpGet on /healthz:8081

### 10.2 Verify Readiness Probe

```bash
oc get deployment postgres-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].readinessProbe}' | python3 -m json.tool
```

**Expected**: httpGet on /readyz:8081

---

## Phase 11: OLM Bundle Validation (Optional)

If operator-sdk is available on the cluster:

### 11.1 Build and Push Bundle Image

```bash
podman build -t $BUNDLE_IMG -f bundle.Dockerfile .
podman push $BUNDLE_IMG
```

### 11.2 Run Scorecard

```bash
operator-sdk scorecard $BUNDLE_IMG --kubeconfig ~/.kube/config --namespace $NAMESPACE
```

**Expected**: All 6 scorecard tests pass (basic-check-spec, olm-bundle-validation, olm-crds-have-validation, olm-crds-have-resources, olm-spec-descriptors, olm-status-descriptors).

### 11.3 Bundle Validate

```bash
operator-sdk bundle validate bundle/
```

**Expected**: No errors.

---

## Phase 12: Multi-Instance

### 12.1 Create Multiple PostgresClusters

```bash
for i in 1 2 3; do
cat <<EOF | oc apply -f -
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-multi-$i
  namespace: $NAMESPACE
spec:
  replicas: 1
  version: "16"
  storage:
    size: 1Gi
EOF
done

sleep 30
oc get postgrescluster -n $NAMESPACE
```

**Expected**:
- [ ] 3 independent PostgresClusters created
- [ ] Each has its own Secret, ConfigMap, Service, StatefulSet
- [ ] No cross-contamination between instances

### 12.2 Delete One, Others Unaffected

```bash
oc delete postgrescluster pg-multi-2 -n $NAMESPACE
sleep 10

oc get postgrescluster -n $NAMESPACE
oc get statefulset -n $NAMESPACE
```

**Expected**: pg-multi-1 and pg-multi-3 still Running, pg-multi-2 fully cleaned up.

---

## Scenario A Cleanup

```bash
# Delete all test CRs (keep operator deployed for Scenario B)
oc delete postgrescluster --all -n $NAMESPACE
sleep 15

# If NOT continuing to Scenario B, undeploy the operator:
# make undeploy                                                    # if deployed with make deploy
# operator-sdk cleanup postgres-operator --namespace $NAMESPACE    # if deployed with OLM
# oc delete project $NAMESPACE
```

---

## Scenario A Summary Checklist

| # | Test | Phase | Expected |
|---|------|-------|----------|
| 1 | Operator deploys and starts | 1 | Pod Running, logs clean |
| 2 | CR creates all managed resources | 2.1 | Secret, ConfigMap, Service, StatefulSet |
| 3 | Owner references set on all resources | 2.1 | All point to PostgresCluster |
| 4 | StatefulSet has correct image and replicas | 2.2 | postgres:16, 3 replicas |
| 5 | ConfigMap has postgresql.conf content | 2.3 | shared_buffers, wal_level |
| 6 | Secret has generated password | 2.4 | 16-char random, user=postgres |
| 7 | Service is headless on port 5432 | 2.5 | ClusterIP: None |
| 8 | Status reaches Running phase | 2.6 | phase=Running, readyReplicas=3 |
| 9 | Conditions set correctly | 2.7 | Available=True, Degraded=False |
| 10 | Print columns display in kubectl/oc | 2.8 | Phase, Ready, Version, Age |
| 11 | Events recorded | 2.9 | Create events for each resource |
| 12 | Idempotent — no duplicates on re-reconcile | 3.1 | Exactly 1 of each resource |
| 13 | Password unchanged on re-reconcile | 3.2 | Same base64 value |
| 14 | Scale up works | 4.1 | 5 replicas |
| 15 | Scale down works | 4.2 | 1 replica |
| 16 | Backup CronJob created when enabled | 5.1 | CronJob with schedule |
| 17 | BackupReady condition reflects state | 5.2 | True when enabled |
| 18 | Finalizer present on CR | 6.1 | Finalizer in metadata |
| 19 | Deletion cleans up all resources | 6.2 | No orphans |
| 20 | Invalid replicas rejected | 7.1 | Validation error |
| 21 | Invalid version rejected | 7.2 | Validation error |
| 22 | Invalid storage pattern rejected | 7.3 | Validation error |
| 23 | Defaults applied when omitted | 7.4 | replicas=3, version=16 |
| 24 | RBAC allows needed operations | 8.1 | can-i returns yes |
| 25 | No excess RBAC permissions | 8.2 | can-i returns no |
| 26 | Container runs as non-root | 9.1 | runAsNonRoot=true |
| 27 | Capabilities dropped | 9.2 | drop=[ALL] |
| 28 | Health probes configured | 10 | /healthz, /readyz |
| 29 | Scorecard passes (optional) | 11 | 6/6 tests pass |
| 30 | Multiple instances independent | 12.1 | No cross-contamination |
| 31 | Deleting one doesn't affect others | 12.2 | Others still Running |

---
---

# Scenario B: High Availability (v0.2.0)

Adds PodDisruptionBudget (policy/v1) + pod anti-affinity to the postgres-operator. Built using:
- **Step 1** (Generate): `designing-operator-api` SKILL (Workflow B) — Added HASpec to types
- **Step 2** (Generate): `implementing-reconciliation` SKILL (Workflow B) — Added reconcilePodDisruptionBudget + anti-affinity
- **Step 3a** (Test): `operator-test-generator` SUBAGENT (Workflow B) — Added PDB tests
- **Step 3b** (Review): `operator-reviewer` SUBAGENT — Reviewed modified code (0 Critical)
- **Step 4** (Generate): `bundling-operator` SKILL (Workflow B) — Updated CSV v0.1.0 → v0.2.0
- **Step 5** (Validate): `operator-bundle-validator` SUBAGENT — Validated updated bundle

**Changes**: HASpec (minAvailable, maxUnavailable, antiAffinityMode), HAReady condition, reconcilePodDisruptionBudget, pod anti-affinity on StatefulSet, CSV v0.2.0 with replaces.

**Prerequisites**: Scenario A completed successfully. All Scenario A CRs deleted (operator may remain deployed).

## Scenario B Environment Setup

```bash
export IMG=quay.io/mpaulgreen/postgres-operator:v0.2.0
export BUNDLE_IMG=quay.io/mpaulgreen/postgres-operator-bundle:v0.2.0
export NAMESPACE=postgres-operator-system

cd e2e/postgres-operator
```

---

## Phase B.1: Build and Deploy v0.2.0

### B.1.1 Build the Operator Image

```bash
podman build --platform linux/amd64 -t $IMG .
podman push $IMG
```

### B.1.2 Deploy the Operator

#### Option A: `make deploy` (Development)

```bash
make manifests
make deploy IMG=$IMG
```

#### Option B: OLM Upgrade

```bash
# Update CSV image reference
sed -i '' "s|quay.io/mpaulgreen/postgres-operator:v0.2.0|$IMG|g" bundle/manifests/postgres-operator.clusterserviceversion.yaml

# Refresh CRD in bundle (types changed — bundle has its own CRD copy)
make manifests
cp config/crd/bases/database.postgres.example.com_postgresclusters.yaml bundle/manifests/

# Build and push bundle
podman build -t $BUNDLE_IMG -f bundle.Dockerfile .
podman push $BUNDLE_IMG

# Create namespace first (operator-sdk run bundle requires it to exist)
oc new-project $NAMESPACE || oc create namespace $NAMESPACE

# Deploy via OLM
operator-sdk run bundle $BUNDLE_IMG --namespace $NAMESPACE --timeout 5m
```

### B.1.3 Verify Deployment

```bash
oc get pods -n $NAMESPACE -l control-plane=controller-manager
oc logs -n $NAMESPACE -l control-plane=controller-manager --tail=20

# Verify CRD has HA fields
oc get crd postgresclusters.database.postgres.example.com -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.ha}' | python3 -m json.tool
```

**Expected**:
- [ ] Pod 1/1 Running with v0.2.0 image
- [ ] CRD includes `ha` field with `minAvailable`, `maxUnavailable`, `antiAffinityMode` properties
- [ ] Controller logs show "starting manager"

---

## Phase B.2: Existing Features Regression

Verify that all Scenario A features still work with the v0.2.0 operator.

### B.2.1 Create CR Without HA

```bash
cat <<EOF | oc apply -f -
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-test
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "16"
  storage:
    size: 1Gi
EOF

sleep 30
```

### B.2.2 Verify All Scenario A Resources Created

```bash
echo "=== Managed Resources ==="
oc get secret pg-test-credentials -n $NAMESPACE && echo "PASS: Secret" || echo "FAIL: Secret"
oc get configmap pg-test-config -n $NAMESPACE && echo "PASS: ConfigMap" || echo "FAIL: ConfigMap"
oc get service pg-test-headless -n $NAMESPACE && echo "PASS: Service" || echo "FAIL: Service"
oc get statefulset pg-test -n $NAMESPACE && echo "PASS: StatefulSet" || echo "FAIL: StatefulSet"

echo ""
echo "=== No PDB (HA not configured) ==="
oc get pdb pg-test-pdb -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: No PDB (correct)" || echo "FAIL: PDB exists unexpectedly"

echo ""
echo "=== Status ==="
oc get postgrescluster pg-test -n $NAMESPACE -o wide
```

**Expected**:
- [ ] All 4 existing resources created (Secret, ConfigMap, Service, StatefulSet)
- [ ] No PDB created (HA not configured)
- [ ] Status shows phase (Initializing or Running)

### B.2.3 Verify No HAReady Condition When HA Not Configured

```bash
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
ha_found = False
for c in conditions:
    if c['type'] == 'HAReady':
        ha_found = True
        print(f\"HAReady: {c['status']} (reason: {c['reason']})\")
if not ha_found:
    print('HAReady condition not present (correct — HA not configured)')
"
```

**Expected**: HAReady condition either not present or False with reason HANotConfigured.

---

## Phase B.3: PodDisruptionBudget — minAvailable

### B.3.1 Enable HA with minAvailable

```bash
oc patch postgrescluster pg-test -n $NAMESPACE --type merge -p '{"spec":{"ha":{"minAvailable":2,"antiAffinityMode":"preferred"}}}'
sleep 15
```

### B.3.2 Verify PDB Created

```bash
echo "=== PDB ==="
oc get pdb pg-test-pdb -n $NAMESPACE
oc get pdb pg-test-pdb -n $NAMESPACE -o jsonpath='{.spec.minAvailable}' && echo " minAvailable (should be 2)"
oc get pdb pg-test-pdb -n $NAMESPACE -o jsonpath='{.spec.maxUnavailable}' && echo " maxUnavailable (should be empty)"

echo ""
echo "=== PDB Owner Reference ==="
oc get pdb pg-test-pdb -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be PostgresCluster)"

echo ""
echo "=== PDB Labels ==="
oc get pdb pg-test-pdb -n $NAMESPACE -o jsonpath='{.metadata.labels}' | python3 -m json.tool

echo ""
echo "=== PDB Selector ==="
oc get pdb pg-test-pdb -n $NAMESPACE -o jsonpath='{.spec.selector.matchLabels}' | python3 -m json.tool
```

**Expected**:
- [ ] PDB `pg-test-pdb` created
- [ ] `minAvailable: 2`
- [ ] `maxUnavailable` not set
- [ ] Owner reference points to PostgresCluster
- [ ] Labels match `labelsForPostgresCluster()`
- [ ] Selector matches StatefulSet pod labels

### B.3.3 Verify HAReady Condition

```bash
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'HAReady':
        print(f\"HAReady: {c['status']} (reason: {c['reason']}, message: {c['message']})\")
"
```

**Expected**: `HAReady: True (reason: HAConfigured, message: PodDisruptionBudget is configured)`

### B.3.4 Verify Existing Resources Unaffected

```bash
oc get secret pg-test-credentials -n $NAMESPACE && echo "PASS: Secret still exists"
oc get statefulset pg-test -n $NAMESPACE && echo "PASS: StatefulSet still exists"
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.status.phase}' && echo " (should still be Running or Initializing)"
```

**Expected**: All existing resources unchanged after enabling HA.

---

## Phase B.4: PodDisruptionBudget — maxUnavailable

### B.4.1 Switch to maxUnavailable

```bash
oc patch postgrescluster pg-test -n $NAMESPACE --type merge -p '{"spec":{"ha":{"minAvailable":null,"maxUnavailable":1,"antiAffinityMode":"preferred"}}}'
sleep 15
```

### B.4.2 Verify PDB Updated

```bash
echo "=== PDB After Update ==="
oc get pdb pg-test-pdb -n $NAMESPACE -o jsonpath='{.spec.maxUnavailable}' && echo " maxUnavailable (should be 1)"
oc get pdb pg-test-pdb -n $NAMESPACE -o jsonpath='{.spec.minAvailable}' && echo " minAvailable (should be empty)"
```

**Expected**:
- [ ] PDB updated (not recreated — same name)
- [ ] `maxUnavailable: 1`
- [ ] `minAvailable` not set (cleared)

---

## Phase B.5: Pod Anti-Affinity

### B.5.1 Verify Preferred Anti-Affinity

```bash
oc get statefulset pg-test -n $NAMESPACE -o jsonpath='{.spec.template.spec.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution}' | python3 -m json.tool
```

**Expected**:
- [ ] `preferredDuringSchedulingIgnoredDuringExecution` present
- [ ] Weight is 100
- [ ] `topologyKey: kubernetes.io/hostname`
- [ ] `labelSelector` uses `app.kubernetes.io/instance: pg-test`

### B.5.2 Switch to Required Anti-Affinity

```bash
oc patch postgrescluster pg-test -n $NAMESPACE --type merge -p '{"spec":{"ha":{"antiAffinityMode":"required"}}}'
sleep 15

oc get statefulset pg-test -n $NAMESPACE -o jsonpath='{.spec.template.spec.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution}' | python3 -m json.tool
```

**Expected**:
- [ ] `requiredDuringSchedulingIgnoredDuringExecution` present
- [ ] `topologyKey: kubernetes.io/hostname`
- [ ] `labelSelector` uses `app.kubernetes.io/instance: pg-test`

**Note**: With required anti-affinity on a single-node cluster, pods beyond the first may stay Pending (this is correct behavior — anti-affinity cannot be satisfied). On a multi-node cluster, pods spread across nodes.

---

## Phase B.6: PDB Default Behavior

### B.6.1 HA Without minAvailable or maxUnavailable

```bash
# Set HA with only antiAffinityMode, no disruption budget values
oc patch postgrescluster pg-test -n $NAMESPACE --type merge -p '{"spec":{"ha":{"minAvailable":null,"maxUnavailable":null,"antiAffinityMode":"preferred"}}}'
sleep 15

echo "=== PDB Default ==="
oc get pdb pg-test-pdb -n $NAMESPACE -o jsonpath='{.spec.minAvailable}' && echo " minAvailable (should be replicas-1)"
```

**Expected**:
- [ ] PDB uses default `minAvailable = replicas - 1` (i.e., 2 when replicas=3)
- [ ] HAReady condition is True

---

## Phase B.7: Idempotency

### B.7.1 Re-reconcile With HA Enabled

```bash
# Delete operator pod to force re-reconciliation
oc delete pod -n $NAMESPACE -l control-plane=controller-manager
oc wait --for=condition=available deployment/postgres-operator-controller-manager -n $NAMESPACE --timeout=60s
sleep 15

echo "PDBs: $(oc get pdb -n $NAMESPACE 2>&1 | grep -c pg-test) (should be 1)"
echo "Secrets: $(oc get secret -n $NAMESPACE 2>&1 | grep -c pg-test) (should be 1)"
echo "StatefulSets: $(oc get statefulset -n $NAMESPACE 2>&1 | grep -c pg-test) (should be 1)"
```

**Expected**: Exactly 1 PDB, no duplicates. All other resources unchanged.

---

## Phase B.8: HA Validation Markers

### B.8.1 Reject Invalid AntiAffinityMode

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-bad-ha
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "16"
  storage:
    size: 1Gi
  ha:
    minAvailable: 2
    antiAffinityMode: "none"
EOF
```

**Expected**: Rejected — antiAffinityMode enum only allows "preferred" or "required".

### B.8.2 Reject Invalid minAvailable

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-bad-ha
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "16"
  storage:
    size: 1Gi
  ha:
    minAvailable: 0
EOF
```

**Expected**: Rejected — minAvailable minimum is 1.

### B.8.3 Verify HA Defaults Applied

```bash
cat <<EOF | oc apply -f -
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-ha-defaults
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "16"
  storage:
    size: 1Gi
  ha:
    minAvailable: 2
EOF

sleep 5
oc get postgrescluster pg-ha-defaults -n $NAMESPACE -o jsonpath='{.spec.ha.antiAffinityMode}' && echo " (should be preferred)"

# Cleanup
oc delete postgrescluster pg-ha-defaults -n $NAMESPACE
```

**Expected**: `antiAffinityMode` defaults to "preferred".

---

## Phase B.9: Disable HA and Cleanup

### B.9.1 Disable HA

```bash
# Remove HA from the CR
oc patch postgrescluster pg-test -n $NAMESPACE --type json -p '[{"op":"remove","path":"/spec/ha"}]'
sleep 15

echo "=== PDB After HA Disabled ==="
oc get pdb pg-test-pdb -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: PDB deleted (correct)" || echo "FAIL: PDB still exists"

echo ""
echo "=== HAReady After HA Disabled ==="
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'HAReady':
        print(f\"HAReady: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**:
- [ ] PDB `pg-test-pdb` deleted
- [ ] HAReady condition is False (reason: HANotConfigured)
- [ ] All other resources (Secret, ConfigMap, Service, StatefulSet) unaffected

### B.9.2 Verify StatefulSet Anti-Affinity Cleared

```bash
oc get statefulset pg-test -n $NAMESPACE -o jsonpath='{.spec.template.spec.affinity}' && echo "" || echo "No affinity set (correct)"
```

**Expected**: Affinity is empty/null (anti-affinity removed when HA disabled).

---

## Phase B.10: Delete CR with HA

### B.10.1 Re-enable HA, Then Delete

```bash
# Re-enable HA to ensure PDB exists
oc patch postgrescluster pg-test -n $NAMESPACE --type merge -p '{"spec":{"ha":{"minAvailable":2}}}'
sleep 15

# Verify PDB exists
oc get pdb pg-test-pdb -n $NAMESPACE && echo "PDB exists before deletion"

# Delete the CR
oc delete postgrescluster pg-test -n $NAMESPACE
sleep 15

# Verify ALL resources garbage collected including PDB
oc get secret pg-test-credentials -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Secret cleaned"
oc get configmap pg-test-config -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: ConfigMap cleaned"
oc get service pg-test-headless -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Service cleaned"
oc get statefulset pg-test -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: StatefulSet cleaned"
oc get pdb pg-test-pdb -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: PDB cleaned"
```

**Expected**:
- [ ] All 5 managed resources garbage collected (Secret, ConfigMap, Service, StatefulSet, PDB)
- [ ] No orphaned resources remain

---

## Phase B.11: RBAC Verification

### B.11.1 Verify PDB RBAC

```bash
oc auth can-i create poddisruptionbudgets --as=system:serviceaccount:$NAMESPACE:postgres-operator-controller-manager && echo "PASS: Can create PDBs" || echo "FAIL"
oc auth can-i delete poddisruptionbudgets --as=system:serviceaccount:$NAMESPACE:postgres-operator-controller-manager && echo "PASS: Can delete PDBs" || echo "FAIL"
```

**Expected**: Both return "yes".

---

## Phase B.12: OLM Bundle Validation

### B.12.1 Verify Bundle Version

```bash
echo "=== CSV Version ==="
grep 'name:.*postgres-operator.v' bundle/manifests/postgres-operator.clusterserviceversion.yaml | head -1
grep 'replaces:' bundle/manifests/postgres-operator.clusterserviceversion.yaml
grep '  version:' bundle/manifests/postgres-operator.clusterserviceversion.yaml | head -1
```

**Expected**:
- [ ] CSV name: `postgres-operator.v0.2.0`
- [ ] replaces: `postgres-operator.v0.1.0`
- [ ] version: `0.2.0`

### B.12.2 Verify HA Descriptors in CSV

```bash
grep -A2 'ha\.\|ha$' bundle/manifests/postgres-operator.clusterserviceversion.yaml | grep -E 'path:|displayName:' | head -10
```

**Expected**: specDescriptors for `ha`, `ha.minAvailable`, `ha.maxUnavailable`, `ha.antiAffinityMode`.

### B.12.3 Verify PDB RBAC in CSV

```bash
grep -A3 'poddisruptionbudgets' bundle/manifests/postgres-operator.clusterserviceversion.yaml
```

**Expected**: `policy/poddisruptionbudgets` with CRUD verbs.

### B.12.4 Verify PDB in Owned Resources

```bash
grep -A1 'PodDisruptionBudget' bundle/manifests/postgres-operator.clusterserviceversion.yaml
```

**Expected**: PodDisruptionBudget listed in owned resources.

### B.12.5 Bundle Validate (if operator-sdk available)

```bash
operator-sdk bundle validate bundle/
```

**Expected**: No errors.

---

## Scenario B Cleanup

```bash
# Delete all test CRs
oc delete postgrescluster --all -n $NAMESPACE
sleep 15

# If NOT continuing to Scenario C, undeploy the operator:
# make undeploy                                                    # if deployed with make deploy
# operator-sdk cleanup postgres-operator --namespace $NAMESPACE    # if deployed with OLM
# oc delete project $NAMESPACE
```

---

## Scenario B Summary Checklist

| # | Test | Phase | Expected |
|---|------|-------|----------|
| 1 | Operator deploys with v0.2.0 image | B.1 | Pod Running, CRD has HA fields |
| 2 | Existing resources work without HA | B.2 | Secret, ConfigMap, Service, StatefulSet created |
| 3 | No PDB when HA not configured | B.2 | PDB not found |
| 4 | No HAReady condition when HA not configured | B.2 | Not present or False |
| 5 | PDB created with minAvailable | B.3 | pg-test-pdb with minAvailable=2 |
| 6 | PDB has correct owner reference | B.3 | PostgresCluster |
| 7 | HAReady condition is True | B.3 | HAConfigured |
| 8 | Existing resources unaffected by HA | B.3 | No changes to Secret/StatefulSet |
| 9 | PDB updated to maxUnavailable | B.4 | maxUnavailable=1, minAvailable cleared |
| 10 | Preferred anti-affinity on StatefulSet | B.5 | preferredDuringScheduling with weight 100 |
| 11 | Required anti-affinity on StatefulSet | B.5 | requiredDuringScheduling |
| 12 | PDB defaults to minAvailable=replicas-1 | B.6 | minAvailable=2 when replicas=3 |
| 13 | Idempotent — no duplicate PDBs | B.7 | Exactly 1 PDB after re-reconcile |
| 14 | Invalid antiAffinityMode rejected | B.8 | Validation error |
| 15 | Invalid minAvailable rejected | B.8 | Validation error |
| 16 | antiAffinityMode defaults to preferred | B.8 | Default applied |
| 17 | PDB deleted when HA disabled | B.9 | PDB not found |
| 18 | HAReady False when HA disabled | B.9 | HANotConfigured |
| 19 | Anti-affinity cleared when HA disabled | B.9 | Affinity empty |
| 20 | All resources cleaned on CR delete (incl PDB) | B.10 | Not found |
| 21 | PDB RBAC works | B.11 | can-i returns yes |
| 22 | CSV version 0.2.0 with replaces | B.12 | Correct upgrade path |
| 23 | HA descriptors in CSV | B.12 | ha.* fields present |
| 24 | PDB RBAC in CSV | B.12 | policy/poddisruptionbudgets |
| 25 | PDB in owned resources | B.12 | PodDisruptionBudget listed |

---
---

# Scenario C: Webhooks + Network Security (v0.3.0)

Adds defaulting/validating admission webhooks + NetworkPolicy to the postgres-operator. Built using:
- **Step 1** (Generate): `designing-operator-api` SKILL (Workflow C) — Added webhook handler + 9 config files
- **Step 2** (Generate): `implementing-reconciliation` SKILL (Workflow B) — Added reconcileNetworkPolicy
- **Step 3a** (Test): `operator-test-generator` SUBAGENT (Workflow B) — Added NP + webhook tests
- **Step 3b** (Review): `operator-reviewer` SUBAGENT — Reviewed modified code
- **Step 4** (Generate): `bundling-operator` SKILL (Workflow B) — Updated CSV v0.2.0 → v0.3.0
- **Step 5** (Validate): `operator-bundle-validator` SUBAGENT — Validated updated bundle

**Changes**: Webhook handler (Default + ValidateCreate/Update/Delete), 9 webhook config files, reconcileNetworkPolicy, NetworkSecured condition, CSV v0.3.0 with replaces + webhookdefinitions.

**Prerequisites**:
- Scenario B completed successfully. All Scenario B CRs deleted.
- **cert-manager operator** installed on OpenShift (required for webhook TLS). Install from OperatorHub: Operators → OperatorHub → search "cert-manager" → install "cert-manager Operator for Red Hat OpenShift".

## Scenario C Environment Setup

```bash
export IMG=quay.io/mpaulgreen/postgres-operator:v0.3.0
export BUNDLE_IMG=quay.io/mpaulgreen/postgres-operator-bundle:v0.3.0
export NAMESPACE=postgres-operator-system

cd e2e/postgres-operator
```

---

## Phase C.1: Build and Deploy v0.3.0

### C.1.1 Verify cert-manager is Running

```bash
oc get pods -n cert-manager 2>/dev/null || oc get pods -n openshift-cert-manager 2>/dev/null || echo "cert-manager not found — install it first"
```

**Expected**: cert-manager pods Running. If not installed, install via OperatorHub before proceeding.

### C.1.2 Build the Operator Image

```bash
podman build --platform linux/amd64 -t $IMG .
podman push $IMG
```

### C.1.3 Deploy the Operator

#### Option A: `make deploy` (Development)

```bash
make manifests
make deploy IMG=$IMG
```

**Note**: With webhooks enabled, `make deploy` now also creates webhook Service, Certificate, MutatingWebhookConfiguration, and ValidatingWebhookConfiguration via kustomize. cert-manager must be running to issue the TLS certificate.

#### Option B: OLM

```bash
# Update CSV image reference
sed -i '' "s|quay.io/mpaulgreen/postgres-operator:v0.3.0|$IMG|g" bundle/manifests/postgres-operator.clusterserviceversion.yaml

# Refresh CRD in bundle
make manifests
cp config/crd/bases/database.postgres.example.com_postgresclusters.yaml bundle/manifests/

# Build and push bundle
podman build -t $BUNDLE_IMG -f bundle.Dockerfile .
podman push $BUNDLE_IMG

# Create namespace first
oc new-project $NAMESPACE || oc create namespace $NAMESPACE

# Deploy via OLM
operator-sdk run bundle $BUNDLE_IMG --namespace $NAMESPACE --timeout 5m
```

### C.1.4 Verify Deployment

```bash
# Operator pod running
oc get pods -n $NAMESPACE -l control-plane=controller-manager

# Webhook service and certificate
oc get service -n $NAMESPACE | grep webhook
oc get certificate -n $NAMESPACE 2>/dev/null || oc get certificate -n $(oc get deployment -n $NAMESPACE postgres-operator-controller-manager -o jsonpath='{.metadata.namespace}') 2>/dev/null || echo "Check cert-manager namespace"

# Webhook configurations registered
oc get mutatingwebhookconfiguration | grep postgres
oc get validatingwebhookconfiguration | grep postgres

# Controller logs — should show 8 EventSources (added NetworkPolicy)
oc logs -n $NAMESPACE -l control-plane=controller-manager --tail=20

# CRD still has HA fields from Scenario B
oc get crd postgresclusters.database.postgres.example.com -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | python3 -c "import json,sys; print(sorted(json.load(sys.stdin).keys()))"
```

**Expected**:
- [ ] Pod 1/1 Running with v0.3.0 image
- [ ] Webhook Service exists
- [ ] MutatingWebhookConfiguration and ValidatingWebhookConfiguration registered
- [ ] Controller watching 8 EventSources (added NetworkPolicy)
- [ ] CRD has all fields: backup, ha, replicas, resources, storage, version

---

## Phase C.2: Existing Features Regression

### C.2.1 Create CR Without Webhooks Interfering

```bash
cat <<EOF | oc apply -f -
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-test
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "16"
  storage:
    size: 1Gi
  ha:
    minAvailable: 2
EOF

sleep 30
```

### C.2.2 Verify All Scenario A+B Resources Created

```bash
echo "=== Managed Resources ==="
oc get secret pg-test-credentials -n $NAMESPACE && echo "PASS: Secret" || echo "FAIL: Secret"
oc get configmap pg-test-config -n $NAMESPACE && echo "PASS: ConfigMap" || echo "FAIL: ConfigMap"
oc get service pg-test-headless -n $NAMESPACE && echo "PASS: Service" || echo "FAIL: Service"
oc get statefulset pg-test -n $NAMESPACE && echo "PASS: StatefulSet" || echo "FAIL: StatefulSet"
oc get pdb pg-test-pdb -n $NAMESPACE && echo "PASS: PDB" || echo "FAIL: PDB"

echo ""
echo "=== New: NetworkPolicy ==="
oc get networkpolicy pg-test-network-policy -n $NAMESPACE && echo "PASS: NetworkPolicy" || echo "FAIL: NetworkPolicy"

echo ""
echo "=== Status ==="
oc get postgrescluster pg-test -n $NAMESPACE -o wide
```

**Expected**:
- [ ] All 5 existing resources created (Secret, ConfigMap, Service, StatefulSet, PDB)
- [ ] NetworkPolicy `pg-test-network-policy` created (new — always created as security baseline)
- [ ] Status shows Running

---

## Phase C.3: Webhook Defaulting

### C.3.1 Create CR with Missing Fields (Defaulting Should Fill Them)

```bash
cat <<EOF | oc apply -f -
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-defaults
  namespace: $NAMESPACE
spec:
  storage:
    size: 1Gi
EOF

sleep 5

echo "=== Defaulted Fields ==="
oc get postgrescluster pg-defaults -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas (should be 3)"
oc get postgrescluster pg-defaults -n $NAMESPACE -o jsonpath='{.spec.version}' && echo " version (should be 16)"
```

**Expected**:
- [ ] `replicas` defaulted to 3
- [ ] `version` defaulted to "16"

### C.3.2 Create CR with HA but No Disruption Budget Values

```bash
cat <<EOF | oc apply -f -
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-ha-default
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "16"
  storage:
    size: 1Gi
  ha: {}
EOF

sleep 5

echo "=== HA Defaults ==="
oc get postgrescluster pg-ha-default -n $NAMESPACE -o jsonpath='{.spec.ha.antiAffinityMode}' && echo " antiAffinityMode (should be preferred)"
oc get postgrescluster pg-ha-default -n $NAMESPACE -o jsonpath='{.spec.ha.minAvailable}' && echo " minAvailable (should be 2 = replicas-1)"
```

**Expected**:
- [ ] `antiAffinityMode` defaulted to "preferred"
- [ ] `minAvailable` defaulted to 2 (replicas - 1)

### C.3.3 Cleanup Defaulting Test CRs

```bash
oc delete postgrescluster pg-defaults pg-ha-default -n $NAMESPACE
sleep 10
```

---

## Phase C.4: Webhook Validation (Create)

### C.4.1 Reject minAvailable >= replicas

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-bad
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "16"
  storage:
    size: 1Gi
  ha:
    minAvailable: 3
EOF
```

**Expected**: Rejected — `ha.minAvailable (3) must be less than replicas (3)`.

### C.4.2 Reject Both minAvailable and maxUnavailable Set

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-bad
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "16"
  storage:
    size: 1Gi
  ha:
    minAvailable: 2
    maxUnavailable: 1
EOF
```

**Expected**: Rejected — `ha.minAvailable and ha.maxUnavailable are mutually exclusive`.

### C.4.3 Reject Backup Enabled Without Schedule

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.postgres.example.com/v1alpha1
kind: PostgresCluster
metadata:
  name: pg-bad
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "16"
  storage:
    size: 1Gi
  backup:
    enabled: true
EOF
```

**Expected**: Rejected — `backup.schedule is required when backup.enabled is true`.

---

## Phase C.5: Webhook Validation (Update)

### C.5.1 Reject Storage Size Reduction

```bash
# First verify pg-test has 1Gi storage
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.spec.storage.size}' && echo " (current size)"

# Try to reduce storage
oc patch postgrescluster pg-test -n $NAMESPACE --type merge -p '{"spec":{"storage":{"size":"500Mi"}}}' 2>&1
```

**Expected**: Rejected — `storage size cannot be reduced from 1Gi to 500Mi`.

### C.5.2 Allow Storage Size Increase

```bash
oc patch postgrescluster pg-test -n $NAMESPACE --type merge -p '{"spec":{"storage":{"size":"2Gi"}}}'
```

**Expected**: Accepted — storage increase is allowed.

### C.5.3 Restore Original Storage Size

```bash
# Note: can't reduce back to 1Gi (webhook blocks it), so create fresh CR later if needed
# For now leave at 2Gi
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.spec.storage.size}' && echo " (should be 2Gi)"
```

---

## Phase C.6: NetworkPolicy

### C.6.1 Verify NetworkPolicy Details

```bash
echo "=== NetworkPolicy ==="
oc get networkpolicy pg-test-network-policy -n $NAMESPACE

echo ""
echo "=== Ingress Rules ==="
oc get networkpolicy pg-test-network-policy -n $NAMESPACE -o jsonpath='{.spec.ingress}' | python3 -m json.tool

echo ""
echo "=== Egress Rules ==="
oc get networkpolicy pg-test-network-policy -n $NAMESPACE -o jsonpath='{.spec.egress}' | python3 -m json.tool

echo ""
echo "=== Policy Types ==="
oc get networkpolicy pg-test-network-policy -n $NAMESPACE -o jsonpath='{.spec.policyTypes}' && echo ""

echo ""
echo "=== Owner Reference ==="
oc get networkpolicy pg-test-network-policy -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be PostgresCluster)"
```

**Expected**:
- [ ] Ingress allows port 5432 from pods in same namespace
- [ ] Egress allows DNS (port 53 TCP/UDP) and intra-cluster replication (port 5432)
- [ ] PolicyTypes includes both Ingress and Egress
- [ ] Owner reference points to PostgresCluster

### C.6.2 Verify NetworkSecured Condition

```bash
oc get postgrescluster pg-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'NetworkSecured':
        print(f\"NetworkSecured: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**: `NetworkSecured: True (reason: NetworkSecured)`

---

## Phase C.7: Idempotency

### C.7.1 Re-reconcile With Webhooks + NetworkPolicy

```bash
oc delete pod -n $NAMESPACE -l control-plane=controller-manager
oc wait --for=condition=available deployment/postgres-operator-controller-manager -n $NAMESPACE --timeout=60s
sleep 15

echo "NetworkPolicies: $(oc get networkpolicy -n $NAMESPACE 2>&1 | grep -c pg-test) (should be 1)"
echo "PDBs: $(oc get pdb -n $NAMESPACE 2>&1 | grep -c pg-test) (should be 1)"
echo "Secrets: $(oc get secret -n $NAMESPACE 2>&1 | grep -c pg-test) (should be 1)"
echo "StatefulSets: $(oc get statefulset -n $NAMESPACE 2>&1 | grep -c pg-test) (should be 1)"
```

**Expected**: Exactly 1 of each resource, no duplicates.

---

## Phase C.8: Delete CR

### C.8.1 Delete and Verify All Resources Cleaned

```bash
oc delete postgrescluster pg-test -n $NAMESPACE
sleep 15

oc get secret pg-test-credentials -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Secret cleaned"
oc get configmap pg-test-config -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: ConfigMap cleaned"
oc get service pg-test-headless -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Service cleaned"
oc get statefulset pg-test -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: StatefulSet cleaned"
oc get pdb pg-test-pdb -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: PDB cleaned"
oc get networkpolicy pg-test-network-policy -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: NetworkPolicy cleaned"
```

**Expected**:
- [ ] All 6 managed resources garbage collected (Secret, ConfigMap, Service, StatefulSet, PDB, NetworkPolicy)
- [ ] No orphaned resources remain

---

## Phase C.9: RBAC Verification

### C.9.1 Verify NetworkPolicy RBAC

```bash
oc auth can-i create networkpolicies --as=system:serviceaccount:$NAMESPACE:postgres-operator-controller-manager && echo "PASS: Can create NetworkPolicies" || echo "FAIL"
oc auth can-i delete networkpolicies --as=system:serviceaccount:$NAMESPACE:postgres-operator-controller-manager && echo "PASS: Can delete NetworkPolicies" || echo "FAIL"
```

**Expected**: Both return "yes".

---

## Phase C.10: OLM Bundle Validation

### C.10.1 Verify Bundle Version

```bash
echo "=== CSV Version ==="
grep 'name:.*postgres-operator.v' bundle/manifests/postgres-operator.clusterserviceversion.yaml | head -1
grep 'replaces:' bundle/manifests/postgres-operator.clusterserviceversion.yaml
grep '^  version:' bundle/manifests/postgres-operator.clusterserviceversion.yaml
```

**Expected**:
- [ ] CSV name: `postgres-operator.v0.3.0`
- [ ] replaces: `postgres-operator.v0.2.0`
- [ ] version: `0.3.0`

### C.10.2 Verify Webhook Definitions in CSV

```bash
grep -A5 'webhookdefinitions' bundle/manifests/postgres-operator.clusterserviceversion.yaml | head -10
grep 'webhookPath' bundle/manifests/postgres-operator.clusterserviceversion.yaml
```

**Expected**:
- [ ] `webhookdefinitions` section present
- [ ] Mutating webhook path: `/mutate-database-postgres-example-com-v1alpha1-postgrescluster`
- [ ] Validating webhook path: `/validate-database-postgres-example-com-v1alpha1-postgrescluster`

### C.10.3 Verify NetworkPolicy RBAC in CSV

```bash
grep -A3 'networkpolicies' bundle/manifests/postgres-operator.clusterserviceversion.yaml
```

**Expected**: `networking.k8s.io/networkpolicies` with CRUD verbs.

### C.10.4 Verify NetworkPolicy in Owned Resources

```bash
grep -A1 'NetworkPolicy' bundle/manifests/postgres-operator.clusterserviceversion.yaml | head -2
```

**Expected**: NetworkPolicy listed in owned resources.

### C.10.5 Bundle Validate (if operator-sdk available)

```bash
operator-sdk bundle validate bundle/
```

**Expected**: No errors.

---

## Scenario C Cleanup

```bash
# Delete all test CRs
oc delete postgrescluster --all -n $NAMESPACE
sleep 15

# If NOT continuing to Scenario D, undeploy the operator:
# make undeploy                                                    # if deployed with make deploy
# operator-sdk cleanup postgres-operator --namespace $NAMESPACE    # if deployed with OLM
# oc delete project $NAMESPACE
```

---

## Scenario C Summary Checklist

| # | Test | Phase | Expected |
|---|------|-------|----------|
| 1 | cert-manager running | C.1 | Pods Running |
| 2 | Operator deploys with v0.3.0 image | C.1 | Pod Running |
| 3 | Webhook Service exists | C.1 | Service on port 443 |
| 4 | MutatingWebhookConfiguration registered | C.1 | Exists |
| 5 | ValidatingWebhookConfiguration registered | C.1 | Exists |
| 6 | All A+B resources still work | C.2 | Secret, ConfigMap, Service, StatefulSet, PDB |
| 7 | NetworkPolicy created (always, security baseline) | C.2 | pg-test-network-policy |
| 8 | Replicas defaulted to 3 when 0 | C.3 | Webhook defaulting |
| 9 | Version defaulted to "16" when empty | C.3 | Webhook defaulting |
| 10 | HA antiAffinityMode defaulted to preferred | C.3 | Webhook defaulting |
| 11 | HA minAvailable defaulted to replicas-1 | C.3 | Webhook defaulting |
| 12 | Reject minAvailable >= replicas | C.4 | Webhook validation error |
| 13 | Reject both minAvailable and maxUnavailable | C.4 | Webhook validation error |
| 14 | Reject backup enabled without schedule | C.4 | Webhook validation error |
| 15 | Reject storage size reduction on update | C.5 | Webhook validation error |
| 16 | Allow storage size increase | C.5 | Accepted |
| 17 | NetworkPolicy has correct ingress (port 5432) | C.6 | From same namespace |
| 18 | NetworkPolicy has correct egress (DNS + replication) | C.6 | Port 53 + 5432 |
| 19 | NetworkPolicy owner reference correct | C.6 | PostgresCluster |
| 20 | NetworkSecured condition True | C.6 | NetworkSecured |
| 21 | Idempotent — no duplicate NetworkPolicies | C.7 | Exactly 1 after re-reconcile |
| 22 | All 6 resources cleaned on CR delete | C.8 | Including NetworkPolicy |
| 23 | NetworkPolicy RBAC works | C.9 | can-i returns yes |
| 24 | CSV version 0.3.0 with replaces | C.10 | Correct upgrade path |
| 25 | Webhook definitions in CSV | C.10 | Both mutating + validating paths |
| 26 | NetworkPolicy RBAC in CSV | C.10 | networking.k8s.io/networkpolicies |
| 27 | NetworkPolicy in owned resources | C.10 | Listed |

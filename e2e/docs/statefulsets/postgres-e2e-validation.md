# E2E Validation Guide: PostgreSQL Operator on OpenShift

End-to-end validation of the PostgreSQL operator on a live OpenShift cluster.

The operator was built using the mandatory workflow from CLAUDE.md:
- **Steps 1-3** (Generate): scaffolding-operator, designing-operator-api, implementing-reconciliation **SKILLS**
- **Step 4a** (Test): operator-test-generator **SUBAGENT** (Discovery → Generate → Validate → Report)
- **Step 4b** (Review): operator-reviewer **SUBAGENT** (Read → Scripts → Inspect → Report)
- **Step 5** (Generate): bundling-operator **SKILL**
- **Step 6** (Validate): operator-bundle-validator **SUBAGENT** (Validate → Certification → Report)

**Project stats**: 55 files (9 scaffold root + 4 API + 7 controller + 29 config + 4 bundle + 2 cmd/hack), 5 reconciler methods, 16 test cases, 9 RBAC markers, all 9 validation scripts pass.

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

## Cleanup

```bash
# Delete all test CRs first
oc delete postgrescluster --all -n $NAMESPACE
sleep 15

# If deployed with `make deploy`:
make undeploy

# If deployed with OLM (Option B1 or B2):
operator-sdk cleanup postgres-operator --namespace $NAMESPACE
oc delete project $NAMESPACE
```

---

## Summary Checklist

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

# E2E Validation Guide: MongoDB Operator on OpenShift

End-to-end validation of the MongoDB operator on a live OpenShift cluster. This is the **gap-coverage test** — validating untested skill patterns: **Job (batch/v1) reconciliation** and **different-group multi-CRD** (Scenario E).

| Scenario | Feature | Bundle Version | Skills Tested | New Resource |
|----------|---------|----------------|---------------|-------------|
| A | Core operator with Job backup | v0.1.0 | All 5 + all 3 subagents | StatefulSet, Service×2, Secret×2, ConfigMap, Job |
| B | Arbiter node | v0.2.0 | 4 (Workflow B) + 3 subagents | Deployment (arbiter) |
| C | Webhooks + Network Security | v0.3.0 | 4 (Workflow C) + 3 subagents | NetworkPolicy |
| D | API Maturity + Sharding Config | v0.4.0 | 4 (Workflow D) + 3 subagents | — (ShardingSpec) |
| E | Different-group CRD (MongoBackupPolicy) | v0.5.0 | scaffolding C + all | MongoBackupPolicy CRD + controller |

**Run scenarios in order** — each builds on the previous.

---

# Scenario A: Core Operator with Job Backup (v0.1.0)

The operator was built using the mandatory workflow from CLAUDE.md:
- **Step 1** (Generate): `scaffolding-operator` SKILL (Workflow A)
- **Step 2** (Generate): `designing-operator-api` SKILL (Workflow A)
- **Step 3** (Generate): `implementing-reconciliation` SKILL (Workflow A)
- **Step 4a** (Test): `operator-test-generator` SUBAGENT
- **Step 4b** (Review): `operator-reviewer` SUBAGENT
- **Step 5** (Generate): `bundling-operator` SKILL (Workflow A)
- **Step 6** (Validate): `operator-bundle-validator` SUBAGENT

**Project stats**: 7 reconciler methods, 4 conditions (Available, Progressing, Degraded, BackupReady), 7 managed resources (Secret×2, ConfigMap, Service×2, StatefulSet, Job), 9 RBAC markers, all validation scripts pass.

**Key differences from PostgreSQL/Redis**: Two Secrets (admin + keyFile), YAML-format ConfigMap (mongod.conf), backup via Job (batch/v1) — **new resource type never tested before**, port 27017, replica set with election semantics. Uses UBI micro mock container for E2E (Red Hat certified MongoDB images require enterprise license).

**Bug found during build**: Bug #18 — `check-idempotency.py` didn't count `List()` as an existence check. Fixed in implementing-reconciliation skill.

## Prerequisites

- OpenShift 4.14+ cluster with cluster-admin access
- `oc` CLI logged in (`oc whoami` returns a user)
- `podman` for building images
- Access to a container registry (quay.io)
- A default StorageClass available (`oc get sc`)

## Environment Setup

```bash
export IMG=quay.io/mpaulgreen/mongodb-operator:v0.1.0
export BUNDLE_IMG=quay.io/mpaulgreen/mongodb-operator-bundle:v0.1.0
export NAMESPACE=mongodb-operator-system

cd e2e/mongodb-operator
```

---

## Phase 1: Build and Deploy

### 1.1 Build the Operator Image

```bash
podman build --platform linux/amd64 -t $IMG .
podman push $IMG
```

**Expected**: Image builds and pushes successfully.

### 1.2 Deploy the Operator

#### Option A: `make deploy` (Development)

```bash
make manifests
make deploy IMG=$IMG
```

#### Option B: OLM

```bash
# Update CSV image reference
sed -i '' "s|quay.io/mpaulgreen/mongodb-operator:v0.1.0|$IMG|g" bundle/manifests/mongodb-operator.clusterserviceversion.yaml

# Build and push bundle
podman build -t $BUNDLE_IMG -f bundle.Dockerfile .
podman push $BUNDLE_IMG

# Create namespace first
oc new-project $NAMESPACE || oc create namespace $NAMESPACE

# Deploy via OLM
operator-sdk run bundle $BUNDLE_IMG --namespace $NAMESPACE --timeout 5m
```

### 1.3 Verify Deployment

```bash
oc get pods -n $NAMESPACE -l control-plane=controller-manager
oc logs -n $NAMESPACE -l control-plane=controller-manager --tail=20

# CRD installed
oc get crd mongoclusters.database.mongodb.example.com

# CRD has expected fields
oc get crd mongoclusters.database.mongodb.example.com -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | python3 -c "import json,sys; print(sorted(json.load(sys.stdin).keys()))"
```

**Expected**:
- [ ] Pod 1/1 Running
- [ ] CRD `mongoclusters.database.mongodb.example.com` installed
- [ ] CRD fields: auth, backup, replicas, resources, storage, version
- [ ] Controller watching EventSources for Secret, ConfigMap, Service, StatefulSet, Job

---

## Phase 2: Basic CR Lifecycle

### 2.1 Create a Minimal MongoCluster

```bash
cat <<EOF | oc apply -f -
apiVersion: database.mongodb.example.com/v1alpha1
kind: MongoCluster
metadata:
  name: mongo-test
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "7.0"
  storage:
    size: 1Gi
EOF

sleep 30
```

### 2.2 Verify All 7 Managed Resources Created

```bash
echo "=== Managed Resources ==="
oc get secret mongo-test-admin -n $NAMESPACE && echo "PASS: Admin Secret" || echo "FAIL: Admin Secret"
oc get secret mongo-test-keyfile -n $NAMESPACE && echo "PASS: KeyFile Secret" || echo "FAIL: KeyFile Secret"
oc get configmap mongo-test-config -n $NAMESPACE && echo "PASS: ConfigMap" || echo "FAIL: ConfigMap"
oc get service mongo-test-headless -n $NAMESPACE && echo "PASS: Headless Service" || echo "FAIL: Headless Service"
oc get service mongo-test-client -n $NAMESPACE && echo "PASS: Client Service" || echo "FAIL: Client Service"
oc get statefulset mongo-test -n $NAMESPACE && echo "PASS: StatefulSet" || echo "FAIL: StatefulSet"

echo ""
echo "=== No Backup Job (backup not enabled) ==="
oc get jobs -n $NAMESPACE -l app.kubernetes.io/instance=mongo-test,app.kubernetes.io/component=backup 2>&1 | grep "No resources" && echo "PASS: No backup Job" || echo "FAIL: Backup Job exists"

echo ""
echo "=== Owner References ==="
oc get secret mongo-test-admin -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be MongoCluster)"
oc get service mongo-test-client -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be MongoCluster)"
oc get statefulset mongo-test -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be MongoCluster)"

echo ""
echo "=== CR Status ==="
oc get mongocluster mongo-test -n $NAMESPACE -o wide
```

**Expected**:
- [ ] Secret `mongo-test-admin` created with `MONGO_INITDB_ROOT_USERNAME` and `MONGO_INITDB_ROOT_PASSWORD` keys
- [ ] Secret `mongo-test-keyfile` created with `keyfile` key
- [ ] ConfigMap `mongo-test-config` created with `mongod.conf`
- [ ] Headless Service `mongo-test-headless` (ClusterIP: None, port 27017)
- [ ] Client Service `mongo-test-client` (ClusterIP, port 27017)
- [ ] StatefulSet `mongo-test` created with 3 replicas
- [ ] No backup Job (backup not enabled)
- [ ] All resources have ownerReferences pointing to MongoCluster

### 2.3 Verify Two Services (MongoDB-Specific)

```bash
echo "=== Headless Service ==="
oc get service mongo-test-headless -n $NAMESPACE -o jsonpath='{.spec.clusterIP}' && echo " (should be None)"
oc get service mongo-test-headless -n $NAMESPACE -o jsonpath='{.spec.ports[0].port}' && echo " port (should be 27017)"

echo ""
echo "=== Client Service ==="
oc get service mongo-test-client -n $NAMESPACE -o jsonpath='{.spec.clusterIP}' && echo " (should NOT be None)"
oc get service mongo-test-client -n $NAMESPACE -o jsonpath='{.spec.ports[0].port}' && echo " port (should be 27017)"
```

**Expected**:
- [ ] Headless service has `clusterIP: None`
- [ ] Client service has a real ClusterIP (not None)
- [ ] Both on port 27017

### 2.4 Verify StatefulSet Details

```bash
echo "=== StatefulSet Spec ==="
oc get statefulset mongo-test -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas"
oc get statefulset mongo-test -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].image}' && echo " image"
oc get statefulset mongo-test -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].ports[0].containerPort}' && echo " port"
oc get statefulset mongo-test -n $NAMESPACE -o jsonpath='{.spec.volumeClaimTemplates[0].spec.resources.requests.storage}' && echo " storage"

echo ""
echo "=== Pod Status ==="
oc get pods -n $NAMESPACE -l app.kubernetes.io/instance=mongo-test
```

**Expected**:
- [ ] 3 replicas
- [ ] Image: `registry.access.redhat.com/ubi9/ubi-micro:latest` (mock container for E2E)
- [ ] Container port 27017
- [ ] PVC requests 1Gi storage

### 2.5 Verify Admin Secret Content

```bash
oc get secret mongo-test-admin -n $NAMESPACE -o jsonpath='{.data.MONGO_INITDB_ROOT_USERNAME}' | base64 -d && echo ""
oc get secret mongo-test-admin -n $NAMESPACE -o jsonpath='{.data.MONGO_INITDB_ROOT_PASSWORD}' | base64 -d && echo ""
```

**Expected**: Username=admin, Password is random (24 chars).

### 2.6 Verify KeyFile Secret Content

```bash
oc get secret mongo-test-keyfile -n $NAMESPACE -o jsonpath='{.data.keyfile}' | base64 -d | wc -c
```

**Expected**: 756 characters (keyFile length).

### 2.7 Verify ConfigMap Content

```bash
oc get configmap mongo-test-config -n $NAMESPACE -o jsonpath='{.data.mongod\.conf}'
```

**Expected**: Contains `storage.dbPath: /var/lib/mongodb/data`, `net.port: 27017`, `replication.replSetName: mongo-test`, `security.keyFile`.

### 2.8 Wait for Running Phase

```bash
oc wait --for=condition=ready pod -l app.kubernetes.io/instance=mongo-test -n $NAMESPACE --timeout=300s

oc get mongocluster mongo-test -n $NAMESPACE -o jsonpath='{.status}' | python3 -m json.tool
```

**Expected**:
- [ ] `phase: Running`
- [ ] `readyReplicas: 3`
- [ ] `currentVersion: "7.0"`
- [ ] `primaryEndpoint: mongo-test-client.$NAMESPACE.svc.cluster.local:27017`

### 2.9 Verify Conditions

```bash
oc get mongocluster mongo-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -m json.tool
```

**Expected conditions**:
- [ ] `Available: True`
- [ ] `Progressing: False`
- [ ] `Degraded: False`
- [ ] `BackupReady: False` (backup not enabled)

### 2.10 Verify Print Columns

```bash
oc get mongocluster -n $NAMESPACE
```

**Expected**: Table with columns Phase, Ready, Version, Age.

### 2.11 Verify Events

```bash
oc get events -n $NAMESPACE --field-selector involvedObject.name=mongo-test --sort-by='.lastTimestamp'
```

**Expected**: Events for AdminSecretCreated, KeyFileSecretCreated, ConfigMapCreated, HeadlessServiceCreated, ClientServiceCreated, StatefulSetCreated.

---

## Phase 3: Idempotency

### 3.1 Verify No Duplicate Resources

```bash
oc delete pod -n $NAMESPACE -l control-plane=controller-manager
oc wait --for=condition=available deployment -l control-plane=controller-manager -n $NAMESPACE --timeout=60s
sleep 15

echo "Admin Secrets: $(oc get secret -n $NAMESPACE 2>&1 | grep -c mongo-test-admin) (should be 1)"
echo "KeyFile Secrets: $(oc get secret -n $NAMESPACE 2>&1 | grep -c mongo-test-keyfile) (should be 1)"
echo "ConfigMaps: $(oc get configmap -n $NAMESPACE 2>&1 | grep -c mongo-test-config) (should be 1)"
echo "Headless Services: $(oc get service -n $NAMESPACE 2>&1 | grep -c mongo-test-headless) (should be 1)"
echo "Client Services: $(oc get service -n $NAMESPACE 2>&1 | grep -c mongo-test-client) (should be 1)"
echo "StatefulSets: $(oc get statefulset -n $NAMESPACE 2>&1 | grep -c mongo-test) (should be 1)"
```

**Expected**: Exactly 1 of each resource, no duplicates.

### 3.2 Verify Passwords Unchanged After Reconciliation

```bash
PASS_BEFORE=$(oc get secret mongo-test-admin -n $NAMESPACE -o jsonpath='{.data.MONGO_INITDB_ROOT_PASSWORD}')
KEY_BEFORE=$(oc get secret mongo-test-keyfile -n $NAMESPACE -o jsonpath='{.data.keyfile}')

oc label mongocluster mongo-test -n $NAMESPACE test-reconcile=true
sleep 10

PASS_AFTER=$(oc get secret mongo-test-admin -n $NAMESPACE -o jsonpath='{.data.MONGO_INITDB_ROOT_PASSWORD}')
KEY_AFTER=$(oc get secret mongo-test-keyfile -n $NAMESPACE -o jsonpath='{.data.keyfile}')
[ "$PASS_BEFORE" = "$PASS_AFTER" ] && echo "PASS: Admin password unchanged" || echo "FAIL: Admin password changed!"
[ "$KEY_BEFORE" = "$KEY_AFTER" ] && echo "PASS: KeyFile unchanged" || echo "FAIL: KeyFile changed!"
```

---

## Phase 4: Scaling

### 4.1 Scale Up

```bash
oc patch mongocluster mongo-test -n $NAMESPACE --type merge -p '{"spec":{"replicas":5}}'
sleep 15

oc get statefulset mongo-test -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas (should be 5)"
```

**Expected**:
- [ ] StatefulSet replicas updated to 5

### 4.2 Scale Down

```bash
oc patch mongocluster mongo-test -n $NAMESPACE --type merge -p '{"spec":{"replicas":1}}'
sleep 15

oc get statefulset mongo-test -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas (should be 1)"
```

**Expected**:
- [ ] Replicas updated to 1

### 4.3 Scale Back to 3

```bash
oc patch mongocluster mongo-test -n $NAMESPACE --type merge -p '{"spec":{"replicas":3}}'
sleep 15

oc get statefulset mongo-test -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas (should be 3)"
```

---

## Phase 5: Backup Job (NEW — batch/v1)

This phase tests **Job reconciliation** — the new resource type not covered by PostgreSQL or Redis.

### 5.1 Enable Backup

```bash
oc patch mongocluster mongo-test -n $NAMESPACE --type merge -p '{"spec":{"backup":{"enabled":true,"retentionDays":7}}}'
sleep 15
```

### 5.2 Verify Backup Job Created

```bash
echo "=== Backup Jobs ==="
oc get jobs -n $NAMESPACE -l app.kubernetes.io/instance=mongo-test,app.kubernetes.io/component=backup

echo ""
echo "=== Job Labels ==="
JOB_NAME=$(oc get jobs -n $NAMESPACE -l app.kubernetes.io/instance=mongo-test,app.kubernetes.io/component=backup -o jsonpath='{.items[0].metadata.name}')
echo "Job name: $JOB_NAME (should match mongo-test-backup-<timestamp>)"
oc get job $JOB_NAME -n $NAMESPACE -o jsonpath='{.metadata.labels}' | python3 -m json.tool

echo ""
echo "=== Job Owner Reference ==="
oc get job $JOB_NAME -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be MongoCluster)"

echo ""
echo "=== Job Container ==="
oc get job $JOB_NAME -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].image}' && echo " image"
oc get job $JOB_NAME -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].name}' && echo " container name (should be mongodump)"
```

**Expected**:
- [ ] Backup Job created with name `mongo-test-backup-<timestamp>`
- [ ] Labels include `app.kubernetes.io/component: backup` and `app.kubernetes.io/instance: mongo-test`
- [ ] Owner reference → MongoCluster
- [ ] Container name: mongodump
- [ ] Image: UBI micro (mock — completes via `/bin/sleep 5`)

### 5.3 Verify No Duplicate Jobs

```bash
# Restart controller to trigger re-reconcile
oc delete pod -n $NAMESPACE -l control-plane=controller-manager
oc wait --for=condition=available deployment -l control-plane=controller-manager -n $NAMESPACE --timeout=60s
sleep 15

JOB_COUNT=$(oc get jobs -n $NAMESPACE -l app.kubernetes.io/instance=mongo-test,app.kubernetes.io/component=backup --no-headers 2>&1 | wc -l)
echo "Backup Jobs: $JOB_COUNT (should be 1 — no duplicate created)"
```

**Expected**:
- [ ] Still exactly 1 backup Job (List()-based idempotency check prevents duplicates)

### 5.4 Verify BackupReady Condition

```bash
oc get mongocluster mongo-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'BackupReady':
        print(f\"BackupReady: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**: BackupReady condition present (True if Job succeeded, False with reason if pending/not yet completed).

### 5.5 Verify Backup Events

```bash
oc get events -n $NAMESPACE --field-selector involvedObject.name=mongo-test --sort-by='.lastTimestamp' | grep -i backup
```

**Expected**: Event for BackupJobCreated.

### 5.6 Disable Backup

```bash
oc patch mongocluster mongo-test -n $NAMESPACE --type merge -p '{"spec":{"backup":{"enabled":false}}}'
sleep 15

echo "=== BackupReady After Disable ==="
oc get mongocluster mongo-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'BackupReady':
        print(f\"BackupReady: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**:
- [ ] BackupReady: False (BackupDisabled)

---

## Phase 6: Finalizer and Deletion

### 6.1 Verify Finalizer Exists

```bash
oc get mongocluster mongo-test -n $NAMESPACE -o jsonpath='{.metadata.finalizers}' && echo ""
```

**Expected**: `["database.mongodb.example.com/finalizer"]`

### 6.2 Delete the CR

```bash
oc delete mongocluster mongo-test -n $NAMESPACE
sleep 15

oc get secret mongo-test-admin -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Admin Secret cleaned"
oc get secret mongo-test-keyfile -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: KeyFile Secret cleaned"
oc get configmap mongo-test-config -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: ConfigMap cleaned"
oc get service mongo-test-headless -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Headless Service cleaned"
oc get service mongo-test-client -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Client Service cleaned"
oc get statefulset mongo-test -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: StatefulSet cleaned"
oc get jobs -n $NAMESPACE -l app.kubernetes.io/instance=mongo-test 2>&1 | grep "No resources" && echo "PASS: Backup Jobs cleaned"
```

**Expected**:
- [ ] All 7 managed resources garbage collected (including backup Jobs)
- [ ] No orphaned resources remain

---

## Phase 7: Validation Markers

### 7.1 Reject Invalid Replicas

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.mongodb.example.com/v1alpha1
kind: MongoCluster
metadata:
  name: mongo-invalid
  namespace: $NAMESPACE
spec:
  replicas: 10
  version: "7.0"
  storage:
    size: 1Gi
EOF
```

**Expected**: Rejected — `replicas` max is 7.

### 7.2 Reject Invalid Version

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.mongodb.example.com/v1alpha1
kind: MongoCluster
metadata:
  name: mongo-invalid
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "6.0"
  storage:
    size: 1Gi
EOF
```

**Expected**: Rejected — version enum only allows "7.0", "8.0".

### 7.3 Reject Invalid Storage Size Pattern

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.mongodb.example.com/v1alpha1
kind: MongoCluster
metadata:
  name: mongo-invalid
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "7.0"
  storage:
    size: "100MB"
EOF
```

**Expected**: Rejected — storage size must match pattern `^[0-9]+[KMGT]i$`.

### 7.4 Reject Invalid Backup RetentionDays

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: database.mongodb.example.com/v1alpha1
kind: MongoCluster
metadata:
  name: mongo-invalid
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "7.0"
  storage:
    size: 1Gi
  backup:
    enabled: true
    retentionDays: 60
EOF
```

**Expected**: Rejected — `retentionDays` max is 30.

### 7.5 Verify Defaults Applied

```bash
cat <<EOF | oc apply -f -
apiVersion: database.mongodb.example.com/v1alpha1
kind: MongoCluster
metadata:
  name: mongo-defaults
  namespace: $NAMESPACE
spec:
  storage:
    size: 5Gi
EOF

sleep 5
oc get mongocluster mongo-defaults -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " (should be 3)"
oc get mongocluster mongo-defaults -n $NAMESPACE -o jsonpath='{.spec.version}' && echo " (should be 7.0)"

oc delete mongocluster mongo-defaults -n $NAMESPACE
```

**Expected**: Defaults applied — replicas=3, version="7.0".

---

## Phase 8: RBAC Verification

### 8.1 Verify Operator RBAC Works

```bash
oc auth can-i get secrets --as=system:serviceaccount:$NAMESPACE:mongodb-operator-controller-manager && echo "PASS" || echo "FAIL"
oc auth can-i create statefulsets --as=system:serviceaccount:$NAMESPACE:mongodb-operator-controller-manager && echo "PASS" || echo "FAIL"
oc auth can-i create jobs --as=system:serviceaccount:$NAMESPACE:mongodb-operator-controller-manager && echo "PASS: Jobs RBAC" || echo "FAIL: Jobs RBAC"
oc auth can-i update mongoclusters/status --as=system:serviceaccount:$NAMESPACE:mongodb-operator-controller-manager && echo "PASS" || echo "FAIL"
```

### 8.2 Verify No Excess Permissions

```bash
oc auth can-i create namespaces --as=system:serviceaccount:$NAMESPACE:mongodb-operator-controller-manager && echo "FAIL: excess perms" || echo "PASS: no excess"
oc auth can-i delete nodes --as=system:serviceaccount:$NAMESPACE:mongodb-operator-controller-manager && echo "FAIL: excess perms" || echo "PASS: no excess"
```

---

## Phase 9: Security Posture

### 9.1 Verify Non-Root Container

```bash
oc get deployment mongodb-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.securityContext.runAsNonRoot}' && echo " (should be true)"
```

### 9.2 Verify Capabilities Dropped

```bash
oc get deployment mongodb-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].securityContext.capabilities.drop}' && echo ""
```

**Expected**: `["ALL"]`

---

## Phase 10: Health Probes

### 10.1 Verify Liveness Probe

```bash
oc get deployment mongodb-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].livenessProbe}' | python3 -m json.tool
```

**Expected**: httpGet on /healthz:8081

### 10.2 Verify Readiness Probe

```bash
oc get deployment mongodb-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].readinessProbe}' | python3 -m json.tool
```

**Expected**: httpGet on /readyz:8081

---

## Phase 11: OLM Bundle Validation (Optional)

### 11.1 Bundle Validate

```bash
operator-sdk bundle validate bundle/
```

**Expected**: No errors.

---

## Phase 12: Multi-Instance

### 12.1 Create Multiple MongoClusters

```bash
for i in 1 2 3; do
cat <<EOF | oc apply -f -
apiVersion: database.mongodb.example.com/v1alpha1
kind: MongoCluster
metadata:
  name: mongo-multi-$i
  namespace: $NAMESPACE
spec:
  replicas: 1
  version: "7.0"
  storage:
    size: 1Gi
EOF
done

sleep 30
oc get mongocluster -n $NAMESPACE
```

**Expected**:
- [ ] 3 independent MongoClusters created
- [ ] Each has its own 2 Secrets, ConfigMap, 2 Services, StatefulSet
- [ ] No cross-contamination

### 12.2 Delete One, Others Unaffected

```bash
oc delete mongocluster mongo-multi-2 -n $NAMESPACE
sleep 10

oc get mongocluster -n $NAMESPACE
oc get statefulset -n $NAMESPACE
```

**Expected**: mongo-multi-1 and mongo-multi-3 still Running, mongo-multi-2 fully cleaned up.

---

## Scenario A Cleanup

```bash
oc delete mongocluster --all -n $NAMESPACE
sleep 15

# If NOT continuing to Scenario B, undeploy the operator:
# make undeploy                                                    # if deployed with make deploy
# operator-sdk cleanup mongodb-operator --namespace $NAMESPACE     # if deployed with OLM
# oc delete project $NAMESPACE
```

---

## Scenario A Summary Checklist

| # | Test | Phase | Expected |
|---|------|-------|----------|
| 1 | Operator deploys and starts | 1 | Pod Running |
| 2 | CRD installed with expected fields | 1 | auth, backup, replicas, resources, storage, version |
| 3 | CR creates all 7 managed resources | 2.2 | Secret×2, ConfigMap, Service×2, StatefulSet, no Job |
| 4 | Owner references set on all resources | 2.2 | All point to MongoCluster |
| 5 | Headless service is ClusterIP None | 2.3 | clusterIP: None, port 27017 |
| 6 | Client service has real ClusterIP | 2.3 | Not None, port 27017 |
| 7 | StatefulSet has correct image and replicas | 2.4 | mongodb image, 3 replicas |
| 8 | Admin Secret has username + password | 2.5 | admin, 24-char random |
| 9 | KeyFile Secret has correct length | 2.6 | 756 chars |
| 10 | ConfigMap has mongod.conf (YAML) | 2.7 | dbPath, port, replSetName, keyFile |
| 11 | Status reaches Running phase | 2.8 | phase=Running, readyReplicas=3 |
| 12 | PrimaryEndpoint points to client service | 2.8 | mongo-test-client...27017 |
| 13 | Conditions set correctly | 2.9 | Available=True, Degraded=False, BackupReady=False |
| 14 | Print columns display | 2.10 | Phase, Ready, Version, Age |
| 15 | Events recorded for each resource | 2.11 | 6 create events |
| 16 | Idempotent — no duplicates | 3.1 | Exactly 1 of each |
| 17 | Admin password unchanged on re-reconcile | 3.2 | Same base64 value |
| 18 | KeyFile unchanged on re-reconcile | 3.2 | Same base64 value |
| 19 | Scale up works | 4.1 | 5 replicas |
| 20 | Scale down works | 4.2 | 1 replica |
| 21 | Backup Job created when enabled | 5.2 | Job with backup labels + ownerRef |
| 22 | Backup Job has correct name pattern | 5.2 | mongo-test-backup-<timestamp> |
| 23 | No duplicate backup Job on re-reconcile | 5.3 | Still 1 Job (List-based check) |
| 24 | BackupReady condition set | 5.4 | Condition present |
| 25 | Backup event recorded | 5.5 | BackupJobCreated |
| 26 | BackupReady False when disabled | 5.6 | BackupDisabled |
| 27 | Finalizer present | 6.1 | Finalizer in metadata |
| 28 | Deletion cleans all 7 resources + Jobs | 6.2 | No orphans |
| 29 | Invalid replicas rejected | 7.1 | Validation error |
| 30 | Invalid version rejected | 7.2 | Validation error |
| 31 | Invalid storage pattern rejected | 7.3 | Validation error |
| 32 | Invalid retentionDays rejected | 7.4 | Validation error |
| 33 | Defaults applied | 7.5 | replicas=3, version=7.0 |
| 34 | RBAC allows needed operations (incl. batch/jobs) | 8.1 | can-i returns yes |
| 35 | No excess RBAC permissions | 8.2 | can-i returns no |
| 36 | Container runs as non-root | 9.1 | runAsNonRoot=true |
| 37 | Capabilities dropped | 9.2 | drop=[ALL] |
| 38 | Health probes configured | 10 | /healthz, /readyz |
| 39 | Bundle validates | 11 | No errors |
| 40 | Multiple instances independent | 12.1 | No cross-contamination |
| 41 | Deleting one doesn't affect others | 12.2 | Others still Running |

---
---

# Scenario B: Arbiter Node (v0.2.0)

Adds a MongoDB arbiter node — a vote-only Deployment with no data storage for replica set elections. Built using:
- **Step 1** (Generate): `designing-operator-api` SKILL (Workflow B) — Added ArbiterSpec to types
- **Step 2** (Generate): `implementing-reconciliation` SKILL (Workflow B) — Added reconcileArbiter (conditional Deployment)
- **Step 3a** (Test): `operator-test-generator` SUBAGENT (Workflow B) — Added arbiter tests (4 cases)
- **Step 3b** (Review): `operator-reviewer` SUBAGENT — Reviewed modified code (0 Critical)
- **Step 4** (Generate): `bundling-operator` SKILL (Workflow B) — Updated CSV v0.1.0 → v0.2.0
- **Step 5** (Validate): `operator-bundle-validator` SUBAGENT — Validated updated bundle

**Changes**: ArbiterSpec (enabled, resources), reconcileArbiter creating conditional Deployment (1 replica, no PVC, port 27017), ArbiterReady condition, Deployment RBAC, check-update for resources, CSV v0.2.0 with replaces.

**Key difference from Redis Sentinel**: Arbiter is always 1 replica (vote-only), has no PVC (no data), and no associated Service (no client access).

**Prerequisites**: Scenario A completed successfully. All Scenario A CRs deleted.

## Scenario B Environment Setup

```bash
export IMG=quay.io/mpaulgreen/mongodb-operator:v0.2.0
export BUNDLE_IMG=quay.io/mpaulgreen/mongodb-operator-bundle:v0.2.0
export NAMESPACE=mongodb-operator-system

cd e2e/mongodb-operator
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

#### Option B: OLM

```bash
# Update CSV image reference
sed -i '' "s|quay.io/mpaulgreen/mongodb-operator:v0.2.0|$IMG|g" bundle/manifests/mongodb-operator.clusterserviceversion.yaml

# Refresh CRD in bundle
make manifests
cp config/crd/bases/database.mongodb.example.com_mongoclusters.yaml bundle/manifests/

# Build and push bundle
podman build -t $BUNDLE_IMG -f bundle.Dockerfile .
podman push $BUNDLE_IMG

# Create namespace first
oc new-project $NAMESPACE || oc create namespace $NAMESPACE

# Deploy via OLM
operator-sdk run bundle $BUNDLE_IMG --namespace $NAMESPACE --timeout 5m
```

### B.1.3 Verify Deployment

```bash
oc get pods -n $NAMESPACE -l control-plane=controller-manager
oc logs -n $NAMESPACE -l control-plane=controller-manager --tail=20

# CRD has arbiter field
oc get crd mongoclusters.database.mongodb.example.com -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | python3 -c "import json,sys; print(sorted(json.load(sys.stdin).keys()))"

# Controller watching 7 EventSources (added Deployment)
oc logs -n $NAMESPACE -l control-plane=controller-manager --tail=20 | grep -E "Starting EventSource|Starting workers"
```

**Expected**:
- [ ] Pod 1/1 Running with v0.2.0 image
- [ ] CRD fields: arbiter, auth, backup, replicas, resources, storage, version
- [ ] Controller watching 7 EventSources (added Deployment)

---

## Phase B.2: Existing Features Regression

### B.2.1 Create CR Without Arbiter

```bash
cat <<EOF | oc apply -f -
apiVersion: database.mongodb.example.com/v1alpha1
kind: MongoCluster
metadata:
  name: mongo-test
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "7.0"
  storage:
    size: 1Gi
EOF

sleep 30
```

### B.2.2 Verify All Scenario A Resources Created

```bash
echo "=== Managed Resources ==="
oc get secret mongo-test-admin -n $NAMESPACE && echo "PASS: Admin Secret" || echo "FAIL"
oc get secret mongo-test-keyfile -n $NAMESPACE && echo "PASS: KeyFile Secret" || echo "FAIL"
oc get configmap mongo-test-config -n $NAMESPACE && echo "PASS: ConfigMap" || echo "FAIL"
oc get service mongo-test-headless -n $NAMESPACE && echo "PASS: Headless Service" || echo "FAIL"
oc get service mongo-test-client -n $NAMESPACE && echo "PASS: Client Service" || echo "FAIL"
oc get statefulset mongo-test -n $NAMESPACE && echo "PASS: StatefulSet" || echo "FAIL"

echo ""
echo "=== No Arbiter (not configured) ==="
oc get deployment mongo-test-arbiter -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: No Arbiter Deployment" || echo "FAIL"

echo ""
echo "=== Status ==="
oc get mongocluster mongo-test -n $NAMESPACE -o wide
```

**Expected**:
- [ ] All 6 existing resources created (Secret×2, ConfigMap, Service×2, StatefulSet)
- [ ] No Arbiter Deployment (arbiter not configured)
- [ ] Status shows Running

---

## Phase B.3: Enable Arbiter

### B.3.1 Enable Arbiter

```bash
oc patch mongocluster mongo-test -n $NAMESPACE --type merge -p '{"spec":{"arbiter":{"enabled":true}}}'
sleep 15
```

### B.3.2 Verify Arbiter Deployment Created

```bash
echo "=== Arbiter Deployment ==="
oc get deployment mongo-test-arbiter -n $NAMESPACE
oc get deployment mongo-test-arbiter -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas (should be 1)"
oc get deployment mongo-test-arbiter -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].ports[0].containerPort}' && echo " port (should be 27017)"

echo ""
echo "=== Arbiter Deployment Owner Reference ==="
oc get deployment mongo-test-arbiter -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be MongoCluster)"

echo ""
echo "=== Arbiter Deployment Labels ==="
oc get deployment mongo-test-arbiter -n $NAMESPACE -o jsonpath='{.metadata.labels}' | python3 -m json.tool
```

**Expected**:
- [ ] Deployment `mongo-test-arbiter` created with 1 replica (always 1)
- [ ] Port 27017
- [ ] Owner reference → MongoCluster
- [ ] Labels include `component: arbiter`

### B.3.3 Verify No PVC on Arbiter (Vote-Only)

```bash
oc get deployment mongo-test-arbiter -n $NAMESPACE -o jsonpath='{.spec.template.spec.volumes}' 2>/dev/null && echo " volumes" || echo "No volumes (correct — arbiter has no data)"
```

**Expected**:
- [ ] No volumes/PVC on arbiter Deployment (vote-only, no data storage)

### B.3.4 Verify ArbiterReady Condition

```bash
oc get mongocluster mongo-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'ArbiterReady':
        print(f\"ArbiterReady: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**:
- [ ] ArbiterReady: True (ArbiterConfigured)

### B.3.5 Verify Existing Resources Unaffected

```bash
oc get statefulset mongo-test -n $NAMESPACE && echo "PASS: StatefulSet still exists"
oc get mongocluster mongo-test -n $NAMESPACE -o jsonpath='{.status.phase}' && echo " (should still be Running)"
```

---

## Phase B.4: Disable Arbiter

### B.4.1 Disable Arbiter

```bash
oc patch mongocluster mongo-test -n $NAMESPACE --type merge -p '{"spec":{"arbiter":{"enabled":false}}}'
sleep 15

echo "=== Arbiter After Disable ==="
oc get deployment mongo-test-arbiter -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Deployment deleted" || echo "FAIL: Deployment still exists"

echo ""
echo "=== ArbiterReady After Disable ==="
oc get mongocluster mongo-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'ArbiterReady':
        print(f\"ArbiterReady: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**:
- [ ] Arbiter Deployment deleted
- [ ] ArbiterReady: False (ArbiterDisabled)

---

## Phase B.5: Idempotency

### B.5.1 Re-enable Arbiter and Re-reconcile

```bash
# Re-enable arbiter
oc patch mongocluster mongo-test -n $NAMESPACE --type merge -p '{"spec":{"arbiter":{"enabled":true}}}'
sleep 15

# Restart controller
oc delete pod -n $NAMESPACE -l control-plane=controller-manager
oc wait --for=condition=available deployment -l control-plane=controller-manager -n $NAMESPACE --timeout=60s
sleep 15

echo "Arbiter Deployments: $(oc get deployment -n $NAMESPACE 2>&1 | grep -c mongo-test-arbiter) (should be 1)"
echo "StatefulSets: $(oc get statefulset -n $NAMESPACE 2>&1 | grep -c mongo-test) (should be 1)"
```

**Expected**: Exactly 1 of each, no duplicates.

---

## Phase B.6: Delete CR with Arbiter

### B.6.1 Delete and Verify All Resources Cleaned

```bash
oc delete mongocluster mongo-test -n $NAMESPACE
sleep 15

oc get secret mongo-test-admin -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Admin Secret cleaned"
oc get secret mongo-test-keyfile -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: KeyFile Secret cleaned"
oc get configmap mongo-test-config -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: ConfigMap cleaned"
oc get service mongo-test-headless -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Headless Service cleaned"
oc get service mongo-test-client -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Client Service cleaned"
oc get statefulset mongo-test -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: StatefulSet cleaned"
oc get deployment mongo-test-arbiter -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Arbiter Deployment cleaned"
```

**Expected**:
- [ ] All 7 managed resources garbage collected (including Arbiter Deployment)

---

## Phase B.7: RBAC Verification

### B.7.1 Verify Deployment RBAC

```bash
oc auth can-i create deployments --as=system:serviceaccount:$NAMESPACE:mongodb-operator-controller-manager -n $NAMESPACE && echo "PASS: Can create Deployments" || echo "FAIL"
oc auth can-i delete deployments --as=system:serviceaccount:$NAMESPACE:mongodb-operator-controller-manager -n $NAMESPACE && echo "PASS: Can delete Deployments" || echo "FAIL"
```

**Expected**: Both return "yes".

---

## Phase B.8: OLM Bundle Validation

### B.8.1 Verify Bundle Version

```bash
echo "=== CSV Version ==="
grep 'name:.*mongodb-operator.v' bundle/manifests/mongodb-operator.clusterserviceversion.yaml | head -1
grep 'replaces:' bundle/manifests/mongodb-operator.clusterserviceversion.yaml
grep '^  version:' bundle/manifests/mongodb-operator.clusterserviceversion.yaml
```

**Expected**:
- [ ] CSV name: `mongodb-operator.v0.2.0`
- [ ] replaces: `mongodb-operator.v0.1.0`
- [ ] version: `0.2.0`

### B.8.2 Verify Arbiter Descriptors

```bash
grep -E 'arbiter' bundle/manifests/mongodb-operator.clusterserviceversion.yaml | grep 'path:' | head -5
```

**Expected**: specDescriptors for arbiter, arbiter.enabled.

### B.8.3 Verify Deployment RBAC in CSV

```bash
grep -A3 'deployments' bundle/manifests/mongodb-operator.clusterserviceversion.yaml | head -5
```

**Expected**: `apps/deployments` with CRUD verbs.

### B.8.4 Bundle Validate

```bash
operator-sdk bundle validate bundle/
```

**Expected**: No errors.

---

## Scenario B Cleanup

```bash
oc delete mongocluster --all -n $NAMESPACE
sleep 15

# If NOT continuing to Scenario C, undeploy:
# make undeploy                                                    # if deployed with make deploy
# operator-sdk cleanup mongodb-operator --namespace $NAMESPACE     # if deployed with OLM
# oc delete project $NAMESPACE
```

---

## Scenario B Summary Checklist

| # | Test | Phase | Expected |
|---|------|-------|----------|
| 1 | Operator deploys with v0.2.0 image | B.1 | Pod Running, CRD has arbiter field |
| 2 | All Scenario A resources work without arbiter | B.2 | 6 resources created |
| 3 | No arbiter Deployment when not configured | B.2 | Not found |
| 4 | Arbiter Deployment created when enabled | B.3 | 1 replica, port 27017 |
| 5 | Arbiter Deployment has correct owner ref | B.3 | MongoCluster |
| 6 | Arbiter Deployment has component=arbiter label | B.3 | Labels correct |
| 7 | No PVC on arbiter (vote-only) | B.3 | No volumes |
| 8 | ArbiterReady condition True | B.3 | ArbiterConfigured |
| 9 | Existing resources unaffected | B.3 | StatefulSet ok |
| 10 | Arbiter Deployment deleted when disabled | B.4 | Not found |
| 11 | ArbiterReady False when disabled | B.4 | ArbiterDisabled |
| 12 | Idempotent — no duplicate arbiter resources | B.5 | Exactly 1 each |
| 13 | All 7 resources cleaned on CR delete | B.6 | Including arbiter |
| 14 | Deployment RBAC works | B.7 | can-i returns yes |
| 15 | CSV version 0.2.0 with replaces | B.8 | Correct upgrade path |
| 16 | Arbiter descriptors in CSV | B.8 | arbiter.* fields present |
| 17 | Deployment RBAC in CSV | B.8 | apps/deployments |
| 18 | Bundle validates | B.8 | No errors |

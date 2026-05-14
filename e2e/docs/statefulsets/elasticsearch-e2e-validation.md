# E2E Validation Guide: Elasticsearch Operator on OpenShift

End-to-end validation of the Elasticsearch operator on a live OpenShift cluster. This is the **N=4 generality proof** — the 4th stateful workload operator validating that skills produce correct code without any further modifications, after PostgreSQL (111 tests, 17 fixes), Redis (139 tests, 0 fixes), and MongoDB (150 tests, 1 fix).

| Scenario | Feature | Bundle Version | Skills Tested | New Resource |
|----------|---------|----------------|---------------|-------------|
| A | Core operator (data nodes) | v0.1.0 | All 5 + all 3 subagents | StatefulSet, Service×2 (HTTP+transport), Secret, ConfigMap, CronJob |
| B | Dedicated master nodes | v0.2.0 | 4 (Workflow B) + 3 subagents | Deployment (master) |
| C | Webhooks + Network Security | v0.3.0 | 4 (Workflow C) + 3 subagents | NetworkPolicy |
| D | API Maturity + ILM Config | v0.4.0 | 4 (Workflow D) + 3 subagents | — (ILMSpec) |
| E | Same-group CRD (ElasticsearchIndex) | v0.5.0 | scaffolding B + all | ElasticsearchIndex CRD |

**Run scenarios in order** — each builds on the previous.

---

# Scenario A: Core Operator — Data Nodes (v0.1.0)

The operator was built using the mandatory workflow from CLAUDE.md:
- **Step 1** (Generate): `scaffolding-operator` SKILL (Workflow A)
- **Step 2** (Generate): `designing-operator-api` SKILL (Workflow A)
- **Step 3** (Generate): `implementing-reconciliation` SKILL (Workflow A)
- **Step 4a** (Test): `operator-test-generator` SUBAGENT
- **Step 4b** (Review): `operator-reviewer` SUBAGENT
- **Step 5** (Generate): `bundling-operator` SKILL (Workflow A)
- **Step 6** (Validate): `operator-bundle-validator` SUBAGENT

**Project stats**: 6 reconciler methods, 4 conditions (Available, Progressing, Degraded, BackupReady), 6 managed resources (Secret, ConfigMap, Service×2, StatefulSet, CronJob), 9 RBAC markers, all validation scripts pass.

**Key differences from PostgreSQL/Redis/MongoDB**: Two Services with different purposes (HTTP API on 9200 + transport/inter-node on 9300), two-port StatefulSet, CronJob backup with schedule (like PostgreSQL but with schedule update), elasticsearch.yml YAML config. Uses UBI micro mock container for E2E.

**Zero skill modifications required** — confirming N=4 generality.

## Prerequisites

- OpenShift 4.14+ cluster with cluster-admin access
- `oc` CLI logged in (`oc whoami` returns a user)
- `podman` for building images
- Access to a container registry (quay.io)
- A default StorageClass available (`oc get sc`)

## Environment Setup

```bash
export IMG=quay.io/mpaulgreen/elasticsearch-operator:v0.1.0
export BUNDLE_IMG=quay.io/mpaulgreen/elasticsearch-operator-bundle:v0.1.0
export NAMESPACE=elasticsearch-operator-system

cd e2e/elasticsearch-operator
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
sed -i '' "s|quay.io/mpaulgreen/elasticsearch-operator:v0.1.0|$IMG|g" bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml

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
oc get crd elasticsearchclusters.search.elasticsearch.example.com

# CRD has expected fields
oc get crd elasticsearchclusters.search.elasticsearch.example.com -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | python3 -c "import json,sys; print(sorted(json.load(sys.stdin).keys()))"

# Controller watching EventSources
oc logs -n $NAMESPACE -l control-plane=controller-manager --tail=20 | grep -E "Starting EventSource|Starting workers"
```

**Expected**:
- [ ] Pod 1/1 Running
- [ ] CRD `elasticsearchclusters.search.elasticsearch.example.com` installed
- [ ] CRD fields: auth, backup, replicas, resources, storage, version
- [ ] Controller watching EventSources for Secret, ConfigMap, Service, StatefulSet, CronJob

---

## Phase 2: Basic CR Lifecycle

### 2.1 Create a Minimal ElasticsearchCluster

```bash
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-test
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
EOF

sleep 30
```

### 2.2 Verify All 6 Managed Resources Created

```bash
echo "=== Managed Resources ==="
oc get secret es-test-auth -n $NAMESPACE && echo "PASS: Auth Secret" || echo "FAIL: Auth Secret"
oc get configmap es-test-config -n $NAMESPACE && echo "PASS: ConfigMap" || echo "FAIL: ConfigMap"
oc get service es-test-http -n $NAMESPACE && echo "PASS: HTTP Service" || echo "FAIL: HTTP Service"
oc get service es-test-transport -n $NAMESPACE && echo "PASS: Transport Service" || echo "FAIL: Transport Service"
oc get statefulset es-test -n $NAMESPACE && echo "PASS: StatefulSet" || echo "FAIL: StatefulSet"

echo ""
echo "=== No Backup CronJob (backup not enabled) ==="
oc get cronjob es-test-backup -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: No backup CronJob" || echo "FAIL: CronJob exists"

echo ""
echo "=== Owner References ==="
oc get secret es-test-auth -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be ElasticsearchCluster)"
oc get service es-test-http -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be ElasticsearchCluster)"
oc get statefulset es-test -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be ElasticsearchCluster)"

echo ""
echo "=== CR Status ==="
oc get elasticsearchcluster es-test -n $NAMESPACE -o wide
```

**Expected**:
- [ ] Secret `es-test-auth` created with `ELASTIC_USERNAME` and `ELASTIC_PASSWORD` keys
- [ ] ConfigMap `es-test-config` created with `elasticsearch.yml`
- [ ] HTTP Service `es-test-http` (ClusterIP, port 9200)
- [ ] Transport Service `es-test-transport` (ClusterIP: None, port 9300)
- [ ] StatefulSet `es-test` created with 3 replicas
- [ ] No backup CronJob (backup not enabled)
- [ ] All resources have ownerReferences pointing to ElasticsearchCluster

### 2.3 Verify Two Services (Elasticsearch-Specific)

```bash
echo "=== HTTP Service ==="
oc get service es-test-http -n $NAMESPACE -o jsonpath='{.spec.clusterIP}' && echo " (should NOT be None)"
oc get service es-test-http -n $NAMESPACE -o jsonpath='{.spec.ports[0].port}' && echo " port (should be 9200)"

echo ""
echo "=== Transport Service ==="
oc get service es-test-transport -n $NAMESPACE -o jsonpath='{.spec.clusterIP}' && echo " (should be None — headless)"
oc get service es-test-transport -n $NAMESPACE -o jsonpath='{.spec.ports[0].port}' && echo " port (should be 9300)"
```

**Expected**:
- [ ] HTTP service has a real ClusterIP (not None), port 9200
- [ ] Transport service has `clusterIP: None` (headless), port 9300

### 2.4 Verify StatefulSet Details

```bash
echo "=== StatefulSet Spec ==="
oc get statefulset es-test -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas"
oc get statefulset es-test -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].image}' && echo " image"
oc get statefulset es-test -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].ports[*].containerPort}' && echo " ports (should be 9200 9300)"
oc get statefulset es-test -n $NAMESPACE -o jsonpath='{.spec.volumeClaimTemplates[0].spec.resources.requests.storage}' && echo " storage"

echo ""
echo "=== Pod Status ==="
oc get pods -n $NAMESPACE -l app.kubernetes.io/instance=es-test
```

**Expected**:
- [ ] 3 replicas
- [ ] Image: `registry.access.redhat.com/ubi9/ubi-micro:latest` (E2E mock)
- [ ] Two container ports: 9200 + 9300
- [ ] PVC requests 1Gi storage

### 2.5 Verify Auth Secret Content

```bash
oc get secret es-test-auth -n $NAMESPACE -o jsonpath='{.data.ELASTIC_USERNAME}' | base64 -d && echo ""
oc get secret es-test-auth -n $NAMESPACE -o jsonpath='{.data.ELASTIC_PASSWORD}' | base64 -d && echo ""
```

**Expected**: Username=elastic, Password is random (24 chars).

### 2.6 Verify ConfigMap Content

```bash
oc get configmap es-test-config -n $NAMESPACE -o jsonpath='{.data.elasticsearch\.yml}'
```

**Expected**: Contains `cluster.name: es-test`, `http.port: 9200`, `transport.port: 9300`, `discovery.seed_hosts`.

### 2.7 Wait for Running Phase

```bash
oc wait --for=condition=ready pod -l app.kubernetes.io/instance=es-test -n $NAMESPACE --timeout=300s

oc get elasticsearchcluster es-test -n $NAMESPACE -o jsonpath='{.status}' | python3 -m json.tool
```

**Expected**:
- [ ] `phase: Running`
- [ ] `readyReplicas: 3`
- [ ] `currentVersion: "8.14"`
- [ ] `httpEndpoint: es-test-http.$NAMESPACE.svc.cluster.local:9200`

### 2.8 Verify Conditions

```bash
oc get elasticsearchcluster es-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -m json.tool
```

**Expected conditions**:
- [ ] `Available: True`
- [ ] `Progressing: False`
- [ ] `Degraded: False`
- [ ] `BackupReady: False` (backup not enabled)

### 2.9 Verify Print Columns

```bash
oc get elasticsearchcluster -n $NAMESPACE
```

**Expected**: Table with columns Phase, Ready, Version, Age.

### 2.10 Verify Events

```bash
oc get events -n $NAMESPACE --field-selector involvedObject.name=es-test --sort-by='.lastTimestamp'
```

**Expected**: Events for SecretCreated, ConfigMapCreated, HTTPServiceCreated, TransportServiceCreated, StatefulSetCreated.

---

## Phase 3: Idempotency

### 3.1 Verify No Duplicate Resources

```bash
oc delete pod -n $NAMESPACE -l control-plane=controller-manager
oc wait --for=condition=available deployment -l control-plane=controller-manager -n $NAMESPACE --timeout=60s
sleep 15

echo "Auth Secrets: $(oc get secret -n $NAMESPACE 2>&1 | grep -c es-test-auth) (should be 1)"
echo "ConfigMaps: $(oc get configmap -n $NAMESPACE 2>&1 | grep -c es-test-config) (should be 1)"
echo "HTTP Services: $(oc get service -n $NAMESPACE 2>&1 | grep -c es-test-http) (should be 1)"
echo "Transport Services: $(oc get service -n $NAMESPACE 2>&1 | grep -c es-test-transport) (should be 1)"
echo "StatefulSets: $(oc get statefulset -n $NAMESPACE 2>&1 | grep -c es-test) (should be 1)"
```

**Expected**: Exactly 1 of each resource, no duplicates.

### 3.2 Verify Password Unchanged After Reconciliation

```bash
PASS_BEFORE=$(oc get secret es-test-auth -n $NAMESPACE -o jsonpath='{.data.ELASTIC_PASSWORD}')

oc label elasticsearchcluster es-test -n $NAMESPACE test-reconcile=true
sleep 10

PASS_AFTER=$(oc get secret es-test-auth -n $NAMESPACE -o jsonpath='{.data.ELASTIC_PASSWORD}')
[ "$PASS_BEFORE" = "$PASS_AFTER" ] && echo "PASS: Password unchanged (idempotent)" || echo "FAIL: Password changed!"
```

---

## Phase 4: Scaling

### 4.1 Scale Up

```bash
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"replicas":5}}'
sleep 15

oc get statefulset es-test -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas (should be 5)"
```

**Expected**: StatefulSet replicas updated to 5.

### 4.2 Scale Down

```bash
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"replicas":1}}'
sleep 15

oc get statefulset es-test -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas (should be 1)"
```

**Expected**: Replicas updated to 1.

### 4.3 Scale Back to 3

```bash
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"replicas":3}}'
sleep 15

oc get statefulset es-test -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas (should be 3)"
```

---

## Phase 5: Backup CronJob

### 5.1 Enable Backup

```bash
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"backup":{"enabled":true,"schedule":"0 2 * * *","retentionDays":7}}}'
sleep 15
```

### 5.2 Verify Backup CronJob Created

```bash
echo "=== Backup CronJob ==="
oc get cronjob es-test-backup -n $NAMESPACE
oc get cronjob es-test-backup -n $NAMESPACE -o jsonpath='{.spec.schedule}' && echo " schedule (should be 0 2 * * *)"
oc get cronjob es-test-backup -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " owner (should be ElasticsearchCluster)"
```

**Expected**:
- [ ] CronJob `es-test-backup` created with schedule `0 2 * * *`
- [ ] Owner reference → ElasticsearchCluster

### 5.3 Verify BackupReady Condition

```bash
oc get elasticsearchcluster es-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'BackupReady':
        print(f\"BackupReady: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**: BackupReady: True (BackupConfigured).

### 5.4 Verify Schedule Update

```bash
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"backup":{"schedule":"0 4 * * *"}}}'
sleep 15

oc get cronjob es-test-backup -n $NAMESPACE -o jsonpath='{.spec.schedule}' && echo " (should be 0 4 * * *)"
```

**Expected**: CronJob schedule updated to `0 4 * * *` (check-update pattern).

### 5.5 Disable Backup

```bash
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"backup":{"enabled":false}}}'
sleep 15

echo "=== BackupReady After Disable ==="
oc get elasticsearchcluster es-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'BackupReady':
        print(f\"BackupReady: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**: BackupReady: False (BackupDisabled).

---

## Phase 6: Finalizer and Deletion

### 6.1 Verify Finalizer Exists

```bash
oc get elasticsearchcluster es-test -n $NAMESPACE -o jsonpath='{.metadata.finalizers}' && echo ""
```

**Expected**: `["search.elasticsearch.example.com/finalizer"]`

### 6.2 Delete the CR

```bash
oc delete elasticsearchcluster es-test -n $NAMESPACE
sleep 15

oc get secret es-test-auth -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Auth Secret cleaned"
oc get configmap es-test-config -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: ConfigMap cleaned"
oc get service es-test-http -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: HTTP Service cleaned"
oc get service es-test-transport -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Transport Service cleaned"
oc get statefulset es-test -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: StatefulSet cleaned"
oc get cronjob es-test-backup -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Backup CronJob cleaned"
```

**Expected**:
- [ ] All 6 managed resources garbage collected
- [ ] No orphaned resources remain

---

## Phase 7: Validation Markers

### 7.1 Reject Invalid Replicas

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-invalid
  namespace: $NAMESPACE
spec:
  replicas: 15
  version: "8.14"
  storage:
    size: 1Gi
EOF
```

**Expected**: Rejected — `replicas` max is 9.

### 7.2 Reject Invalid Version

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-invalid
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "7.17"
  storage:
    size: 1Gi
EOF
```

**Expected**: Rejected — version enum only allows "8.12", "8.14".

### 7.3 Reject Invalid Storage Size Pattern

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-invalid
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: "100MB"
EOF
```

**Expected**: Rejected — storage size must match pattern `^[0-9]+[KMGT]i$`.

### 7.4 Verify Defaults Applied

```bash
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-defaults
  namespace: $NAMESPACE
spec:
  storage:
    size: 5Gi
EOF

sleep 5
oc get elasticsearchcluster es-defaults -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " (should be 3)"
oc get elasticsearchcluster es-defaults -n $NAMESPACE -o jsonpath='{.spec.version}' && echo " (should be 8.14)"

oc delete elasticsearchcluster es-defaults -n $NAMESPACE
```

**Expected**: Defaults applied — replicas=3, version="8.14".

---

## Phase 8: RBAC Verification

### 8.1 Verify Operator RBAC Works

```bash
oc auth can-i get secrets --as=system:serviceaccount:$NAMESPACE:elasticsearch-operator-controller-manager && echo "PASS" || echo "FAIL"
oc auth can-i create statefulsets --as=system:serviceaccount:$NAMESPACE:elasticsearch-operator-controller-manager && echo "PASS" || echo "FAIL"
oc auth can-i create cronjobs --as=system:serviceaccount:$NAMESPACE:elasticsearch-operator-controller-manager && echo "PASS: CronJobs RBAC" || echo "FAIL"
oc auth can-i update elasticsearchclusters/status --as=system:serviceaccount:$NAMESPACE:elasticsearch-operator-controller-manager && echo "PASS" || echo "FAIL"
```

### 8.2 Verify No Excess Permissions

```bash
oc auth can-i create namespaces --as=system:serviceaccount:$NAMESPACE:elasticsearch-operator-controller-manager && echo "FAIL: excess perms" || echo "PASS: no excess"
oc auth can-i delete nodes --as=system:serviceaccount:$NAMESPACE:elasticsearch-operator-controller-manager && echo "FAIL: excess perms" || echo "PASS: no excess"
```

---

## Phase 9: Security Posture

### 9.1 Verify Non-Root Container

```bash
oc get deployment elasticsearch-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.securityContext.runAsNonRoot}' && echo " (should be true)"
```

### 9.2 Verify Capabilities Dropped

```bash
oc get deployment elasticsearch-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].securityContext.capabilities.drop}' && echo ""
```

**Expected**: `["ALL"]`

---

## Phase 10: Health Probes

### 10.1 Verify Liveness Probe

```bash
oc get deployment elasticsearch-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].livenessProbe}' | python3 -m json.tool
```

**Expected**: httpGet on /healthz:8081

### 10.2 Verify Readiness Probe

```bash
oc get deployment elasticsearch-operator-controller-manager -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].readinessProbe}' | python3 -m json.tool
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

### 12.1 Create Multiple ElasticsearchClusters

```bash
for i in 1 2 3; do
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-multi-$i
  namespace: $NAMESPACE
spec:
  replicas: 1
  version: "8.14"
  storage:
    size: 1Gi
EOF
done

sleep 30
oc get elasticsearchcluster -n $NAMESPACE
```

**Expected**:
- [ ] 3 independent ElasticsearchClusters created
- [ ] Each has its own Secret, ConfigMap, 2 Services, StatefulSet
- [ ] No cross-contamination

### 12.2 Delete One, Others Unaffected

```bash
oc delete elasticsearchcluster es-multi-2 -n $NAMESPACE
sleep 10

oc get elasticsearchcluster -n $NAMESPACE
oc get statefulset -n $NAMESPACE
```

**Expected**: es-multi-1 and es-multi-3 still Running, es-multi-2 fully cleaned up.

---

## Scenario A Cleanup

```bash
oc delete elasticsearchcluster --all -n $NAMESPACE
sleep 15

# If NOT continuing to Scenario B, undeploy the operator:
# make undeploy                                                    # if deployed with make deploy
# operator-sdk cleanup elasticsearch-operator --namespace $NAMESPACE  # if deployed with OLM
# oc delete project $NAMESPACE
```

---

## Scenario A Summary Checklist

| # | Test | Phase | Expected |
|---|------|-------|----------|
| 1 | Operator deploys and starts | 1 | Pod Running |
| 2 | CRD installed with expected fields | 1 | auth, backup, replicas, resources, storage, version |
| 3 | CR creates all 6 managed resources | 2.2 | Secret, ConfigMap, Service×2, StatefulSet, no CronJob |
| 4 | Owner references set on all resources | 2.2 | All point to ElasticsearchCluster |
| 5 | HTTP service has real ClusterIP, port 9200 | 2.3 | Not None |
| 6 | Transport service is headless, port 9300 | 2.3 | clusterIP: None |
| 7 | StatefulSet has correct image and 2 ports | 2.4 | UBI micro, 9200+9300 |
| 8 | Auth Secret has username + password | 2.5 | elastic, 24-char random |
| 9 | ConfigMap has elasticsearch.yml | 2.6 | cluster.name, http.port, transport.port |
| 10 | Status reaches Running phase | 2.7 | phase=Running, readyReplicas=3 |
| 11 | HttpEndpoint points to HTTP service | 2.7 | es-test-http...9200 |
| 12 | Conditions set correctly | 2.8 | Available=True, BackupReady=False |
| 13 | Print columns display | 2.9 | Phase, Ready, Version, Age |
| 14 | Events recorded for each resource | 2.10 | 5 create events |
| 15 | Idempotent — no duplicates | 3.1 | Exactly 1 of each |
| 16 | Password unchanged on re-reconcile | 3.2 | Same base64 value |
| 17 | Scale up works | 4.1 | 5 replicas |
| 18 | Scale down works | 4.2 | 1 replica |
| 19 | Backup CronJob created when enabled | 5.2 | schedule=0 2 * * *, ownerRef |
| 20 | BackupReady condition set | 5.3 | BackupConfigured |
| 21 | CronJob schedule updated | 5.4 | 0 4 * * * (check-update) |
| 22 | BackupReady False when disabled | 5.5 | BackupDisabled |
| 23 | Finalizer present | 6.1 | Finalizer in metadata |
| 24 | Deletion cleans all 6 resources | 6.2 | No orphans |
| 25 | Invalid replicas rejected | 7.1 | Validation error |
| 26 | Invalid version rejected | 7.2 | Validation error |
| 27 | Invalid storage pattern rejected | 7.3 | Validation error |
| 28 | Defaults applied | 7.4 | replicas=3, version=8.14 |
| 29 | RBAC allows needed operations (incl. batch/cronjobs) | 8.1 | can-i returns yes |
| 30 | No excess RBAC permissions | 8.2 | can-i returns no |
| 31 | Container runs as non-root | 9.1 | runAsNonRoot=true |
| 32 | Capabilities dropped | 9.2 | drop=[ALL] |
| 33 | Health probes configured | 10 | /healthz, /readyz |
| 34 | Bundle validates | 11 | No errors |
| 35 | Multiple instances independent | 12.1 | No cross-contamination |
| 36 | Deleting one doesn't affect others | 12.2 | Others still Running |

---
---

# Scenario B: Dedicated Master Nodes (v0.2.0)

Adds dedicated master-eligible nodes as a separate Deployment for cluster coordination and election. Unlike MongoDB Arbiter (always 1 replica), Elasticsearch master nodes are multi-replica (1-5, odd for quorum) with **two ports** (9200+9300). Built using:
- **Step 1** (Generate): `designing-operator-api` SKILL (Workflow B) — Added MasterSpec to types
- **Step 2** (Generate): `implementing-reconciliation` SKILL (Workflow B) — Added reconcileMaster (conditional Deployment)
- **Step 3a** (Test): `operator-test-generator` SUBAGENT (Workflow B) — Added master tests
- **Step 3b** (Review): `operator-reviewer` SUBAGENT — Reviewed modified code
- **Step 4** (Generate): `bundling-operator` SKILL (Workflow B) — Updated CSV v0.1.0 → v0.2.0
- **Step 5** (Validate): `operator-bundle-validator` SUBAGENT — Validated updated bundle

**Changes**: MasterSpec (enabled, replicas 1-5 odd, resources), reconcileMaster creating conditional multi-replica Deployment with two ports (9200+9300, no PVC), MasterReady condition, Deployment RBAC, check-update for replicas+resources, CSV v0.2.0 with replaces.

**Key differences from previous conditional Deployments**:

| Aspect | Redis Sentinel | MongoDB Arbiter | ES Master |
|--------|---------------|-----------------|-----------|
| Replicas | spec.sentinel.replicas (3-7) | Always 1 | spec.master.replicas (1-5, odd) |
| Ports | 26379 only | 27017 only | 9200 + 9300 (two ports) |
| PVC | No | No | No |
| Quorum | Odd for quorum | Vote-only | Odd for quorum |

**Prerequisites**: Scenario A completed successfully. All Scenario A CRs deleted.

## Scenario B Environment Setup

```bash
export IMG=quay.io/mpaulgreen/elasticsearch-operator:v0.2.0
export BUNDLE_IMG=quay.io/mpaulgreen/elasticsearch-operator-bundle:v0.2.0
export NAMESPACE=elasticsearch-operator-system

cd e2e/elasticsearch-operator
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
sed -i '' "s|quay.io/mpaulgreen/elasticsearch-operator:v0.2.0|$IMG|g" bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml

# Refresh CRD in bundle
make manifests
cp config/crd/bases/search.elasticsearch.example.com_elasticsearchclusters.yaml bundle/manifests/

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

# CRD has master field
oc get crd elasticsearchclusters.search.elasticsearch.example.com -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}' | python3 -c "import json,sys; print(sorted(json.load(sys.stdin).keys()))"

# Controller watching 7 EventSources (added Deployment)
oc logs -n $NAMESPACE -l control-plane=controller-manager --tail=20 | grep -E "Starting EventSource|Starting workers"
```

**Expected**:
- [ ] Pod 1/1 Running with v0.2.0 image
- [ ] CRD fields: auth, backup, master, replicas, resources, storage, version
- [ ] Controller watching 7 EventSources (added Deployment)

---

## Phase B.2: Existing Features Regression

### B.2.1 Create CR Without Master

```bash
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-test
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
EOF

sleep 30
```

### B.2.2 Verify All Scenario A Resources Created

```bash
echo "=== Managed Resources ==="
oc get secret es-test-auth -n $NAMESPACE && echo "PASS: Secret" || echo "FAIL"
oc get configmap es-test-config -n $NAMESPACE && echo "PASS: ConfigMap" || echo "FAIL"
oc get service es-test-http -n $NAMESPACE && echo "PASS: HTTP Service" || echo "FAIL"
oc get service es-test-transport -n $NAMESPACE && echo "PASS: Transport Service" || echo "FAIL"
oc get statefulset es-test -n $NAMESPACE && echo "PASS: StatefulSet" || echo "FAIL"

echo ""
echo "=== No Master (not configured) ==="
oc get deployment es-test-master -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: No Master Deployment" || echo "FAIL"

echo ""
echo "=== Status ==="
oc get elasticsearchcluster es-test -n $NAMESPACE -o wide
```

**Expected**:
- [ ] All 5 existing resources created (Secret, ConfigMap, Service×2, StatefulSet)
- [ ] No Master Deployment (master not configured)
- [ ] Status shows Running

---

## Phase B.3: Enable Master Nodes

### B.3.1 Enable Master

```bash
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"master":{"enabled":true,"replicas":3}}}'
sleep 15
```

### B.3.2 Verify Master Deployment Created

```bash
echo "=== Master Deployment ==="
oc get deployment es-test-master -n $NAMESPACE
oc get deployment es-test-master -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " replicas (should be 3)"

echo ""
echo "=== Master Ports (TWO — 9200+9300) ==="
oc get deployment es-test-master -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].ports[*].containerPort}' && echo " (should be 9200 9300)"

echo ""
echo "=== Master Deployment Owner Reference ==="
oc get deployment es-test-master -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be ElasticsearchCluster)"

echo ""
echo "=== Master Deployment Labels ==="
oc get deployment es-test-master -n $NAMESPACE -o jsonpath='{.metadata.labels}' | python3 -m json.tool
```

**Expected**:
- [ ] Deployment `es-test-master` created with 3 replicas (multi-replica, unlike MongoDB Arbiter's 1)
- [ ] Two ports: 9200 + 9300 (both HTTP and transport)
- [ ] Owner reference → ElasticsearchCluster
- [ ] Labels include `component: master`

### B.3.3 Verify No PVC on Master (Coordination-Only)

```bash
oc get deployment es-test-master -n $NAMESPACE -o jsonpath='{.spec.template.spec.volumes}' 2>/dev/null && echo " volumes" || echo "No volumes (correct — master nodes don't store data)"
```

**Expected**:
- [ ] No volumes/PVC on master Deployment (coordination-only, no data storage)

### B.3.4 Verify MasterReady Condition

```bash
oc get elasticsearchcluster es-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'MasterReady':
        print(f\"MasterReady: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**:
- [ ] MasterReady: True

### B.3.5 Verify Existing Resources Unaffected

```bash
oc get statefulset es-test -n $NAMESPACE && echo "PASS: StatefulSet still exists"
oc get elasticsearchcluster es-test -n $NAMESPACE -o jsonpath='{.status.phase}' && echo " (should still be Running)"
```

---

## Phase B.4: Disable Master

### B.4.1 Disable Master

```bash
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"master":{"enabled":false}}}'
sleep 15

echo "=== Master After Disable ==="
oc get deployment es-test-master -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Deployment deleted" || echo "FAIL: Deployment still exists"

echo ""
echo "=== MasterReady After Disable ==="
oc get elasticsearchcluster es-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'MasterReady':
        print(f\"MasterReady: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**:
- [ ] Master Deployment deleted
- [ ] MasterReady: False (MasterDisabled)

---

## Phase B.5: Idempotency

### B.5.1 Re-enable Master and Re-reconcile

```bash
# Re-enable master
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"master":{"enabled":true,"replicas":3}}}'
sleep 15

# Restart controller
oc delete pod -n $NAMESPACE -l control-plane=controller-manager
oc wait --for=condition=available deployment -l control-plane=controller-manager -n $NAMESPACE --timeout=60s
sleep 15

echo "Master Deployments: $(oc get deployment -n $NAMESPACE 2>&1 | grep -c es-test-master) (should be 1)"
echo "StatefulSets: $(oc get statefulset -n $NAMESPACE 2>&1 | grep -c es-test) (should be 1)"
```

**Expected**: Exactly 1 of each, no duplicates.

---

## Phase B.6: Delete CR with Master

### B.6.1 Delete and Verify All Resources Cleaned

```bash
oc delete elasticsearchcluster es-test -n $NAMESPACE
sleep 15

oc get secret es-test-auth -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Secret cleaned"
oc get configmap es-test-config -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: ConfigMap cleaned"
oc get service es-test-http -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: HTTP Service cleaned"
oc get service es-test-transport -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Transport Service cleaned"
oc get statefulset es-test -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: StatefulSet cleaned"
oc get deployment es-test-master -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Master Deployment cleaned"
```

**Expected**:
- [ ] All 6 managed resources garbage collected (including Master Deployment)

---

## Phase B.7: RBAC Verification

### B.7.1 Verify Deployment RBAC

```bash
oc auth can-i create deployments --as=system:serviceaccount:elasticsearch-operator-system:elasticsearch-operator-controller-manager -n elasticsearch-operator-system && echo "PASS: Can create Deployments" || echo "FAIL"
oc auth can-i delete deployments --as=system:serviceaccount:elasticsearch-operator-system:elasticsearch-operator-controller-manager -n elasticsearch-operator-system && echo "PASS: Can delete Deployments" || echo "FAIL"
```

**Expected**: Both return "yes".

---

## Phase B.8: OLM Bundle Validation

### B.8.1 Verify Bundle Version

```bash
echo "=== CSV Version ==="
grep 'name:.*elasticsearch-operator.v' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml | head -1
grep 'replaces:' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml
grep '^  version:' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml
```

**Expected**:
- [ ] CSV name: `elasticsearch-operator.v0.2.0`
- [ ] replaces: `elasticsearch-operator.v0.1.0`
- [ ] version: `0.2.0`

### B.8.2 Verify Master Descriptors

```bash
grep -E 'master' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml | grep 'path:' | head -5
```

**Expected**: specDescriptors for master, master.enabled, master.replicas.

### B.8.3 Verify Deployment RBAC in CSV

```bash
grep -A3 'deployments' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml | head -5
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
oc delete elasticsearchcluster --all -n $NAMESPACE
sleep 15

# If NOT continuing to Scenario C, undeploy:
# make undeploy                                                        # if deployed with make deploy
# operator-sdk cleanup elasticsearch-operator --namespace $NAMESPACE    # if deployed with OLM
# oc delete project $NAMESPACE
```

---

## Scenario B Summary Checklist

| # | Test | Phase | Expected |
|---|------|-------|----------|
| 1 | Operator deploys with v0.2.0 image | B.1 | Pod Running, CRD has master field |
| 2 | All Scenario A resources work without master | B.2 | 5 resources created |
| 3 | No master Deployment when not configured | B.2 | Not found |
| 4 | Master Deployment created when enabled | B.3 | 3 replicas (multi-replica) |
| 5 | Master Deployment has TWO ports (9200+9300) | B.3 | Both ports present |
| 6 | Master Deployment has correct owner ref | B.3 | ElasticsearchCluster |
| 7 | Master Deployment has component=master label | B.3 | Labels correct |
| 8 | No PVC on master (coordination-only) | B.3 | No volumes |
| 9 | MasterReady condition True | B.3 | MasterConfigured |
| 10 | Existing resources unaffected | B.3 | StatefulSet ok |
| 11 | Master Deployment deleted when disabled | B.4 | Not found |
| 12 | MasterReady False when disabled | B.4 | MasterDisabled |
| 13 | Idempotent — no duplicate master resources | B.5 | Exactly 1 each |
| 14 | All 6 resources cleaned on CR delete | B.6 | Including master |
| 15 | Deployment RBAC works | B.7 | can-i returns yes |
| 16 | CSV version 0.2.0 with replaces | B.8 | Correct upgrade path |
| 17 | Master descriptors in CSV | B.8 | master.* fields present |
| 18 | Deployment RBAC in CSV | B.8 | apps/deployments |
| 19 | Bundle validates | B.8 | No errors |

---
---

# Scenario C: Webhooks + Network Security (v0.3.0)

Adds admission webhooks (defaulting + validating) and a NetworkPolicy for network isolation. Built using:
- **Step 1** (Generate): `designing-operator-api` SKILL (Workflow C) — Webhook handler + 9 config files
- **Step 2** (Generate): `implementing-reconciliation` SKILL (Workflow B) — reconcileNetworkPolicy + NetworkSecured condition
- **Step 3a** (Test): `operator-test-generator` SUBAGENT (Workflow B) — NP + webhook tests
- **Step 3b** (Review): `operator-reviewer` SUBAGENT — Reviewed all changes (0 Critical)
- **Step 4** (Generate): `bundling-operator` SKILL (Workflow B) — CSV v0.3.0 with webhookdefinitions
- **Step 5** (Validate): `operator-bundle-validator` SUBAGENT — Validated bundle

**Changes**: Webhook handler (`Default()` + `ValidateCreate/Update/Delete()`), 9 webhook config files with kustomize replacements for cert-manager TLS, `reconcileNetworkPolicy()` (check-create, two ingress ports 9200+9300, DNS + intra-cluster egress), NetworkSecured condition, CSV v0.3.0 with replaces v0.2.0 + webhookdefinitions + networkpolicies RBAC.

**Key differences from prior Scenario C operators**:

| Aspect | PostgreSQL C | Redis C | MongoDB C | ES C |
|--------|-------------|---------|-----------|------|
| Ingress ports | 5432 | 6379+26379 | 27017 | 9200+9300 (two ports) |
| Egress replication | 5432 | 6379 | 27017 | 9200+9300 (two ports) |
| Quorum validation | N/A | sentinel odd | replicas odd | master.replicas odd |
| RetentionDays default | N/A | N/A | backup.retentionDays=7 | backup.retentionDays=7 |
| Webhook API version | v1alpha1 | v1alpha1 | v1alpha1 | v1alpha1 |

**Prerequisites**: Scenario B completed successfully. All Scenario B CRs deleted. Operator cleaned up from cluster.

## Scenario C Environment Setup

```bash
export IMG=quay.io/mpaulgreen/elasticsearch-operator:v0.3.0
export BUNDLE_IMG=quay.io/mpaulgreen/elasticsearch-operator-bundle:v0.3.0
export NAMESPACE=elasticsearch-operator-system

cd e2e/elasticsearch-operator
```

---

## Phase C.1: Build and Deploy v0.3.0

### C.1.1 Verify cert-manager is Running

```bash
oc get pods -n cert-manager
```

**Expected**: cert-manager, cert-manager-cainjector, cert-manager-webhook pods all Running. If not installed, install cert-manager first:
```bash
oc apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.5/cert-manager.yaml
oc wait --for=condition=available deployment -n cert-manager --all --timeout=120s
```

### C.1.2 Build the Operator Image

```bash
make manifests
podman build --platform linux/amd64 -t $IMG .
podman push $IMG
```

### C.1.3 Deploy the Operator

#### Option A: `make deploy` (Development — requires cert-manager)

```bash
make deploy IMG=$IMG
```

#### Option B: OLM

```bash
# Update CSV image reference
sed -i '' "s|quay.io/mpaulgreen/elasticsearch-operator:v0.3.0|$IMG|g" bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml

# Refresh CRD in bundle
cp config/crd/bases/search.elasticsearch.example.com_elasticsearchclusters.yaml bundle/manifests/

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
oc get pods -n $NAMESPACE -l control-plane=controller-manager
oc logs -n $NAMESPACE -l control-plane=controller-manager --tail=20

# Webhook Service exists
oc get service -n $NAMESPACE | grep webhook

# Webhook configurations registered
oc get mutatingwebhookconfigurations | grep elasticsearch
oc get validatingwebhookconfigurations | grep elasticsearch

# Controller watching 8 EventSources (added NetworkPolicy)
oc logs -n $NAMESPACE -l control-plane=controller-manager --tail=30 | grep -E "Starting EventSource|Starting workers"
```

**Expected**:
- [ ] Pod 1/1 Running with v0.3.0 image
- [ ] Webhook Service exists on port 443
- [ ] MutatingWebhookConfiguration and ValidatingWebhookConfiguration registered
- [ ] Controller watching 8 EventSources (added NetworkPolicy)

---

## Phase C.2: Existing Features Regression

### C.2.1 Create CR Without Master or Backup

```bash
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-test
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
EOF

sleep 30
```

### C.2.2 Verify All Scenario A Resources + NetworkPolicy Created

```bash
echo "=== Core Resources ==="
oc get secret es-test-auth -n $NAMESPACE && echo "PASS: Secret" || echo "FAIL"
oc get configmap es-test-config -n $NAMESPACE && echo "PASS: ConfigMap" || echo "FAIL"
oc get service es-test-http -n $NAMESPACE && echo "PASS: HTTP Service" || echo "FAIL"
oc get service es-test-transport -n $NAMESPACE && echo "PASS: Transport Service" || echo "FAIL"
oc get statefulset es-test -n $NAMESPACE && echo "PASS: StatefulSet" || echo "FAIL"

echo ""
echo "=== NetworkPolicy (always created) ==="
oc get networkpolicy es-test-network-policy -n $NAMESPACE && echo "PASS: NetworkPolicy" || echo "FAIL"

echo ""
echo "=== No Master/Backup (not configured) ==="
oc get deployment es-test-master -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: No Master" || echo "FAIL"
oc get cronjob es-test-backup -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: No Backup" || echo "FAIL"

echo ""
echo "=== Status ==="
oc get elasticsearchcluster es-test -n $NAMESPACE -o wide
```

**Expected**:
- [ ] All 5 core resources created (Secret, ConfigMap, Service×2, StatefulSet)
- [ ] NetworkPolicy `es-test-network-policy` created (always, not conditional)
- [ ] No master Deployment, no backup CronJob

---

## Phase C.3: Webhook Defaulting

### C.3.1 Defaults Applied for replicas and version

```bash
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-defaults
  namespace: $NAMESPACE
spec:
  storage:
    size: 1Gi
EOF

sleep 5
oc get elasticsearchcluster es-defaults -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " (should be 3)"
oc get elasticsearchcluster es-defaults -n $NAMESPACE -o jsonpath='{.spec.version}' && echo " (should be 8.14)"
```

**Expected**:
- [ ] Replicas defaulted to 3 (was 0/omitted)
- [ ] Version defaulted to "8.14" (was empty)

### C.3.2 backup.retentionDays Defaulted

```bash
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-backup-default
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
  backup:
    enabled: true
    schedule: "0 2 * * *"
EOF

sleep 5
oc get elasticsearchcluster es-backup-default -n $NAMESPACE -o jsonpath='{.spec.backup.retentionDays}' && echo " (should be 7)"
```

**Expected**:
- [ ] backup.retentionDays defaulted to 7

### C.3.3 master.replicas Defaulted

```bash
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-master-default
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
  master:
    enabled: true
EOF

sleep 5
oc get elasticsearchcluster es-master-default -n $NAMESPACE -o jsonpath='{.spec.master.replicas}' && echo " (should be 3)"
```

**Expected**:
- [ ] master.replicas defaulted to 3

### C.3.4 Cleanup Defaulting Test CRs

```bash
oc delete elasticsearchcluster es-defaults es-backup-default es-master-default -n $NAMESPACE
sleep 10
```

---

## Phase C.4: Webhook Validation (Create)

### C.4.1 Reject replicas < 1

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-bad
  namespace: $NAMESPACE
spec:
  replicas: -1
  version: "8.14"
  storage:
    size: 1Gi
EOF
```

**Expected**: Rejected — `replicas must be at least 1`.

### C.4.2 Reject Even master.replicas (Quorum)

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-bad
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
  master:
    enabled: true
    replicas: 4
EOF
```

**Expected**: Rejected — `master.replicas must be odd for quorum, got 4`.

### C.4.3 Reject Mutual Exclusion (auth.adminPassword + auth.existingSecret)

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-bad
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
  auth:
    adminPassword: "mypassword"
    existingSecret: "my-secret"
EOF
```

**Expected**: Rejected — `auth.adminPassword and auth.existingSecret are mutually exclusive`.

### C.4.4 Reject Backup Enabled Without Schedule

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-bad
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
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
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"storage":{"size":"500Mi"}}}' 2>&1
```

**Expected**: Rejected — `storage size cannot be reduced from 1Gi to 500Mi`.

### C.5.2 Allow Storage Size Increase

```bash
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"storage":{"size":"2Gi"}}}'
```

**Expected**: Accepted — storage can increase.

---

## Phase C.6: NetworkPolicy

### C.6.1 Verify NetworkPolicy Details

```bash
echo "=== NetworkPolicy ==="
oc get networkpolicy es-test-network-policy -n $NAMESPACE

echo ""
echo "=== Ingress Rules (should allow ports 9200+9300 from same namespace) ==="
oc get networkpolicy es-test-network-policy -n $NAMESPACE -o jsonpath='{.spec.ingress[0].ports[*].port}' && echo " (should be 9200 9300)"

echo ""
echo "=== Egress Rules ==="
echo "DNS egress:"
oc get networkpolicy es-test-network-policy -n $NAMESPACE -o jsonpath='{.spec.egress[0].ports[*].port}' && echo " (should be 53 53)"
echo "Intra-cluster egress:"
oc get networkpolicy es-test-network-policy -n $NAMESPACE -o jsonpath='{.spec.egress[1].ports[*].port}' && echo " (should be 9200 9300)"

echo ""
echo "=== PolicyTypes ==="
oc get networkpolicy es-test-network-policy -n $NAMESPACE -o jsonpath='{.spec.policyTypes}' && echo ""

echo ""
echo "=== Owner Reference ==="
oc get networkpolicy es-test-network-policy -n $NAMESPACE -o jsonpath='{.metadata.ownerReferences[0].kind}' && echo " (should be ElasticsearchCluster)"
```

**Expected**:
- [ ] Ingress allows ports 9200 (HTTP) + 9300 (transport) from same namespace
- [ ] Egress allows DNS (port 53 TCP+UDP) + intra-cluster replication (9200+9300)
- [ ] Owner reference → ElasticsearchCluster

### C.6.2 Verify NetworkSecured Condition

```bash
oc get elasticsearchcluster es-test -n $NAMESPACE -o jsonpath='{.status.conditions}' | python3 -c "
import json, sys
conditions = json.load(sys.stdin)
for c in conditions:
    if c['type'] == 'NetworkSecured':
        print(f\"NetworkSecured: {c['status']} (reason: {c['reason']})\")
"
```

**Expected**:
- [ ] NetworkSecured: True (NetworkSecured)

---

## Phase C.7: Idempotency

### C.7.1 Re-reconcile and Verify No Duplicates

```bash
oc delete pod -n $NAMESPACE -l control-plane=controller-manager
oc wait --for=condition=available deployment -l control-plane=controller-manager -n $NAMESPACE --timeout=60s
sleep 15

echo "NetworkPolicies: $(oc get networkpolicy -n $NAMESPACE 2>&1 | grep -c es-test-network-policy) (should be 1)"
echo "StatefulSets: $(oc get statefulset -n $NAMESPACE 2>&1 | grep -c es-test) (should be 1)"
echo "Secrets: $(oc get secret -n $NAMESPACE 2>&1 | grep -c es-test-auth) (should be 1)"
```

**Expected**: Exactly 1 of each, no duplicates.

---

## Phase C.8: Delete CR

### C.8.1 Delete and Verify All Resources Cleaned

```bash
oc delete elasticsearchcluster es-test -n $NAMESPACE
sleep 15

oc get secret es-test-auth -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Secret cleaned"
oc get configmap es-test-config -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: ConfigMap cleaned"
oc get service es-test-http -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: HTTP Service cleaned"
oc get service es-test-transport -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Transport Service cleaned"
oc get statefulset es-test -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: StatefulSet cleaned"
oc get networkpolicy es-test-network-policy -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: NetworkPolicy cleaned"
```

**Expected**:
- [ ] All 6 managed resources garbage collected (including NetworkPolicy)

---

## Phase C.9: RBAC Verification

### C.9.1 Verify NetworkPolicy RBAC

```bash
oc auth can-i create networkpolicies --as=system:serviceaccount:elasticsearch-operator-system:elasticsearch-operator-controller-manager -n elasticsearch-operator-system && echo "PASS: Can create NetworkPolicies" || echo "FAIL"
oc auth can-i delete networkpolicies --as=system:serviceaccount:elasticsearch-operator-system:elasticsearch-operator-controller-manager -n elasticsearch-operator-system && echo "PASS: Can delete NetworkPolicies" || echo "FAIL"
```

**Expected**: Both return "yes".

---

## Phase C.10: OLM Bundle Validation

### C.10.1 Verify Bundle Version

```bash
echo "=== CSV Version ==="
grep 'name:.*elasticsearch-operator.v' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml | head -1
grep 'replaces:' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml
grep '^  version:' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml
```

**Expected**:
- [ ] CSV name: `elasticsearch-operator.v0.3.0`
- [ ] replaces: `elasticsearch-operator.v0.2.0`
- [ ] version: `0.3.0`

### C.10.2 Verify Webhook Definitions in CSV

```bash
grep -A2 'webhookdefinitions' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml | head -5
grep 'webhookPath' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml
```

**Expected**: Both mutating (`/mutate-...`) and validating (`/validate-...`) webhook paths present.

### C.10.3 Verify NetworkPolicy RBAC in CSV

```bash
grep -A4 'networking.k8s.io' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml | head -5
```

**Expected**: `networking.k8s.io/networkpolicies` with CRUD verbs.

### C.10.4 Bundle Validate

```bash
operator-sdk bundle validate bundle/
```

**Expected**: No errors.

---

## Scenario C Cleanup

```bash
oc delete elasticsearchcluster --all -n $NAMESPACE
sleep 15

# If NOT continuing to Scenario D, undeploy:
# make undeploy                                                        # if deployed with make deploy
# operator-sdk cleanup elasticsearch-operator --namespace $NAMESPACE    # if deployed with OLM
# oc delete project $NAMESPACE
```

---

## Scenario C Summary Checklist

| # | Test | Phase | Expected |
|---|------|-------|----------|
| 1 | cert-manager running | C.1 | Pods Running |
| 2 | Operator deploys with v0.3.0 image | C.1 | Pod Running |
| 3 | Webhook Service + configurations registered | C.1 | Mutating + Validating |
| 4 | Controller watching 8 EventSources (added NP) | C.1 | 8 sources |
| 5 | All 5 core resources created | C.2 | Secret, ConfigMap, Service×2, StatefulSet |
| 6 | NetworkPolicy created (always, not conditional) | C.2 | es-test-network-policy |
| 7 | No master/backup when not configured | C.2 | Not found |
| 8 | Replicas defaulted to 3 when 0 | C.3 | Webhook defaulting |
| 9 | Version defaulted to "8.14" when empty | C.3 | Webhook defaulting |
| 10 | backup.retentionDays defaulted to 7 | C.3 | Webhook defaulting |
| 11 | master.replicas defaulted to 3 | C.3 | Webhook defaulting |
| 12 | Reject replicas < 1 | C.4 | Validation error |
| 13 | Reject even master.replicas (quorum) | C.4 | Validation error |
| 14 | Reject both auth.adminPassword + existingSecret | C.4 | Mutual exclusivity |
| 15 | Reject backup.enabled without schedule | C.4 | Validation error |
| 16 | Reject storage size reduction on update | C.5 | Validation error |
| 17 | Allow storage size increase | C.5 | Accepted |
| 18 | NetworkPolicy allows ports 9200+9300 ingress | C.6 | Two-port ingress |
| 19 | NetworkPolicy has DNS + intra-cluster egress | C.6 | DNS 53 + replication 9200+9300 |
| 20 | NetworkPolicy owner ref → ElasticsearchCluster | C.6 | Correct |
| 21 | NetworkSecured condition True | C.6 | NetworkSecured |
| 22 | Idempotent — no duplicate NetworkPolicies | C.7 | Exactly 1 |
| 23 | All 6 resources cleaned on CR delete | C.8 | Including NetworkPolicy |
| 24 | NetworkPolicy RBAC works | C.9 | can-i returns yes |
| 25 | CSV version 0.3.0 with replaces v0.2.0 | C.10 | Correct upgrade path |
| 26 | Webhook definitions in CSV | C.10 | Both mutating + validating |
| 27 | NetworkPolicy RBAC in CSV | C.10 | networking.k8s.io |
| 28 | Bundle validates | C.10 | No errors |

---
---

# Scenario D: API Maturity + ILM Configuration (v0.4.0)

Promotes the API to v1beta1 and adds Index Lifecycle Management (ILM) configuration. Built using:
- **Step 1** (Generate): `designing-operator-api` SKILL (Workflow D) — v1beta1 with storageversion, ILMSpec, maxShards, move webhook
- **Step 2** (Generate): `implementing-reconciliation` SKILL (Workflow B) — Controller imports v1beta1, ILMEnabled status
- **Step 3a** (Test): `operator-test-generator` SUBAGENT (Workflow B) — v1beta1 webhook tests (17 cases)
- **Step 3b** (Review): `operator-reviewer` SUBAGENT — 0 Critical, 0 Warnings, 20/20 checks
- **Step 4** (Generate): `bundling-operator` SKILL (Workflow B) — CSV v0.4.0 with maturity beta
- **Step 5** (Validate): `operator-bundle-validator` SUBAGENT — All checks pass

**Changes**: v1beta1 API with `+kubebuilder:storageversion`, ILMSpec (enabled, hotPhase, warmPhase, deletePhase), MaxShards (`*int32`), ILMEnabled status field, webhook moved from v1alpha1 to v1beta1 (v1alpha1 webhook deleted), new validation (ilm.enabled requires hotPhase), dual scheme registration in main.go, CSV v0.4.0 with replaces v0.3.0 + maturity beta + v1beta1 webhookdefinitions.

**Key Elasticsearch-specific aspects**:
- ILM is Elasticsearch's built-in index lifecycle feature (hot→warm→delete phases)
- maxShards controls `cluster.max_shards_per_node` cluster setting
- v1beta1 webhook path: `/mutate-search-elasticsearch-example-com-v1beta1-elasticsearchcluster`
- v1alpha1 types preserved (backward compatibility) but webhook removed (only v1beta1 webhook registered)

**Prerequisites**: Scenario C completed successfully. Operator cleaned up from cluster.

## Scenario D Environment Setup

```bash
export IMG=quay.io/mpaulgreen/elasticsearch-operator:v0.4.0
export BUNDLE_IMG=quay.io/mpaulgreen/elasticsearch-operator-bundle:v0.4.0
export NAMESPACE=elasticsearch-operator-system

cd e2e/elasticsearch-operator
```

---

## Phase D.1: Build and Deploy v0.4.0

### D.1.1 Build the Operator Image

```bash
make manifests
podman build --platform linux/amd64 -t $IMG .
podman push $IMG
```

### D.1.2 Deploy the Operator

#### Option A: `make deploy` (Development — requires cert-manager)

```bash
make deploy IMG=$IMG
```

#### Option B: OLM

```bash
# Update CSV image reference
sed -i '' "s|quay.io/mpaulgreen/elasticsearch-operator:v0.4.0|$IMG|g" bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml

# Refresh CRD in bundle
cp config/crd/bases/search.elasticsearch.example.com_elasticsearchclusters.yaml bundle/manifests/

# Build and push bundle
podman build -t $BUNDLE_IMG -f bundle.Dockerfile .
podman push $BUNDLE_IMG

# Create namespace first
oc new-project $NAMESPACE || oc create namespace $NAMESPACE

# Deploy via OLM
operator-sdk run bundle $BUNDLE_IMG --namespace $NAMESPACE --timeout 5m
```

### D.1.3 Verify Deployment

```bash
oc get pods -n $NAMESPACE -l control-plane=controller-manager

# CRD has v1beta1 with ILM and maxShards fields
oc get crd elasticsearchclusters.search.elasticsearch.example.com -o jsonpath='{.spec.versions}' | python3 -c "
import json, sys
versions = json.load(sys.stdin)
for v in versions:
    name = v['name']
    storage = v.get('storage', False)
    fields = sorted(v['schema']['openAPIV3Schema']['properties']['spec']['properties'].keys())
    print(f\"{name} (storage={storage}): {fields}\")
"

# Webhook registered with v1beta1 paths
oc get mutatingwebhookconfigurations -o yaml | grep 'elasticsearchcluster' | head -3
oc get validatingwebhookconfigurations -o yaml | grep 'elasticsearchcluster' | head -3
```

**Expected**:
- [ ] Pod 1/1 Running with v0.4.0 image
- [ ] CRD has v1beta1 (storage=True) with fields: auth, backup, ilm, master, maxShards, replicas, resources, storage, version
- [ ] Webhook paths use v1beta1 (not v1alpha1)

---

## Phase D.2: Existing Features Regression with v1beta1

### D.2.1 Create CR Using v1beta1 API

```bash
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1beta1
kind: ElasticsearchCluster
metadata:
  name: es-test
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
EOF

sleep 30
```

### D.2.2 Verify All Resources Created

```bash
echo "=== Core Resources ==="
oc get secret es-test-auth -n $NAMESPACE && echo "PASS: Secret" || echo "FAIL"
oc get configmap es-test-config -n $NAMESPACE && echo "PASS: ConfigMap" || echo "FAIL"
oc get service es-test-http -n $NAMESPACE && echo "PASS: HTTP Service" || echo "FAIL"
oc get service es-test-transport -n $NAMESPACE && echo "PASS: Transport Service" || echo "FAIL"
oc get statefulset es-test -n $NAMESPACE && echo "PASS: StatefulSet" || echo "FAIL"

echo ""
echo "=== NetworkPolicy ==="
oc get networkpolicy es-test-network-policy -n $NAMESPACE && echo "PASS: NetworkPolicy" || echo "FAIL"

echo ""
echo "=== Status ==="
oc get elasticsearchcluster es-test -n $NAMESPACE -o wide
```

**Expected**:
- [ ] All 5 core resources + NetworkPolicy created with v1beta1 API
- [ ] Status shows Running

---

## Phase D.3: v1beta1 New Fields

### D.3.1 Create CR with ILM Enabled

```bash
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1beta1
kind: ElasticsearchCluster
metadata:
  name: es-ilm
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
  ilm:
    enabled: true
    hotPhase: "30d"
    warmPhase: "90d"
    deletePhase: "365d"
  maxShards: 1000
EOF

sleep 15
```

### D.3.2 Verify ILM Fields and Status

```bash
echo "=== ILM Spec ==="
oc get elasticsearchcluster es-ilm -n $NAMESPACE -o jsonpath='{.spec.ilm}' | python3 -m json.tool

echo ""
echo "=== MaxShards ==="
oc get elasticsearchcluster es-ilm -n $NAMESPACE -o jsonpath='{.spec.maxShards}' && echo " (should be 1000)"

echo ""
echo "=== ILMEnabled Status ==="
oc get elasticsearchcluster es-ilm -n $NAMESPACE -o jsonpath='{.status.ilmEnabled}' && echo " (should be true)"
```

**Expected**:
- [ ] ILM spec has enabled=true, hotPhase=30d, warmPhase=90d, deletePhase=365d
- [ ] maxShards = 1000
- [ ] status.ilmEnabled = true

### D.3.3 Cleanup ILM Test CR

```bash
oc delete elasticsearchcluster es-ilm -n $NAMESPACE
sleep 10
```

---

## Phase D.4: Webhook Defaulting (v1beta1)

### D.4.1 Defaults Still Applied via v1beta1 Webhook

```bash
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1beta1
kind: ElasticsearchCluster
metadata:
  name: es-v1beta1-defaults
  namespace: $NAMESPACE
spec:
  storage:
    size: 1Gi
EOF

sleep 5
oc get elasticsearchcluster es-v1beta1-defaults -n $NAMESPACE -o jsonpath='{.spec.replicas}' && echo " (should be 3)"
oc get elasticsearchcluster es-v1beta1-defaults -n $NAMESPACE -o jsonpath='{.spec.version}' && echo " (should be 8.14)"

oc delete elasticsearchcluster es-v1beta1-defaults -n $NAMESPACE
sleep 10
```

**Expected**:
- [ ] Replicas defaulted to 3
- [ ] Version defaulted to "8.14"

---

## Phase D.5: Webhook Validation Create (v1beta1)

### D.5.1 Reject ILM Enabled Without hotPhase (New Validation)

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: search.elasticsearch.example.com/v1beta1
kind: ElasticsearchCluster
metadata:
  name: es-bad
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
  ilm:
    enabled: true
EOF
```

**Expected**: Rejected — `ilm.hotPhase is required when ilm.enabled is true`.

### D.5.2 Existing Validations Still Work (Even Master Quorum)

```bash
cat <<EOF | oc apply -f - 2>&1
apiVersion: search.elasticsearch.example.com/v1beta1
kind: ElasticsearchCluster
metadata:
  name: es-bad
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
  master:
    enabled: true
    replicas: 4
EOF
```

**Expected**: Rejected — `master.replicas must be odd for quorum, got 4`.

---

## Phase D.6: Webhook Validation Update (v1beta1)

### D.6.1 Reject Storage Size Reduction

```bash
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"storage":{"size":"500Mi"}}}' 2>&1
```

**Expected**: Rejected — `storage size cannot be reduced from 1Gi to 500Mi`.

### D.6.2 Allow Storage Size Increase

```bash
oc patch elasticsearchcluster es-test -n $NAMESPACE --type merge -p '{"spec":{"storage":{"size":"2Gi"}}}'
```

**Expected**: Accepted.

---

## Phase D.7: v1alpha1 Backward Compatibility

### D.7.1 Create CR Using v1alpha1 API Version

```bash
cat <<EOF | oc apply -f -
apiVersion: search.elasticsearch.example.com/v1alpha1
kind: ElasticsearchCluster
metadata:
  name: es-v1alpha1
  namespace: $NAMESPACE
spec:
  replicas: 3
  version: "8.14"
  storage:
    size: 1Gi
EOF

sleep 10
oc get elasticsearchcluster es-v1alpha1 -n $NAMESPACE -o wide
```

**Expected**:
- [ ] v1alpha1 CR accepted (API server converts to v1beta1 storage)

### D.7.2 Verify v1alpha1 CR Has No ILM Fields

```bash
oc get elasticsearchcluster es-v1alpha1 -n $NAMESPACE -o jsonpath='{.spec.ilm}' && echo " (should be empty)" || echo "No ILM (correct)"
oc get elasticsearchcluster es-v1alpha1 -n $NAMESPACE -o jsonpath='{.spec.maxShards}' && echo " (should be empty)" || echo "No maxShards (correct)"

oc delete elasticsearchcluster es-v1alpha1 -n $NAMESPACE
sleep 10
```

**Expected**:
- [ ] v1alpha1 CR does not have ilm or maxShards fields

---

## Phase D.8: Idempotency

### D.8.1 Re-reconcile and Verify No Duplicates

```bash
oc delete pod -n $NAMESPACE -l control-plane=controller-manager
oc wait --for=condition=available deployment -l control-plane=controller-manager -n $NAMESPACE --timeout=60s
sleep 15

echo "StatefulSets: $(oc get statefulset -n $NAMESPACE 2>&1 | grep -c es-test) (should be 1)"
echo "NetworkPolicies: $(oc get networkpolicy -n $NAMESPACE 2>&1 | grep -c es-test-network-policy) (should be 1)"
echo "Secrets: $(oc get secret -n $NAMESPACE 2>&1 | grep -c es-test-auth) (should be 1)"
```

**Expected**: Exactly 1 of each, no duplicates.

---

## Phase D.9: Delete CR

### D.9.1 Delete and Verify All Resources Cleaned

```bash
oc delete elasticsearchcluster es-test -n $NAMESPACE
sleep 15

oc get secret es-test-auth -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Secret cleaned"
oc get configmap es-test-config -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: ConfigMap cleaned"
oc get service es-test-http -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: HTTP Service cleaned"
oc get service es-test-transport -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: Transport Service cleaned"
oc get statefulset es-test -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: StatefulSet cleaned"
oc get networkpolicy es-test-network-policy -n $NAMESPACE 2>&1 | grep "not found" && echo "PASS: NetworkPolicy cleaned"
```

**Expected**:
- [ ] All 6 managed resources garbage collected

---

## Phase D.10: OLM Bundle Validation

### D.10.1 Verify Bundle Version and Maturity

```bash
echo "=== CSV Version ==="
grep 'name:.*elasticsearch-operator.v' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml | head -1
grep 'replaces:' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml
grep '^  version:' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml
grep 'maturity:' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml
```

**Expected**:
- [ ] CSV name: `elasticsearch-operator.v0.4.0`
- [ ] replaces: `elasticsearch-operator.v0.3.0`
- [ ] version: `0.4.0`
- [ ] maturity: `beta` (promoted from alpha)

### D.10.2 Verify v1beta1 Webhook Definitions Only

```bash
grep 'webhookPath' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml
```

**Expected**: Both paths use `v1beta1` (not v1alpha1).

### D.10.3 Verify ILM Descriptors in CSV

```bash
grep -E 'ilm|maxShards' bundle/manifests/elasticsearch-operator.clusterserviceversion.yaml | grep 'path:' | head -10
```

**Expected**: specDescriptors for ilm, ilm.enabled, ilm.hotPhase, ilm.warmPhase, ilm.deletePhase, maxShards + statusDescriptor for ilmEnabled.

### D.10.4 Bundle Validate

```bash
operator-sdk bundle validate bundle/
```

**Expected**: No errors.

---

## Scenario D Cleanup

```bash
oc delete elasticsearchcluster --all -n $NAMESPACE
sleep 15

# If NOT continuing to Scenario E, undeploy:
# make undeploy                                                        # if deployed with make deploy
# operator-sdk cleanup elasticsearch-operator --namespace $NAMESPACE    # if deployed with OLM
# oc delete project $NAMESPACE
```

---

## Scenario D Summary Checklist

| # | Test | Phase | Expected |
|---|------|-------|----------|
| 1 | Operator deploys with v0.4.0 image | D.1 | Pod Running |
| 2 | CRD has v1beta1 (storage) with ilm + maxShards fields | D.1 | 9 spec fields |
| 3 | Webhook paths use v1beta1 | D.1 | Not v1alpha1 |
| 4 | All resources created with v1beta1 API | D.2 | 5 core + NP |
| 5 | Status shows Running | D.2 | Phase=Running |
| 6 | ILM spec fields accepted (hotPhase, warmPhase, deletePhase) | D.3 | All present |
| 7 | maxShards field accepted | D.3 | 1000 |
| 8 | status.ilmEnabled = true when ILM enabled | D.3 | true |
| 9 | Replicas defaulted to 3 via v1beta1 webhook | D.4 | Defaulting works |
| 10 | Version defaulted to "8.14" via v1beta1 webhook | D.4 | Defaulting works |
| 11 | Reject ILM enabled without hotPhase | D.5 | New validation |
| 12 | Existing validations work (even master quorum) | D.5 | Carried forward |
| 13 | Reject storage size reduction | D.6 | Update validation |
| 14 | Allow storage size increase | D.6 | Accepted |
| 15 | v1alpha1 CR accepted (backward compatible) | D.7 | API server converts |
| 16 | v1alpha1 CR has no ILM/maxShards fields | D.7 | Not in v1alpha1 |
| 17 | Idempotent — no duplicates | D.8 | Exactly 1 each |
| 18 | All 6 resources cleaned on delete | D.9 | Including NP |
| 19 | CSV version 0.4.0 with replaces v0.3.0 | D.10 | Correct upgrade |
| 20 | Maturity promoted to beta | D.10 | alpha → beta |
| 21 | Webhook definitions use v1beta1 paths only | D.10 | Not v1alpha1 |
| 22 | ILM descriptors in CSV | D.10 | ilm.*, maxShards, ilmEnabled |
| 23 | Bundle validates | D.10 | No errors |

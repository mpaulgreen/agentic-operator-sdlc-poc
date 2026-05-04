# Sprint 8 Test Guide: `operator-bundle-validator` Subagent

## Prerequisites

- Sprints 1-7 complete (all 5 skills + 2 subagents built)
- The agent is at `.claude/agents/operator-bundle-validator.md`
- A redis-operator project with OLM bundle at `/tmp/redis-operator-test/` (from Test 5.1)
- PyYAML installed (`pip install pyyaml`)

## Test Order

1. **8.1**: Validate correct bundle
2. **8.2**: Validate bundle with deliberate issues (4 planted bugs)
3. **I-8**: Validate then fix

---

## Test 8.1 — Validate Correct Bundle

### Step 1: Ensure bundle exists

```bash
test -d /tmp/redis-operator-test/bundle/manifests && echo "PASS" || echo "FAIL: no bundle"
test -f /tmp/redis-operator-test/bundle/manifests/redis-operator.clusterserviceversion.yaml && echo "PASS: CSV" || echo "FAIL"
test -f /tmp/redis-operator-test/bundle/metadata/annotations.yaml && echo "PASS: annotations" || echo "FAIL"
test -f /tmp/redis-operator-test/bundle/tests/scorecard/config.yaml && echo "PASS: scorecard" || echo "FAIL"
test -f /tmp/redis-operator-test/bundle.Dockerfile && echo "PASS: Dockerfile" || echo "FAIL"
```

### Step 2: Prompt

```
Using the operator-bundle-validator agent, validate the OLM bundle at
/tmp/redis-operator-test/bundle/ for correctness and certification readiness.
```

### Step 3: Verify

```bash
# All 3 scripts should pass
bash .claude/skills/bundling-operator/scripts/validate-bundle-structure.sh /tmp/redis-operator-test/
python3 .claude/skills/bundling-operator/scripts/validate-csv.py /tmp/redis-operator-test/bundle/manifests/redis-operator.clusterserviceversion.yaml
python3 .claude/skills/bundling-operator/scripts/check-scorecard-readiness.py /tmp/redis-operator-test/bundle/

# Report should include certification checklist
```

### Acceptance Criteria

- [ ] Clean bundle passes all 3 validation scripts
- [ ] Each validation category reported separately (structure, CSV, scorecard)
- [ ] Certification checklist shows items met/not met
- [ ] Certification gaps identified (e.g., empty icon, missing links)
- [ ] Structured report output

---

## Test 8.2 — Validate Bundle with Deliberate Issues

### Step 1: Create flawed bundle

Copy the valid bundle and plant 4 issues:

```bash
rm -rf /tmp/redis-operator-flawed-bundle
cp -r /tmp/redis-operator-test /tmp/redis-operator-flawed-bundle
```

**Issue 1 — Missing annotations.yaml**:
```bash
rm /tmp/redis-operator-flawed-bundle/bundle/metadata/annotations.yaml
```

**Issue 2 — CSV missing installModes**:
Remove the `installModes` section from the CSV:
```bash
# Edit CSV to remove installModes block (lines containing installModes through AllNamespaces)
python3 -c "
import yaml
with open('/tmp/redis-operator-flawed-bundle/bundle/manifests/redis-operator.clusterserviceversion.yaml') as f:
    csv = yaml.safe_load(f)
del csv['spec']['installModes']
with open('/tmp/redis-operator-flawed-bundle/bundle/manifests/redis-operator.clusterserviceversion.yaml', 'w') as f:
    yaml.dump(csv, f, default_flow_style=False, sort_keys=False)
"
```

**Issue 3 — Invalid alm-examples JSON**:
```bash
# Replace alm-examples with invalid JSON
python3 -c "
import yaml
with open('/tmp/redis-operator-flawed-bundle/bundle/manifests/redis-operator.clusterserviceversion.yaml') as f:
    csv = yaml.safe_load(f)
csv['metadata']['annotations']['alm-examples'] = '[{invalid json'
with open('/tmp/redis-operator-flawed-bundle/bundle/manifests/redis-operator.clusterserviceversion.yaml', 'w') as f:
    yaml.dump(csv, f, default_flow_style=False, sort_keys=False)
"
```

**Issue 4 — specDescriptor referencing non-existent field**:
```bash
# Add a descriptor for a field that doesn't exist in the CRD
python3 -c "
import yaml
with open('/tmp/redis-operator-flawed-bundle/bundle/manifests/redis-operator.clusterserviceversion.yaml') as f:
    csv = yaml.safe_load(f)
csv['spec']['customresourcedefinitions']['owned'][0]['specDescriptors'].append({
    'description': 'Non-existent field',
    'displayName': 'Ghost Field',
    'path': 'nonExistentField'
})
with open('/tmp/redis-operator-flawed-bundle/bundle/manifests/redis-operator.clusterserviceversion.yaml', 'w') as f:
    yaml.dump(csv, f, default_flow_style=False, sort_keys=False)
"
```

### Step 2: Prompt

```
Using the operator-bundle-validator agent, validate the bundle at 
/tmp/redis-operator-flawed-bundle/bundle/ and report all issues found.
```

### Step 3: Verify

```bash
# Structure script should FAIL (missing annotations.yaml)
bash .claude/skills/bundling-operator/scripts/validate-bundle-structure.sh /tmp/redis-operator-flawed-bundle/

# CSV script should FAIL (missing installModes, invalid alm-examples)
python3 .claude/skills/bundling-operator/scripts/validate-csv.py /tmp/redis-operator-flawed-bundle/bundle/manifests/redis-operator.clusterserviceversion.yaml

# Scorecard script should detect non-existent descriptor path
python3 .claude/skills/bundling-operator/scripts/check-scorecard-readiness.py /tmp/redis-operator-flawed-bundle/bundle/
```

### Acceptance Criteria

- [ ] Issue 1 detected: missing annotations.yaml (FAIL from structure script)
- [ ] Issue 2 detected: missing installModes (FAIL from CSV script)
- [ ] Issue 3 detected: invalid alm-examples JSON (FAIL from CSV script)
- [ ] Issue 4 detected: non-existent descriptor path (WARN from scorecard script)
- [ ] Each issue has actionable fix suggestion
- [ ] Issues categorized by severity

---

## Test I-8 — Validate Then Fix (Integration)

### Step 1: Ensure flawed bundle exists from Test 8.2

### Step 2: Prompt

```
Validate the bundle at /tmp/redis-operator-flawed-bundle/bundle/. For any 
issues found, fix them using the bundling-operator skill and re-validate.
```

### Step 3: Verify

```bash
# After fixes, all 3 scripts should pass
bash .claude/skills/bundling-operator/scripts/validate-bundle-structure.sh /tmp/redis-operator-flawed-bundle/
python3 .claude/skills/bundling-operator/scripts/validate-csv.py /tmp/redis-operator-flawed-bundle/bundle/manifests/redis-operator.clusterserviceversion.yaml
python3 .claude/skills/bundling-operator/scripts/check-scorecard-readiness.py /tmp/redis-operator-flawed-bundle/bundle/
```

### Acceptance Criteria

- [ ] Issues identified in validation report
- [ ] Fixes applied (annotations restored, installModes added, alm-examples fixed, bad descriptor removed)
- [ ] Re-validation passes all 3 scripts

---

## Cleanup

```bash
rm -rf /tmp/redis-operator-flawed-bundle
```

## Troubleshooting

| Issue | Cause | Fix |
|-------|-------|-----|
| validate-csv.py: no yaml module | PyYAML missing | `pip install pyyaml` |
| Structure script fails on Dockerfile | bundle.Dockerfile at wrong path | Must be at project root |
| Scorecard warns on descriptors | Descriptor path doesn't match CRD | Check CRD spec.properties for valid paths |
| CSV name mismatch | metadata.name doesn't match version | Ensure `<pkg>.v<version>` pattern |

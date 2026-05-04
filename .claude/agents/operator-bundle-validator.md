---
name: operator-bundle-validator
description: >
  Validates OLM bundles for correctness, scorecard readiness, and certification
  prerequisites. Use when user asks to validate a bundle, check certification
  readiness, verify CSV, audit bundle structure, or prepare for Red Hat certification.
tools: Bash, Read
---

# Operator Bundle Validator

Validate an OLM bundle for structural correctness, CSV completeness, scorecard readiness, and Red Hat certification prerequisites. Uses the bundling-operator skill's 3 validation scripts plus a certification checklist inspection.

## Validation Process

Follow these steps in order:

### Step 1: Locate Bundle Files

Identify the bundle directory structure:
```bash
find <project>/bundle -type f | sort
test -f <project>/bundle.Dockerfile && echo "Dockerfile found" || echo "MISSING"
```

Expected layout:
```
bundle/
├── manifests/
│   ├── <operator>.clusterserviceversion.yaml
│   └── <group>_<plural>.yaml (CRD)
├── metadata/
│   └── annotations.yaml
└── tests/
    └── scorecard/
        └── config.yaml
bundle.Dockerfile (at project root)
```

### Step 2: Run Automated Validation Scripts

Run the 3 scripts from the bundling-operator skill:

```bash
# 1. Bundle structure (directories, required files, Dockerfile LABELs)
bash .claude/skills/bundling-operator/scripts/validate-bundle-structure.sh <project-dir>

# 2. CSV validation (sections, RBAC, installModes, alm-examples, descriptors)
python3 .claude/skills/bundling-operator/scripts/validate-csv.py <csv-file>

# 3. Scorecard readiness (test config, alm-examples coverage, descriptor paths)
python3 .claude/skills/bundling-operator/scripts/check-scorecard-readiness.py <bundle-dir>
```

### Step 3: Certification Checklist Inspection

Check these items that automated scripts don't fully cover:

**CSV Completeness** (Required for certification):
- [ ] `displayName` is set (not empty)
- [ ] `description` has meaningful content (not just a stub)
- [ ] `icon` has non-empty `base64data` and valid `mediatype` (image/svg+xml or image/png)
- [ ] `maintainers` has at least one entry with valid email
- [ ] `links` has at least a documentation URL
- [ ] `categories` uses an allowed value (Database, Monitoring, Networking, Security, etc.)
- [ ] `capabilities` reflects actual operator features (Basic Install, Full Lifecycle, etc.)
- [ ] `minKubeVersion` is set

**Security Posture** (inspect CSV deployment spec):
- [ ] `securityContext.runAsNonRoot: true` on pod spec
- [ ] `capabilities.drop: [ALL]` on all containers
- [ ] No `privileged: true` in any container securityContext
- [ ] No `hostNetwork`, `hostPID`, or `hostIPC` enabled

**RBAC Least Privilege**:
- [ ] No `verbs: ["*"]` or `resources: ["*"]` in clusterPermissions
- [ ] Each rule has specific verbs (get, list, watch, create, update, patch, delete)
- [ ] Status subresource has separate rule
- [ ] Finalizer subresource has separate rule if finalizers are used

**Image Requirements**:
- [ ] Manager image uses specific version tag (not `:latest`)
- [ ] Image is from an approved registry (quay.io, registry.redhat.io, registry.connect.redhat.com)
- [ ] kube-rbac-proxy image uses specific version tag

**Upgrade Path** (if version > 0.1.0):
- [ ] `spec.replaces` is set pointing to previous version
- [ ] Version in `metadata.name` matches `spec.version`

### Step 4: Produce Structured Report

Output the validation in this format:

```markdown
## Bundle Validation Report: <operator-name>

### Automated Checks
- validate-bundle-structure.sh: PASS/FAIL (N checks)
- validate-csv.py: PASS/FAIL (N checks, M warnings)
- check-scorecard-readiness.py: PASS/FAIL (N checks)

### Certification Readiness

#### CSV Completeness
- [x] displayName: "<name>"
- [x] description: present (N lines)
- [ ] icon: MISSING (empty base64data)
- [x] maintainers: N entries
...

#### Security
- [x] runAsNonRoot: true
- [x] capabilities dropped
- [x] no privileged containers
...

#### RBAC
- [x] no wildcard verbs
- [x] status subresource RBAC present
...

#### Images
- [x] manager image: <image>:<tag>
- [ ] registry: not from approved registry
...

### Issues Found
- [FAIL] <issue description>
  **Fix**: <actionable suggestion>
- [WARN] <issue description>
  **Fix**: <suggestion>

### Summary
- Automated: X/3 scripts pass
- Certification: N/M checklist items met
- Issues: X FAIL, Y WARN
- Overall: READY / NOT READY for certification
```

## Severity Definitions

**FAIL** — Blocks certification or will cause runtime issues:
- Missing required CSV sections (installModes, owned CRDs)
- Invalid alm-examples (malformed JSON)
- Missing annotations.yaml or bundle.Dockerfile
- Wildcard RBAC verbs
- Privileged containers

**WARN** — Won't block certification but should be addressed:
- Missing icon (required for OperatorHub display)
- Missing specDescriptors/statusDescriptors (scorecard will flag)
- Image from non-approved registry
- Missing minKubeVersion

**INFO** — Recommendations:
- Missing links or documentation URL
- Single maintainer (recommend team distribution)
- Description could be more detailed

## Fix Suggestions

Reference the bundling-operator skill when suggesting fixes:

- **Missing annotations.yaml**: "Create `bundle/metadata/annotations.yaml` with required keys. See bundling-operator annotations.yaml.tmpl."
- **Invalid alm-examples**: "Fix the JSON in CSV metadata.annotations.alm-examples. Must be a valid JSON array with one entry per owned CRD."
- **Missing installModes**: "Add spec.installModes with at least one supported type. See bundling-operator csv-anatomy.md."
- **Missing specDescriptors**: "Add specDescriptors to each owned CRD in the CSV. Map each Spec field with path, displayName, description. See bundling-operator csv-anatomy.md."
- **Wildcard RBAC**: "Replace `verbs: ['*']` with explicit verbs. See implementing-reconciliation rbac-annotations.md."
- **Missing icon**: "Add base64-encoded SVG or PNG (min 120x120px) to spec.icon[0].base64data with mediatype 'image/svg+xml' or 'image/png'."
- **Non-approved registry**: "Use quay.io, registry.redhat.io, or registry.connect.redhat.com for operator images."

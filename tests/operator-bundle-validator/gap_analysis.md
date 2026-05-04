# Sprint 8 Gap Analysis: `operator-bundle-validator` Subagent

## What the Validator Checks

The subagent combines automated script validation with a certification checklist inspection.

### Automated (via bundling-operator skill scripts)

| Script | Checks | What It Catches |
|--------|--------|-----------------|
| validate-bundle-structure.sh | 13 | Missing dirs, CSV, CRD, annotations, Dockerfile, LABELs |
| validate-csv.py | 20+ | apiVersion, name pattern, version, alm-examples, CRDs, RBAC, deployments, installModes, descriptors |
| check-scorecard-readiness.py | 10+ | Scorecard config, alm-examples coverage, descriptor paths vs CRD schema |

### Manual (agent inspection)

| Area | Checks | What It Catches |
|------|--------|-----------------|
| CSV completeness | 8 | Empty icon, missing maintainers, no links, no minKubeVersion |
| Security posture | 4 | Privileged containers, missing runAsNonRoot, capabilities not dropped |
| RBAC | 3 | Wildcard verbs, missing status/finalizer rules |
| Images | 3 | :latest tag, non-approved registry |
| Upgrade path | 2 | Missing replaces field, version mismatch |

## Comparison: Validator vs `operator-sdk bundle validate`

| Aspect | `operator-sdk bundle validate` | Subagent |
|--------|-------------------------------|----------|
| Bundle structure | Checks directory layout | Same + Dockerfile LABEL sync |
| CSV required fields | Basic presence check | Detailed per-section analysis |
| Scorecard readiness | Not checked | Pre-flight descriptor path validation |
| Certification checklist | Not checked | Full security + image + RBAC audit |
| Fix suggestions | None | Actionable fixes referencing skill docs |
| alm-examples | Validates JSON | Validates JSON + checks CRD coverage |
| Descriptor paths | Not validated | Cross-checked against CRD schema |
| Availability | Requires operator-sdk binary | Python + Bash scripts (no binary deps) |

## What the Validator Does NOT Check

| Area | Why Not | Where It's Checked |
|------|---------|-------------------|
| Actual scorecard test execution | Requires live cluster | `operator-sdk scorecard bundle/` on cluster |
| Container image signing | Requires registry access | Red Hat Preflight scan |
| UBI base image verification | Requires image inspection | `preflight check operator` |
| Operator install/upgrade | Requires live cluster | E2E tests |
| API backward compatibility | Complex analysis | Manual review |

## Summary

The subagent provides ~80% of the validation needed before submitting for certification, without requiring a live cluster or the `operator-sdk` binary. The remaining 20% (scorecard execution, image signing, actual install testing) requires cluster access and happens during the Red Hat certification pipeline.

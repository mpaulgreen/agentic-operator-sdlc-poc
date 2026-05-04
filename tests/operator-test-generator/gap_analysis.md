# Sprint 7 Gap Analysis: `operator-test-generator` Subagent

## What the Test Generator Does

The subagent automates the full test generation workflow: discovery, generation, validation, and reporting.

### Automated Workflow

| Step | Manual Approach | Subagent Approach |
|------|----------------|-------------------|
| Discover methods | Read code, list methods manually | Parse reconcileX methods via grep |
| Generate suite_test.go | Copy template, fill in scheme/CRD paths | Generate from testing-operator template patterns |
| Generate test cases | Write per-method tests by hand | Generate create + idempotent tests per method |
| Validate compilation | Run `go vet` manually | Run automatically, report results |
| Check coverage | Manually inspect test file | Run generate-test-matrix.py automatically |

### Time Comparison

| Task | Manual | Subagent |
|------|--------|----------|
| Full test suite (4 methods) | 30-60 min | 2-5 min |
| Incremental (1 new method) | 10-15 min | 1-2 min |
| Coverage verification | 5 min | Automatic |

## What the Subagent Does NOT Do

| Area | Why Not |
|------|---------|
| Run actual envtest tests | Requires envtest binaries (k8s API server + etcd) |
| E2E tests | Requires real cluster |
| Mutation testing | Out of scope for this project |
| Performance testing | Not applicable to operator controllers |

## Comparison vs operator-sdk Test Generation

| Aspect | operator-sdk | Subagent |
|--------|-------------|----------|
| Test files generated | 3 (suite, controller, e2e) | 2 (suite, controller) |
| Test cases per method | 0 (empty stub) | 2 (create + idempotent) |
| Lifecycle tests | 1 (basic reconcile) | 4 (finalizer, create all, idempotent, deletion) |
| Helper tests | 0 | 2 (labels, password) |
| Content verification | No | Yes (checks data, not just existence) |
| Coverage validation | No | Yes (generate-test-matrix.py) |
| Incremental mode | No | Yes (add tests for single new method) |

## Summary

The subagent generates 14x more test cases than operator-sdk (14 vs 1) and validates coverage automatically. The main gap vs manual test writing is that it can't run the actual tests (envtest) or handle complex interaction scenarios. For the 80% case (per-method create/idempotent), it eliminates all the boilerplate.

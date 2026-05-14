# Agentic Operator POC

Build OpenShift operators using Claude Agentic Skills instead of the traditional `operator-sdk` CLI workflow.

## What This Is

A composable set of **5 skills + 3 subagents** that replace the manual, repetitive steps of operator development — scaffolding, CRD design, controller reconciliation, testing, and OLM bundling — with guided, template-driven AI workflows.

Each skill is validated against `operator-sdk` v1.37.0 output to ensure structural compatibility while producing significantly more useful code (real validation markers vs empty stubs, production-ready controllers vs TODO comments).

## Architecture

```
Developer
    │
    ├── scaffolding-operator         → Project init (replaces operator-sdk init/create api)
    ├── designing-operator-api       → CRD types, markers, webhooks, API versioning
    ├── implementing-reconciliation  → Controller logic with idempotency patterns
    ├── testing-operator             → envtest + Ginkgo test generation
    └── bundling-operator            → OLM bundle, CSV, certification readiness
```

Three subagents (`operator-reviewer`, `operator-test-generator`, `operator-bundle-validator`) combine skills for specialized workflows like code review and bundle validation.

See [architecture.md](architecture.md) for the full design rationale, directory structures, and composition patterns.

## Current Status

| Sprint | Skill | Files | Status |
|--------|-------|-------|--------|
| 1 | `scaffolding-operator` | 29 | Done — 4 patterns (new project, same-group, cluster-scoped, multi-group) |
| 2 | `designing-operator-api` | 24 | Done — types, markers, webhooks, API versioning |
| 3 | `implementing-reconciliation` | 19 | Done — three-phase reconciliation, idempotency, finalizers, conditions |
| 4 | `testing-operator` | 12 | Done — envtest + Ginkgo test generation, per-method coverage |
| 5 | `bundling-operator` | 15 | Done — OLM bundle, CSV, scorecard, certification readiness |
| 6 | `operator-reviewer` (subagent) | 1 | Done — code review, composes skills 2+3, validated against ACM operator |
| 7 | `operator-test-generator` (subagent) | 1 | Done — test generation, uses skill 4, 100% method coverage |
| 8 | `operator-bundle-validator` (subagent) | 1 | Done — bundle validation, certification readiness, uses skill 5 |

**99 skill files + 3 subagents** — all 8 sprints complete. 9 validation scripts, validated against operator-sdk and ACM.

## Project Structure

```
agentic-operator-poc/
├── architecture.md              # Full architecture and design decisions
├── CLAUDE.md                    # Project context for Claude Code sessions
├── references/                  # Research and planning documents
│   ├── openshift-operator-research.md
│   └── development-plan.md     # Sprint plan + E2E validation categories
├── tests/                       # Test guides and gap analyses per skill
│   ├── scaffolding-operator/
│   ├── designing-operator-api/
│   ├── implementing-reconciliation/
│   ├── testing-operator/
│   ├── bundling-operator/
│   ├── operator-reviewer/
│   ├── operator-test-generator/
│   └── operator-bundle-validator/
├── e2e/                         # E2E scenario tests (by operator category)
│   ├── postgres-operator/       # PostgreSQL operator (Scenarios A-D, v0.1.0→v0.4.0)
│   ├── redis-operator/          # Redis operator (Scenarios A-E, v0.1.0→v0.5.0)
│   ├── mongodb-operator/        # MongoDB operator (Scenarios A-E, v0.1.0→v0.5.0)
│   ├── elasticsearch-operator/  # Elasticsearch operator (Scenarios A-C, v0.1.0→v0.3.0)
│   └── docs/
│       └── statefulsets/
│           ├── postgres-prompts.md              # Scenario prompts + acceptance criteria
│           ├── postgres-e2e-validation.md       # OpenShift validation guide (111 tests)
│           ├── redis-prompts.md                 # Redis scenario prompts
│           ├── redis-e2e-validation.md          # Redis OpenShift validation guide (139 tests)
│           ├── mongodb-prompts.md               # MongoDB scenario prompts
│           ├── mongodb-e2e-validation.md        # MongoDB OpenShift validation guide (150 tests)
│           ├── elasticsearch-prompts.md         # Elasticsearch scenario prompts
│           └── elasticsearch-e2e-validation.md  # Elasticsearch OpenShift validation guide (83 tests)
├── .claude/
│   ├── agents/                  # Subagent definitions
│   │   ├── operator-reviewer.md
│   │   ├── operator-test-generator.md
│   │   └── operator-bundle-validator.md
│   └── skills/                  # Skill implementations
│       ├── scaffolding-operator/       (29 files)
│       ├── designing-operator-api/     (24 files)
│       ├── implementing-reconciliation/ (19 files)
│       ├── testing-operator/           (12 files)
│       └── bundling-operator/          (15 files)
```

## How to Use

### With Claude Code

The skills are in `.claude/skills/`. When working in a project that has these skills, ask Claude to scaffold, design, implement, test, or bundle an operator:

```
"Create a new operator that manages PostgreSQL clusters on OpenShift"
"Add a BackupSchedule CRD to my existing operator"
"Implement reconciliation for my controller with Secret, Service, and StatefulSet"
"Add webhooks with defaulting and validation to my CRD"
```

### End-to-End Example

Build a complete operator in 3 steps:

```
Step 1 (scaffolding-operator): "Create a notification-operator project with 
domain notify.example.com, group notify, kind NotificationChannel"

Step 2 (designing-operator-api): "Design the CRD with type enum email/slack/pagerduty, 
endpoint string, retryCount 1-5 default 3, conditions in status"

Step 3 (implementing-reconciliation): "Implement the controller reconciling 
Secret for API credentials and Deployment for the notification worker"

Step 4 (testing-operator): "Generate a complete test suite with envtest 
and Ginkgo for all reconciler methods"

Step 5 (bundling-operator): "Create an OLM bundle v0.1.0 for the alpha 
channel with category Monitoring"
```

Result: A compilable operator project with production-ready types, markers, conditions, controller with check-create idempotency, owner references, event recording, finalizers, status updates, comprehensive test suite, and OLM bundle ready for certification.

### Testing a Skill

Each skill has a test guide in `tests/<skill-name>/test_guide.md` with:
- Exact prompts to give Claude
- Verification commands to run
- Acceptance criteria checklists
- Comparison against `operator-sdk` output

## What Skills Produce vs operator-sdk

| Aspect | operator-sdk | Skills |
|--------|-------------|--------|
| Types | Stub with `Foo` field | Real fields with validation markers, conditions, print columns |
| Controller | Empty `Reconcile()` | Three-phase pattern, check-create idempotency, owner refs, events |
| Webhooks | Empty `Default()`/`Validate()` | Real defaulting + validation logic |
| Status | Empty | Phase, conditions (Available/Progressing/Degraded), ObservedGeneration |
| RBAC | 3 markers (CRD only) | 8+ markers (all managed resources) |
| Tests | 1 test case (basic reconcile stub) | 14 test cases (lifecycle, per-method create/idempotent, helpers) |
| CSV | No specDescriptors/statusDescriptors | 9 specDescriptors + 4 statusDescriptors, rich alm-examples |
| Config | Generated incrementally | All files in one pass |

## Prerequisites

- Go 1.22+
- operator-sdk v1.37.0+ (for comparison testing)
- Claude Code

## References

- [Operator SDK Documentation](https://sdk.operatorframework.io/docs/overview/)
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Claude Agent Skills Specification](https://docs.anthropic.com/en/docs/claude-code/skills)

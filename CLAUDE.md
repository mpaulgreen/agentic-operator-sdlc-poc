# Agentic Operator POC

## Project Purpose

Replace the traditional OpenShift operator development workflow (operator-sdk CLI, hand-written controllers, manual OLM bundling) with a composable set of Claude Agentic Skills that eliminate boilerplate and guide creative work.

## Architecture

**5 Core Skills + 3 Subagents**, decomposed by concern (not phase).

Skills:
1. `scaffolding-operator` — Project init, Makefile, Dockerfile, Kustomize
2. `designing-operator-api` — CRD schema design, types.go, kubebuilder markers, webhooks, API versioning
3. `implementing-reconciliation` — Controller logic, idempotency, finalizers, RBAC
4. `testing-operator` — envtest + Ginkgo test suites
5. `bundling-operator` — OLM bundle, CSV, scorecard, certification

Subagents:
- `operator-reviewer` — Code review (uses skills 2+3)
- `operator-test-generator` — Test generation + execution (uses skill 4)
- `operator-bundle-validator` — Bundle validation (uses skill 5)

Full architecture: `architecture.md`

## Project Structure

```
agentic-operator-poc/
├── CLAUDE.md                          # This file
├── README.md                          # Project overview
├── .gitignore
├── architecture.md                    # Architecture: 5 skills + 3 subagents, rationale, composition
├── references/                        # Research and design documents
│   ├── openshift-operator-research.md # Initial research on OpenShift operator development
│   ├── development-plan.md           # Sprint plan: build order, test prompts, acceptance criteria
│   └── self_prompts.txt              # Prompts used during research phase
├── tests/                             # Test guides and gap analyses, organized by skill/subagent
│   ├── scaffolding-operator/
│   │   ├── test_guide.md
│   │   └── gap_analysis.md
│   ├── designing-operator-api/
│   │   ├── test_guide.md
│   │   └── gap_analysis.md
│   ├── implementing-reconciliation/
│   │   ├── test_guide.md
│   │   └── gap_analysis.md
│   ├── testing-operator/
│   │   ├── test_guide.md
│   │   └── gap_analysis.md
│   └── bundling-operator/
│       ├── test_guide.md
│       └── gap_analysis.md
└── .claude/
    ├── settings.local.json
    ├── agents/                        # Subagent definitions
    │   ├── operator-reviewer.md      # DONE — code review subagent
    │   ├── operator-test-generator.md # DONE — test generation subagent
    │   └── operator-bundle-validator.md # DONE — bundle validation subagent
    └── skills/                        # Skill implementations
        ├── scaffolding-operator/     # DONE — 29 files
        ├── designing-operator-api/   # DONE — 24 files
        ├── implementing-reconciliation/ # DONE — 19 files
        ├── testing-operator/         # DONE — 12 files
        └── bundling-operator/        # DONE — 15 files
```

## Development Plan

8 sprints, each building one component with unit + integration tests before proceeding.

| Sprint | Component | Dependencies |
|--------|-----------|-------------|
| 1 | scaffolding-operator | None |
| 2 | designing-operator-api | Sprint 1 |
| 3 | implementing-reconciliation | Sprint 2 |
| 4 | testing-operator | Sprint 3 |
| 5 | bundling-operator | Sprints 2+3 |
| 6 | operator-reviewer (subagent) | Skills 2+3 |
| 7 | operator-test-generator (subagent) | Skill 4 |
| 8 | operator-bundle-validator (subagent) | Skill 5 |

Full plan with sample prompts and acceptance criteria: `references/development-plan.md`

## Testing Methodology

Three layers, applied progressively:
- **Unit tests**: Each skill/subagent tested in isolation
- **Integration tests**: Cumulative skill chains (1+2, 1+2+3, etc.)
- **E2E scenario tests**: Full workflows across all components

Test artifacts organized by skill in `tests/<skill-name>/`:
- `test_guide.md` — test prompts, verification commands, acceptance criteria
- `gap_analysis.md` — comparison against operator-sdk output

## Key Conventions

- Skills live under `.claude/skills/<skill-name>/`
- Subagents live under `.claude/agents/<agent-name>.md`
- Tests live under `tests/<skill-or-agent-name>/`
- Each skill has: SKILL.md, references/, scripts/, assets/templates/
- Scripts validate (guardrails), the agent generates (contextual decisions)
- Templates and examples are extracted from real production operators in the knowledgebase, not synthetic code
- Progressive disclosure: frontmatter always loaded, SKILL.md body on trigger, references on demand
- Each skill is validated against `operator-sdk` output before marking complete

## Reference Operators (Source Material)

These knowledgebase operators provide real patterns for templates and examples:
- `../go-operator/operators/database-operator/` — Complete Go operator (controller, reconcilers, types, tests, CSV)
- `../model-registry-operator/` — Complex reconciliation (35+ resource types, Istio, Authorino)
- `../rhods-operator/` — Component-based operator design
- `../serverless-operator/` — OpenShift-native serverless operator

## Current Status

All 8 sprints complete. 5 skills (99 files) + 3 subagents built and validated.

### Completed
- **Sprint 1**: `scaffolding-operator` — 29 files
  - Tests 1.1-1.4 PASS: All 4 patterns (new project, same-group, cluster-scoped, different-group)
  - Test 1.5 PASS: Matches operator-sdk output

- **Sprint 2**: `designing-operator-api` — 24 files (SKILL.md, 7 references, 1 script, 11 templates, 4 examples)
  - Test 2.1 PASS: Simple CRD design (14/14 validation, markers, conditions, print columns)
  - Test 2.2 PASS: Complex CRD (StorageSpec, BackupSpec, ResourceRequirements, pointer types)
  - Test 2.3 PASS: SDK comparison (9 markers vs 0, 4 columns vs 0, conditions vs none)
  - Test 2.4 PASS: Webhooks (handler + 9 config files + main.go update, compiles)
  - Test 2.5 PASS: SDK webhook comparison (structure matches, skill has real logic)
  - Test 2.6 PASS: API versioning (v1beta1 + storageversion + maxMemory field)
  - Test I-1.2 PASS: Integration scaffold + design (message-queue-operator end-to-end)

- **Sprint 3**: `implementing-reconciliation` — 19 files (SKILL.md, 7 references, 2 scripts, 6 templates, 3 examples)
  - Test 3.1 PASS: Simple reconciler (5 files, RBAC 8→9 markers, idempotency, 3 reconciler methods, compiles)
  - Test 3.2 PASS: Finalizer lifecycle (add/check/cleanup/remove, handleDeletion)
  - Test 3.3 PASS: Add ConfigMap resource (Workflow B — new method, RBAC, Owns, correct dependency order)
  - Test 3.4 PASS: SDK comparison (5 files vs 1 stub, 9 RBAC vs 3, 7 events vs 0, 4 Owns vs 0)
  - Test I-1.2.3 PASS: Integration scaffold+design+reconcile (notification-operator, all 4 scripts pass, compiles)
  - Scripts validated against real database-operator (10 RBAC markers, all idempotency checks pass)

- **Sprint 4**: `testing-operator` — 12 files (SKILL.md, 4 references, 2 scripts, 4 templates, 1 example)
  - Test 4.1 PASS: Full test suite (suite_test.go + controller_test.go, 14 test cases, 4/4 methods 100% coverage, go vet passes)
  - Test 4.2 PASS: ConfigMap tests present with redis.conf content verification and idempotency
  - Test 4.3 PASS: SDK comparison (14 test cases vs 1, skill has per-method + idempotency + helpers)

- **Sprint 5**: `bundling-operator` — 15 files (SKILL.md, 6 references, 3 scripts, 4 templates, 1 example)
  - Test 5.1 PASS: Initial bundle (structure 13/13, CSV 22/22, scorecard 17/17, 0 errors 0 warnings)
  - Test 5.2 PASS: Version update v0.1.0→v0.2.0 (replaces, cronjobs RBAC, 3 backup descriptors, all scripts pass)
  - Test 5.3 PASS: SDK comparison (9 specDescriptors vs 0, 4 statusDescriptors vs 0, 11 RBAC rules vs 5)

- **Sprint 6**: `operator-reviewer` — 1 agent definition (.claude/agents/operator-reviewer.md)
  - Test 6.1 PASS: Reviewed flawed operator — all 5 planted issues detected (3 by scripts, 2 by manual inspection), 0 false positives
  - Test 6.2 PASS: Reviewed clean database-operator — 0 false Critical findings, all 3 scripts PASS

- **Sprint 7**: `operator-test-generator` — 1 agent definition (.claude/agents/operator-test-generator.md)
  - Test 7.1 PASS: Full suite generated (4/4 methods discovered, 14 test cases, go vet passes, 100% coverage)
  - Test 7.2 PASS: Incremental PDB tests added (5/5 methods, 16 test cases, existing tests preserved)

- **Sprint 8**: `operator-bundle-validator` — 1 agent definition (.claude/agents/operator-bundle-validator.md)
  - Test 8.1 PASS: Correct bundle validated (3/3 scripts pass, 7/8 certification checklist — only gap: empty icon)
  - Test 8.2 PASS: Flawed bundle — all 4 planted issues detected (missing annotations, missing installModes, invalid alm-examples, non-existent descriptor path)

### Status: ALL SPRINTS COMPLETE
- 5 skills: scaffolding-operator (29), designing-operator-api (24), implementing-reconciliation (19), testing-operator (12), bundling-operator (15) = **99 skill files**
- 3 subagents: operator-reviewer, operator-test-generator, operator-bundle-validator
- Next: E2E scenario tests (Scenarios A, B, C, D from development-plan.md)

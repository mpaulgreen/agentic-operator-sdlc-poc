# Agentic Operator POC

## Project Purpose

Replace the traditional OpenShift operator development workflow (operator-sdk CLI, hand-written controllers, manual OLM bundling) with a composable set of Claude Agentic Skills that eliminate boilerplate and guide creative work.

## Architecture

**5 Core Skills + 3 Subagents**, decomposed by concern (not phase).

Skills:
1. `scaffolding-operator` — Project init, Makefile, Dockerfile, Kustomize
2. `designing-operator-api` — CRD schema design, types.go, kubebuilder markers
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
│   └── scaffolding-operator/         # (others created per sprint)
│       ├── test_guide.md             # Test prompts, verification commands, acceptance criteria
│       └── gap_analysis.md           # Comparison with operator-sdk output
└── .claude/
    ├── settings.local.json
    └── skills/                        # Skills built here (agents/ created in Sprint 6)
        └── scaffolding-operator/     # DONE — 29 files (others created per sprint)
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

Sprint 1 complete. `scaffolding-operator` skill built and validated against operator-sdk.

### Completed
- **Sprint 1**: `scaffolding-operator` — 29 files (SKILL.md, 3 references, 1 script, 25 templates)
  - Test 1.1 PASS: New project scaffold (48/48 structural checks, compiles)
  - Test 1.2 PASS: Same-group kind (cache/RedisSentinel) — flat layout, compiles
  - Test 1.3 PASS: Cluster-scoped kind (cache/ClusterRedisConfig) — scope=Cluster marker, compiles
  - Test 1.4 PASS: Different-group kind (monitoring/AlertPolicy) — multi-group layout, aliased imports, compiles
  - Test 1.5 PASS: Matches operator-sdk output across all 4 patterns

### Next
- Sprint 2: `designing-operator-api`

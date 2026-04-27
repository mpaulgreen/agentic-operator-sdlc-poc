# Agentic Operator POC

Build OpenShift operators using Claude Agentic Skills instead of the traditional `operator-sdk` CLI workflow.

## What This Is

A composable set of **5 skills + 3 subagents** that replace the manual, repetitive steps of operator development — scaffolding, CRD design, controller reconciliation, testing, and OLM bundling — with guided, template-driven AI workflows.

## Architecture

```
Developer
    │
    ├── scaffolding-operator      → Project init (replaces operator-sdk init/create api)
    ├── designing-operator-api    → CRD schema design from natural language
    ├── implementing-reconciliation → Controller logic with idempotency patterns
    ├── testing-operator          → envtest + Ginkgo test generation
    └── bundling-operator         → OLM bundle, CSV, certification readiness
```

Three subagents (`operator-reviewer`, `operator-test-generator`, `operator-bundle-validator`) combine skills for specialized workflows like code review and bundle validation.

See [architecture.md](architecture.md) for the full design rationale, directory structures, and composition patterns.

## Project Structure

```
agentic-operator-poc/
├── architecture.md              # Full architecture and design decisions
├── references/                  # Research and planning documents
│   ├── openshift-operator-research.md
│   └── development-plan.md
├── tests/                       # Test guides and gap analyses per skill
│   └── scaffolding-operator/
├── .claude/
│   ├── skills/                  # Skill implementations
│   │   └── scaffolding-operator/  (30 files — built & validated)
│   └── agents/                  # Subagent definitions
└── CLAUDE.md                    # Project context for Claude Code sessions
```

## Current Status

| Sprint | Skill | Status |
|--------|-------|--------|
| 1 | `scaffolding-operator` | Done — validated against operator-sdk v1.37.0 |
| 2 | `designing-operator-api` | Next |
| 3-8 | Remaining skills + subagents | Planned |

## How to Use

### With Claude Code

The skills are in `.claude/skills/`. When working in a project that has these skills, ask Claude to scaffold, design, implement, test, or bundle an operator:

```
"Create a new operator that manages PostgreSQL clusters on OpenShift"
"Add a BackupSchedule CRD to my existing operator"
"Review my operator controller for best practices"
```

### Testing a Skill

Each skill has a test guide in `tests/<skill-name>/test_guide.md` with:
- Exact prompts to give Claude
- Verification commands to run
- Acceptance criteria checklists
- Comparison against `operator-sdk` output

## Prerequisites

- Go 1.22+
- operator-sdk v1.37.0+ (for comparison testing)
- Claude Code

## References

- [Operator SDK Documentation](https://sdk.operatorframework.io/docs/overview/)
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Claude Agent Skills Specification](https://docs.anthropic.com/en/docs/claude-code/skills)

# Recommended Architecture: OpenShift Operators via Claude Agentic Skills

## Context

Building OpenShift operators the traditional way (operator-sdk CLI, hand-writing controllers, debugging OLM bundles) is cumbersome. The development lifecycle has 8 phases — of which ~40% is pure boilerplate and the remaining 60% is creative work buried under repetitive patterns. Claude Agentic Skills can eliminate the boilerplate and guide the creative work through composable, focused skills.

This document proposes a **5-skill + 3-subagent** architecture that maps to operator development concerns (not phases), enabling both end-to-end operator creation and targeted use on existing operators.

---

## Design Rationale: Why Concern-Based, Not Phase-Based

Three decomposition strategies were considered:

| Strategy | Pros | Cons |
|----------|------|------|
| **Phase-based** (init, api, reconcile, test, bundle) | Mirrors operator-sdk workflow | Creates artificial sequential dependencies; prevents composability |
| **Pain-point-based** (one skill per pain point) | Directly addresses developer pain | Too many skills (7+); reconciliation idempotency, error handling, and orchestration all live in the same files |
| **Concern-based** (groups activities sharing the same knowledge and templates) | Composable, independently usable, right granularity | Requires clear cross-references between skills |

**Chosen: Concern-based.** Each skill groups activities that share the same deep knowledge, code templates, and validation logic. A developer working on reconciliation needs idempotency patterns, resource builders, owner references, and error handling — these belong together regardless of which development "phase" they're in.

---

## Architecture Overview

```
                    ┌─────────────────────────────────────────┐
                    │           Developer / Main Agent         │
                    └──────────────┬──────────────────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                     │
    ┌─────────▼──────────┐  ┌─────▼──────────┐  ┌──────▼─────────────┐
    │  operator-reviewer  │  │ operator-test-  │  │ operator-bundle-   │
    │    (subagent)       │  │  generator      │  │  validator         │
    │                     │  │  (subagent)     │  │  (subagent)        │
    └─────────────────────┘  └────────────────┘  └────────────────────┘
              │                    │                     │
              │ uses               │ uses                │ uses
              ▼                    ▼                     ▼
    ┌───────────────────────────────────────────────────────────────┐
    │                      5 CORE SKILLS                            │
    │                                                               │
    │  ┌──────────────┐   ┌──────────────┐   ┌──────────────────┐  │
    │  │ scaffolding-  │──▶│ designing-   │──▶│ implementing-    │  │
    │  │ operator      │   │ operator-api │   │ reconciliation   │  │
    │  └──────────────┘   └──────────────┘   └────────┬─────────┘  │
    │                                           ┌─────┴──────┐     │
    │                                           │            │     │
    │                                    ┌──────▼─────┐ ┌────▼───┐ │
    │                                    │ testing-   │ │bundling│ │
    │                                    │ operator   │ │operator│ │
    │                                    └────────────┘ └────────┘ │
    └───────────────────────────────────────────────────────────────┘
```

---

## The 5 Core Skills

### Skill 1: `scaffolding-operator` (BUILT & VALIDATED)
**Purpose**: Project initialization and structure generation  
**Maps to**: Initialization (100% boilerplate) + Build/Deploy (95% boilerplate)  
**Trigger**: "create operator", "scaffold operator", "init operator", "new operator project", "add API group/resource"  
**Status**: Sprint 1 complete. Validated against `operator-sdk` v1.37.0 — output matches across all four patterns.

**Two workflows, four patterns tested:**
- **Workflow A** (Pattern A): New project from scratch — generates ~40 files (PROJECT, cmd/main.go, Makefile, Dockerfile, API stubs, controller stubs, full config/ Kustomize tree). Supports both namespace-scoped (default) and cluster-scoped resources.
- **Workflow B** covers three patterns:
  - **Pattern B** (same-group, namespaced): Flat layout — types and controllers in shared packages, no aliases needed.
  - **Pattern D** (same-group, cluster-scoped): Same flat layout but with `//+kubebuilder:resource:scope=Cluster` marker and `namespaced: true` omitted from PROJECT.
  - **Pattern C** (different-group): Multi-group layout — separate packages (`api/<group>/<version>/`, `internal/controller/<group>/`), aliased imports, `multigroup: true` in PROJECT. Correct ordering: all same-group kinds before enabling multigroup.

```
scaffolding-operator/
├── SKILL.md                              # Workflow A (new project) + Workflow B (add resource)
├── references/
│   ├── project-layout.md                # Standard operator directory structure
│   ├── makefile-targets.md              # All make targets and their purposes
│   └── kustomize-structure.md           # config/ directory with all overlays
├── scripts/
│   └── validate-project-structure.sh    # 48 structural checks (files, dirs, targets, fields)
└── assets/templates/                     # 25 templates (9 top-level + 16 config)
    ├── main.go.tmpl                     # cmd/main.go with manager, health probes, HTTP/2 security
    ├── dockerfile.tmpl                  # Multi-stage build, distroless nonroot
    ├── makefile.tmpl                    # Complete Makefile (~200 lines, all targets)
    ├── project.tmpl                     # PROJECT metadata
    ├── groupversion_info.go.tmpl        # API group registration with license header
    ├── readme.md.tmpl                   # README with deploy/undeploy instructions
    ├── gitignore.tmpl                   # .gitignore
    ├── dockerignore.tmpl                # .dockerignore
    ├── golangci-lint.yml.tmpl           # .golangci.yml linter config
    └── config/                          # 16 config templates (flat naming)
        ├── default-manager-auth-proxy-patch.yaml.tmpl
        ├── default-manager-config-patch.yaml.tmpl
        ├── manifests-kustomization.yaml.tmpl
        ├── crd-kustomizeconfig.yaml.tmpl
        ├── prometheus-kustomization.yaml.tmpl
        ├── prometheus-monitor.yaml.tmpl
        ├── rbac-auth-proxy-*.yaml.tmpl  (4 files)
        ├── rbac-editor-role.yaml.tmpl
        ├── rbac-viewer-role.yaml.tmpl
        ├── scorecard-config.yaml.tmpl
        ├── scorecard-kustomization.yaml.tmpl
        ├── scorecard-patches-basic.config.yaml.tmpl
        └── scorecard-patches-olm.config.yaml.tmpl
```

**29 files total** (1 SKILL.md, 3 references, 1 script, 25 templates — originally planned 5 templates, expanded to 25 after gap analysis against operator-sdk). All templates use flat naming under `assets/templates/` and `assets/templates/config/` with no subdirectories.

**What it eliminates**: Manual `operator-sdk init` and `operator-sdk create api`. The agent generates all scaffolding files in one pass, including config files that the SDK generates incrementally across multiple commands.

**Validated against operator-sdk**: Directory layout, import patterns, controller registrations, and config files all match across all four patterns. The skill additionally provides event recorder setup, secure metrics by default, `doc.go`, and CRD base YAMLs at scaffold time (SDK requires `make manifests` to generate these). See `tests/scaffolding-operator/gap_analysis.md` for the detailed comparison.

---

### Skill 2: `designing-operator-api`
**Purpose**: CRD schema design from natural language descriptions  
**Maps to**: API Definition (70% boilerplate / 30% creative) + CRD Generation (100% boilerplate)  
**Trigger**: "define api", "design crd", "define types", "add fields", "add status conditions"

```
designing-operator-api/
├── SKILL.md                          # Decision tree: "What does your operator manage?"
├── references/
│   ├── type-design-patterns.md      # Spec/Status patterns, optional fields, embedded types
│   ├── validation-markers.md        # All kubebuilder markers with examples
│   ├── cel-validation-rules.md      # CEL expressions (2025+ best practice)
│   ├── status-conventions.md        # Condition types, phase enums, subresource patterns
│   └── api-versioning.md            # v1alpha1 → v1 progression, conversion webhooks
├── scripts/
│   └── validate-api-types.py        # Checks for missing json tags, missing markers
└── assets/
    ├── templates/
    │   └── types.go.tmpl            # Parameterized Spec/Status with all markers
    └── examples/
        ├── simple-spec.go           # Level 1: minimal spec (3 fields)
        ├── complex-spec.go          # Level 3+: nested types, storage specs
        └── status-conditions.go     # Standard condition patterns
```

**What it eliminates**: Trial-and-error with kubebuilder marker syntax, missing json tags, incorrect validation rules. The agent walks the developer through API design as a conversation: "What fields does your resource need? What should be validated? What status does the user need to see?"

---

### Skill 3: `implementing-reconciliation`
**Purpose**: Controller logic with idempotency, error handling, RBAC, and finalizers  
**Maps to**: Reconciliation (30% boilerplate / 70% creative) — **the highest pain point**  
**Trigger**: "implement reconciler", "write controller", "add resource", "add finalizer", "fix error handling"

```
implementing-reconciliation/
├── SKILL.md                          # Recipe book: "To reconcile X, use pattern Y"
├── references/
│   ├── reconciliation-architecture.md  # Three-phase pattern, level-based triggers, requeue
│   ├── idempotency-patterns.md        # Check-create, check-update, Server-Side Apply
│   ├── resource-orchestration.md      # Dependency ordering, resource builders, owner refs
│   ├── error-handling-patterns.md     # Retry vs. fail, degraded conditions
│   ├── finalizer-lifecycle.md         # Add/remove, cleanup ordering, race prevention
│   ├── rbac-annotations.md            # All markers, least-privilege, namespace vs. cluster
│   └── event-recording.md             # Event types, reason strings, message conventions
├── scripts/
│   ├── validate-rbac-annotations.py   # Checks for over/under-granting
│   └── check-idempotency.py           # Flags non-idempotent patterns in reconcile code
└── assets/
    ├── templates/
    │   ├── controller.go.tmpl          # Full Reconcile() + SetupWithManager() skeleton
    │   ├── reconciler-method.go.tmpl   # Parameterized reconcileResource() with check-create
    │   ├── resource-builder.go.tmpl    # deploymentForX() builder function pattern
    │   ├── status-updater.go.tmpl      # updateStatus() with condition management
    │   ├── conditions.go.tmpl          # Condition types and setters
    │   └── helpers.go.tmpl             # Labels, naming, utility functions
    └── examples/
        ├── simple-reconciler.go        # Level 1: Secret + Deployment + Service
        ├── complex-reconciler.go       # Level 3: 10+ resources with dependency ordering
        └── ssa-reconciler.go           # Server-Side Apply pattern (modern approach)
```

**What it eliminates**: The check-create boilerplate repeated 20+ times per controller. The agent stamps out the mechanical pattern while the developer focuses on *what the desired state looks like* for each resource. This is the single biggest productivity gain — the reconciler-method template alone saves writing the same 15-line pattern dozens of times.

**Why 7 reference documents**: Reconciliation is the highest-pain area (10/10). Each reference covers a distinct concern loaded independently via progressive disclosure. Only 1-2 are loaded per interaction — never all 7 at once.

---

### Skill 4: `testing-operator`
**Purpose**: Generate unit, integration, and E2E test suites  
**Maps to**: Testing (60% boilerplate / 40% creative)  
**Trigger**: "write tests", "test operator", "add coverage", "create e2e tests"

```
testing-operator/
├── SKILL.md                          # "Given a controller, generate matching tests"
├── references/
│   ├── envtest-setup.md             # suite_test.go, CRD paths, binary assets
│   ├── ginkgo-patterns.md           # Describe/Context/It, Eventually, BeforeEach
│   ├── test-scenarios.md            # What to test: create, update, delete, idempotency, errors
│   └── e2e-patterns.md             # Chainsaw/kuttl, real-cluster testing
├── scripts/
│   ├── check-test-coverage.sh       # Runs go test -coverprofile, reports uncovered methods
│   └── generate-test-matrix.py      # Given controller, outputs required test scenarios
└── assets/
    ├── templates/
    │   ├── suite_test.go.tmpl       # Complete envtest suite setup
    │   ├── controller_test.go.tmpl  # Controller test with BeforeEach/AfterEach
    │   ├── reconciler_test.go.tmpl  # Per-resource: create, idempotent, error paths
    │   └── e2e_test.go.tmpl        # E2E skeleton
    └── examples/
        └── database-controller-test.go  # Real test from database-operator
```

**What it eliminates**: Envtest boilerplate, Ginkgo/Gomega ceremony, the mental effort of mapping "for every reconcileX method, test (1) creates when absent, (2) is idempotent when present, (3) handles errors, (4) updates on spec change."

---

### Skill 5: `bundling-operator`
**Purpose**: OLM bundle generation, CSV authoring, certification readiness  
**Maps to**: OLM Bundling (80% boilerplate / 20% creative)  
**Trigger**: "create bundle", "generate csv", "olm bundle", "scorecard", "certify operator"

```
bundling-operator/
├── SKILL.md                          # Two workflows: initial bundle & version update
├── references/
│   ├── csv-anatomy.md               # Every CSV section explained
│   ├── bundle-structure.md          # manifests/, metadata/, annotations.yaml
│   ├── olm-v0-vs-v1.md             # Current patterns + OLM v1 migration guidance
│   ├── scorecard-tests.md           # Built-in tests, common failures, fixes
│   ├── certification-checklist.md   # Red Hat certification prerequisites
│   └── catalog-management.md        # Index images, CatalogSource, channels
├── scripts/
│   ├── validate-csv.py              # Checks CSV for missing descriptors, bad versions
│   ├── validate-bundle-structure.sh # Verifies bundle directory layout
│   └── check-scorecard-readiness.py # Pre-flight before running scorecard
└── assets/
    ├── templates/
    │   ├── csv.yaml.tmpl            # Parameterized CSV with all required sections
    │   ├── annotations.yaml.tmpl    # Bundle metadata
    │   ├── bundle.dockerfile.tmpl   # Bundle image Dockerfile
    │   └── scorecard-config.yaml.tmpl
    └── examples/
        └── database-operator-csv.yaml  # Real 300+ line CSV as reference
```

**What it eliminates**: Hand-editing a 1000+ line CSV. The agent generates the CSV from the CRD types (descriptors map directly to spec/status fields), validates it with scripts before scorecard ever runs, and pre-checks certification requirements.

---

## The 3 Subagents

Subagents are specialized workers defined in `.claude/agents/` that combine skills with focused expertise. They can be dispatched by the main agent or invoked directly.

### `operator-reviewer`
```yaml
---
name: operator-reviewer
description: Reviews operator code for quality and common mistakes
tools: Bash, Read
skills: implementing-reconciliation, designing-operator-api
---
```
**Checks for**: Non-idempotent reconciliation, missing owner references, RBAC over-granting, missing status updates, finalizer race conditions, naming violations.  
**Output**: Structured review with Critical/Warning/OK categories, line numbers, fix suggestions.

### `operator-test-generator`
```yaml
---
name: operator-test-generator
description: Generates and runs tests for operator controllers
tools: Bash, Read, Edit, Write
skills: testing-operator
---
```
**Workflow**: Read controller → identify reconcileX methods → generate/update test file → run `make test` → report results → suggest fixes for failures.

### `operator-bundle-validator`
```yaml
---
name: operator-bundle-validator
description: Validates OLM bundles and checks certification readiness
tools: Bash, Read
skills: bundling-operator
---
```
**Workflow**: Validate bundle structure → parse CSV → check descriptors completeness → run scorecard pre-flight → report readiness with actionable fixes.

---

## How the Skills Compose

### Scenario A: New Operator from Scratch

> "Build me an operator that manages Redis clusters on OpenShift"

```
1. scaffolding-operator      → generates project structure, Makefile, Dockerfile
2. designing-operator-api    → interactive API design → types.go with markers
3. implementing-reconciliation → controller with reconcilers for Secret, ConfigMap,
                                StatefulSet, Service (agent stamps out pattern,
                                developer specifies desired state per resource)
4. operator-test-generator ─┐
   operator-reviewer       ─┤  (dispatched in parallel)
5. bundling-operator         → generate CSV and bundle
6. operator-bundle-validator → validate before scorecard
```

### Scenario B: Add Feature to Existing Operator (Same Group)

> "Add sentinel support to my redis operator"

```
1. scaffolding-operator (Workflow B, same-group)
                                → adds RedisSentinel types to api/v1alpha1/,
                                  controller to internal/controller/ (flat layout),
                                  editor/viewer roles, sample CR, kustomization updates
2. designing-operator-api       → flesh out RedisSentinelSpec fields
3. implementing-reconciliation  → add reconcileDeployment() to controller
4. operator-test-generator      → generate tests for new method
5. bundling-operator            → update CSV version and descriptors
```

### Scenario B2: Add Feature in New Group

> "Add monitoring/alerting to my redis operator"

```
1. scaffolding-operator (Workflow B, different-group)
                                → enables multigroup, creates api/monitoring/v1alpha1/
                                  and internal/controller/monitoring/ (multi-group layout),
                                  aliased imports, editor/viewer roles
2. designing-operator-api       → flesh out AlertPolicySpec fields
3. implementing-reconciliation  → implement reconciliation in monitoring controller
4. operator-test-generator      → generate tests
5. bundling-operator            → update CSV with new CRD
```

### Scenario C: Review Existing Operator

> "Review my operator for best practices"

```
1. operator-reviewer   → reads all controller files, applies checks
2. Main agent          → uses appropriate skills to fix findings
```

### Scenario D: Prepare for Certification

> "Get my operator ready for Red Hat certification"

```
1. operator-bundle-validator  → validate bundle, check certification prerequisites
2. bundling-operator          → fix issues, regenerate CSV
3. operator-reviewer          → review security (non-root, SCC, RBAC least-privilege)
```

---

## Progressive Disclosure in Practice

| Layer | When Loaded | Size | Example |
|-------|------------|------|---------|
| **Metadata** | Always (all 5 skills) | ~250 tokens total | Frontmatter: name + description |
| **Instructions** | When skill triggered | ~300 tokens per skill | SKILL.md body: workflow steps |
| **References** | On demand (1-2 per interaction) | ~500-2000 tokens each | `idempotency-patterns.md` loaded only when writing reconcilers |
| **Templates** | When generating specific file | ~200-500 tokens each | `controller.go.tmpl` loaded only when writing controller |
| **Scripts** | Execute externally, return results | ~50 tokens (output only) | `validate-csv.py` returns pass/fail + issues |

The full knowledge base across all 5 skills is ~30,000+ tokens. Progressive disclosure ensures only ~1,000-3,000 tokens are loaded per interaction.

---

## Maturity Model Alignment

The same 5 skills scale across all operator capability levels:

| Level | What Changes | Skills Affected |
|-------|-------------|-----------------|
| **L1: Basic Install** | Simple spec, 2-3 resource types | All skills use "simple" templates/examples |
| **L2: Seamless Upgrades** | Version-aware reconciliation, migration | `designing-operator-api` (versioning refs), `implementing-reconciliation` (upgrade patterns) |
| **L3: Full Lifecycle** | Backup/restore, storage lifecycle | `implementing-reconciliation` (CronJob reconcilers, PDB), `designing-operator-api` (backup types) |
| **L4: Deep Insights** | Metrics, alerts, monitoring | `implementing-reconciliation` (PrometheusRule/ServiceMonitor reconcilers) |
| **L5: Auto Pilot** | Autoscaling, self-tuning | `implementing-reconciliation` (HPA reconcilers, metric-based decisions) |

No new skills needed for higher levels — just deeper reference documents and more advanced templates within existing skills.

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **5 skills, not 7 or 3** | Fewer than 5 overloads `implementing-reconciliation`. More than 5 fragments concerns that share files (error handling + RBAC + reconciliation all live in controller.go) |
| **3 subagents, not 5** | Each maps to a distinct work mode: review (read-only), test generation (write+execute), bundle validation (read+validate). Scaffolding doesn't need a subagent — it's a one-time activity |
| **Scripts validate, not generate** | Code generation is the agent's job (it can make contextual decisions). Scripts serve as guardrails — catching mistakes the agent might make |
| **Examples from real operators** | Templates and examples should be extracted from the actual database-operator and model-registry-operator in the knowledgebase — real code with real patterns, not idealized abstractions |
| **References over inline instructions** | Deep knowledge (7 refs in implementing-reconciliation) stays out of context until needed. Loading all 7 would be ~10K tokens; typically only 1-2 are needed per interaction |

---

## Estimated Productivity Gains

| Activity | Traditional | With Skills | Improvement |
|----------|------------|-------------|-------------|
| Project scaffolding | 15-30 min | 2-3 min | ~10x |
| API/CRD design | 1-2 hours (marker trial-and-error) | 15-20 min (guided conversation) | ~5x |
| Controller reconciliation | 1-3 days (per 5 resource types) | 2-4 hours (pattern stamping + domain decisions) | ~5-8x |
| Test generation | 4-8 hours | 30-60 min | ~8x |
| OLM bundle/CSV | 2-4 hours (debugging scorecard) | 20-30 min | ~6x |
| Code review | 1-2 hours (manual checklist) | 10-15 min (automated) | ~8x |

---

## Complete Directory Structure

```
.claude/
├── skills/
│   ├── scaffolding-operator/           ← BUILT & VALIDATED (Sprint 1)
│   │   ├── SKILL.md
│   │   ├── references/   (3 docs)
│   │   ├── scripts/      (1 validator, 48 checks)
│   │   └── assets/templates/  (25 templates — 9 top-level + 16 config)
│   │
│   ├── designing-operator-api/
│   │   ├── SKILL.md
│   │   ├── references/   (5 docs)
│   │   ├── scripts/      (1 validator)
│   │   └── assets/       (1 template + 3 examples)
│   │
│   ├── implementing-reconciliation/
│   │   ├── SKILL.md
│   │   ├── references/   (7 docs)
│   │   ├── scripts/      (2 validators)
│   │   └── assets/       (6 templates + 3 examples)
│   │
│   ├── testing-operator/
│   │   ├── SKILL.md
│   │   ├── references/   (4 docs)
│   │   ├── scripts/      (2 tools)
│   │   └── assets/       (4 templates + 1 example)
│   │
│   └── bundling-operator/
│       ├── SKILL.md
│       ├── references/   (6 docs)
│       ├── scripts/      (3 validators)
│       └── assets/       (4 templates + 1 example)
│
└── agents/
    ├── operator-reviewer.md
    ├── operator-test-generator.md
    └── operator-bundle-validator.md
```

**Planned totals**: 5 skills, 25 reference docs, 9 scripts, 20+ templates, 8 examples, 3 subagents.
**Actual (Sprint 1)**: 1 skill built with 29 files (originally planned 11, expanded to 29 after operator-sdk gap analysis).

---

## Implementation Status

| Component | Sprint | Status | Files | Notes |
|-----------|--------|--------|-------|-------|
| `scaffolding-operator` | 1 | **DONE** | 29 | Validated against operator-sdk v1.37.0. 4 patterns tested: A (new project), B (same-group), D (cluster-scoped), C (different-group). 48-check validation script. |
| `designing-operator-api` | 2 | Pending | — | |
| `implementing-reconciliation` | 3 | Pending | — | |
| `testing-operator` | 4 | Pending | — | |
| `bundling-operator` | 5 | Pending | — | |
| `operator-reviewer` | 6 | Pending | — | |
| `operator-test-generator` | 7 | Pending | — | |
| `operator-bundle-validator` | 8 | Pending | — | |

### Sprint 1 Lessons Learned

1. **Gap analysis against operator-sdk is essential.** Initial skill had 11 files. After comparing against `operator-sdk` output, expanded to 29 files to cover config/manifests, config/scorecard, config/prometheus, auth proxy RBAC, editor/viewer roles, license headers, README, .dockerignore, and .golangci.yml.
2. **DeepCopy methods must be generated at scaffold time.** Types register with `SchemeBuilder.Register()` which requires `runtime.Object` — without DeepCopy stubs the project won't compile.
3. **`hack/boilerplate.go.txt` is critical.** The Makefile's `generate` target references it via `controller-gen object:headerFile`. Missing = `make generate/build/test` all fail.
4. **Multi-group layout requires correct API creation order.** Same-group kinds must be created before enabling multigroup. Reversing this triggers an operator-sdk bug (duplicate import aliases). Our skill handles this correctly by detecting same-group vs. different-group.
5. **Workflow B has three patterns.** Same-group namespaced (flat layout), same-group cluster-scoped (flat + `scope=Cluster` marker), and different-group (multi-group layout, aliased imports, separate packages).
6. **Cluster-scoped resources are a flag, not a separate workflow.** Only two differences from namespaced: `//+kubebuilder:resource:scope=Cluster` marker on root type, and `namespaced: true` omitted from PROJECT. Everything else (controller, RBAC, config) is identical.
7. **Pluralization follows Kubernetes conventions, not simple `+s`.** Kinds ending in `s` keep the same plural (`redis` → `redis`), `y` after consonant becomes `ies` (`policy` → `policies`). SKILL.md documents this with user override via `--plural`.
8. **Test ordering matters for SDK comparison.** All same-group APIs (namespaced + cluster-scoped) must be created before `operator-sdk edit --multigroup`. Reversing this triggers an SDK bug (duplicate import aliases).

---

## Source Material for Templates and Examples

These files in the knowledgebase should be used as the source of truth for extracting real patterns:

- `go-operator/operators/database-operator/internal/controller/databasecluster_controller.go` — controller skeleton, Reconcile(), SetupWithManager()
- `go-operator/operators/database-operator/internal/controller/databasecluster_reconcilers.go` — check-create idempotency pattern (4 resource types)
- `go-operator/operators/database-operator/api/v1alpha1/databasecluster_types.go` — types with kubebuilder markers
- `go-operator/operators/database-operator/internal/controller/databasecluster_controller_test.go` — envtest + Ginkgo test patterns
- `go-operator/operators/database-operator/bundle/manifests/database-operator.clusterserviceversion.yaml` — complete CSV
- `model-registry-operator/internal/controller/modelregistry_controller.go` — complex reconciliation with 35+ resource types

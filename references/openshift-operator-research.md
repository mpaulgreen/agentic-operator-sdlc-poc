# Building OpenShift Operators: Comprehensive Research

**Research Date:** April 2026
**Scope:** Concepts, tooling, architecture, lifecycle management, testing, best practices

---

## 1. What is an OpenShift Operator

### The Operator Pattern

The Operator pattern is a Kubernetes-native way to extend cluster behavior without modifying Kubernetes source code. An Operator is a method of packaging, deploying, and managing a Kubernetes application that encodes human operational knowledge -- how to deploy, configure, upgrade, backup, and maintain an application -- into software that runs as a controller inside the cluster.

**Core formula:** `Operator = Custom Resource Definition (CRD) + Custom Controller`

The Operator runs as a Pod on the cluster, interacting with the Kubernetes API server. It introduces new object types through CRDs (an extension mechanism in Kubernetes) and watches for changes to those custom resources. When a user creates, updates, or deletes a custom resource, the Operator's controller detects the change and reconciles the cluster state to match the desired state described in the resource.

### How Operators Extend Kubernetes

- **Custom Resources (CRs):** Define application-specific configuration as first-class Kubernetes API objects
- **Custom Controllers:** Implement the reconciliation loop that continuously drives actual state toward desired state
- **Domain-Specific Logic:** Encode day-1 (installation, configuration) and day-2 (upgrades, backups, scaling, failure recovery) operational knowledge

### OpenShift-Specific Context

Red Hat OpenShift Operators automate the creation, configuration, and management of instances of Kubernetes-native applications. They provide automation at every level of the stack, from managing platform components to delivering applications as managed services. OpenShift itself is managed by a collection of operators (the Cluster Version Operator, Machine Config Operator, etc.), making the operator pattern foundational to the platform.

**Sources:**
- [Kubernetes -- Operator Pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)
- [Red Hat -- What are OpenShift Operators?](https://www.redhat.com/en/technologies/cloud-computing/openshift/what-are-openshift-operators)
- [Operator Pattern: Kubernetes/OpenShift (Medium)](https://medium.com/operators/operator-pattern-kubernetes-openshift-380ddc6a147c)
- [Trilio -- Understanding the OpenShift Operator Pattern](https://trilio.io/openshift-tutorial/openshift-operator/)

---

## 2. Operator SDK

### Overview

The Operator SDK is a component of the Operator Framework, an open-source CNCF toolkit for managing Kubernetes-native applications. It provides workflows, scaffolding, and libraries to develop operators efficiently. Kubebuilder is embedded into the Operator SDK as the scaffolding solution for Go-based operators, meaning existing Kubebuilder projects work with the SDK as-is.

### Supported Project Types

| Type | Language/Tool | Best For | Post-Scaffolding Development |
|------|--------------|----------|------------------------------|
| **Go** | Go + controller-runtime | Complex operators needing fine-grained control, custom business logic | Ongoing use of SDK for code generation, API scaffolding |
| **Ansible** | Ansible playbooks/roles | Teams with Ansible expertise, configuration-heavy operators | Development moves entirely outside the SDK CLI |
| **Helm** | Helm charts | Wrapping existing Helm charts as operators | Development moves entirely outside the SDK CLI |

### Key SDK CLI Commands

- `operator-sdk init` -- Initialize a new operator project
- `operator-sdk create api` -- Scaffold a new API (CRD + controller)
- `operator-sdk create webhook` -- Scaffold admission/conversion webhooks
- `operator-sdk bundle create` -- Create an OLM bundle
- `operator-sdk scorecard` -- Run scorecard tests
- `operator-sdk run bundle` -- Deploy operator via OLM bundle on a cluster

### Important: Future of the Operator SDK (November 2025)

Red Hat announced significant changes to the Operator SDK's relationship with OpenShift:

- **OpenShift 4.18 is the last release to ship the Operator SDK CLI** as a downstream component
- The **upstream CNCF Operator SDK project is NOT being sunset** -- it remains active
- Rationale: Kubernetes APIs have stabilized enough that newer client libraries work on older K8s versions, reducing the need for per-release SDK builds
- **Ansible Operator** is being separated into its own repository as a standalone Kubebuilder plugin
- **Helm Operator** remains part of the Operator SDK
- **Base images** (UBI9-based) for Ansible and Helm operators continue to be released with new OpenShift versions
- ISVs should continue updating to the latest upstream SDK CLI binary

**Latest upstream release highlights (2025-2026):**
- Kubernetes 1.31 API support
- Kubebuilder v4.2.0 scaffolding
- New `metrics-require-rbac` flag for Helm/Ansible runtimes
- UBI9 base image updates (ubi-minimal 9.6 to 9.7)
- golangci-lint v2 adoption

**Sources:**
- [The Future of the Red Hat OpenShift Operator SDK](https://www.redhat.com/en/blog/future-red-hat-openshift-operator-sdk)
- [Operator SDK Overview](https://sdk.operatorframework.io/docs/overview/)
- [Operator SDK GitHub Releases](https://github.com/operator-framework/operator-sdk/releases)
- [Red Hat Scholars -- Operator SDK Tutorial](https://redhat-scholars.github.io/operators-sdk-tutorial/template-tutorial/index.html)

---

## 3. Key Components

### Custom Resource Definitions (CRDs)

CRDs extend the Kubernetes API by defining new resource types. They declare the schema (using OpenAPI v3) for custom resources that users create to express desired state. Key considerations:

- **Versioning:** Start with `v1alpha1` until semantics stabilize, then promote to `v1beta1` and `v1`
- **Validation:** Use CEL (Common Expression Language) validation rules to enforce invariants without admission webhooks (a 2025 best practice)
- **Printer columns:** Define custom columns for `kubectl get` output to improve operational visibility
- **Status subresource:** Isolate status writes from spec mutations by enabling the `/status` subresource

### Controllers

Controllers implement the business logic that drives reconciliation. Each controller watches one primary resource type and optionally watches related (owned) resources.

**Architecture principles:**
- **One Controller Per Kind:** Avoid having a single controller manage multiple CRD types. This prevents complexity, scalability issues, responsibility diffusion, error isolation problems, and race conditions
- **Event-driven with predicates:** Use predicates to filter watch events and avoid unnecessary reconciliation (e.g., only reconcile when `metadata.generation` changes)

### Reconciliation Loop

The reconciliation loop is the core mechanism:

1. User applies a Custom Resource (desired state)
2. Kubernetes API server stores the CR
3. Controller detects the change via watch/informer
4. `Reconcile()` function is called
5. Controller reads the CR, compares actual state vs. desired state
6. Controller creates, updates, or deletes resources to converge
7. Controller updates the CR status
8. Loop repeats

**Critical:** The reconciliation loop operates on **level-based triggers**, not edge-based. The framework does not pass event type information (create/update/delete) -- the reconcile function must determine what action to take by examining current state. Writing logic tied to specific events breaks the operator pattern.

### Watches

Watches are the mechanism by which controllers receive events about resource changes:

- **`For()`** -- Specifies the primary resource the controller reconciles (the CRD)
- **`Owns()`** -- Watches resources created/owned by the primary resource (e.g., Deployments, Services created by the operator)
- **`Watches()`** -- Watches arbitrary resources not owned by the controller (e.g., ConfigMaps, Secrets from other sources)

Behind the scenes, watches use **informers** that establish HTTP streaming connections with the API server. The controller-runtime maintains a **cache** of watched resources to reduce API server load.

### RBAC

Operators need explicit permissions to interact with Kubernetes resources:

- Defined via annotations in controller source code: `//+kubebuilder:rbac:groups=...,resources=...,verbs=...`
- SDK generates `ClusterRole` and `ClusterRoleBinding` manifests from these annotations
- **Namespace-scoped operators** use `Role` and `RoleBinding` limited to specific namespaces
- **Cluster-scoped operators** use `ClusterRole` and `ClusterRoleBinding`
- **Best practice:** Default to namespace scope; grant cluster privileges only when necessary
- OLM uses **OperatorGroup** resources to control operator permissions across namespaces

**Sources:**
- [Controller Reconciliation -- Andreas Karis Blog](https://andreaskaris.github.io/blog/operator-sdk/operator-sdk-reconciliation/)
- [Beyond YAML: Building Kubernetes Operators with CRDs (DEV Community)](https://dev.to/naveens16/beyond-yaml-building-kubernetes-operators-with-crds-and-the-reconciliation-loop-524d)
- [Kubebuilder -- Good Practices](https://book.kubebuilder.io/reference/good-practices)
- [Operator Cache Configuration (Red Hat Developer, March 2026)](https://developers.redhat.com/articles/2026/03/31/unlocking-efficiency-guide-operator-cache-configuration-red-hat-openshift-and)
- [OpenShift RBAC Permissions Operator (GitHub)](https://github.com/openshift/rbac-permissions-operator)

---

## 4. Operator Lifecycle Manager (OLM)

### What is OLM?

OLM extends Kubernetes to provide a declarative way to install, manage, and upgrade operators and their dependencies. It handles:

- Dependency resolution between operators
- Update/upgrade orchestration
- RBAC management for installed operators
- Catalog management and discovery
- Multi-tenancy (namespace-based)

### OLM v0 (Current/Legacy)

The original OLM uses several key concepts:
- **CatalogSource:** Points to an index of available operators
- **Subscription:** Declares intent to install/update an operator from a catalog
- **InstallPlan:** Tracks the resources needed to install an operator
- **ClusterServiceVersion (CSV):** Describes a specific version of an operator
- **OperatorGroup:** Controls RBAC and namespace targeting

### OLM v1 (Next Generation -- Announced Late 2025)

OLM v1 is a ground-up redesign with significant improvements:

| Aspect | OLM v0 | OLM v1 |
|--------|--------|--------|
| **Install API** | Subscription + InstallPlan | Single ClusterExtension object |
| **Catalog management** | CRD-based | RESTful API (reduced API server load) |
| **Failure handling** | Manual (delete InstallPlan) | Automatic continuous reconciliation |
| **Version control** | Channel-based | Semver ranges (e.g., `3.12.x`, `~3.12`) |
| **Rollbacks** | Not supported | Optional rollback support |
| **CRD cleanup on uninstall** | CRDs retained | CRDs removed (may become optional) |
| **ServiceAccount** | Auto-generated | User-provided (least-privilege model) |

**Key OLM v1 lifecycle operations:**
1. **Exploration:** Query catalog images via `opm render`
2. **Installation:** Create and apply ClusterExtension objects
3. **Upgrades:** Edit ClusterExtension specs (pinned or range-based)
4. **Rollbacks:** Downgrade using `SelfCertified` upgrade constraint policy
5. **Access Control:** Configure RBAC for operator-provided APIs
6. **Uninstallation:** Delete ClusterExtension objects

**OLM v1 limitations (as of 2025-2026):**
- Operators must use `registry+v1` bundle format
- Must support `AllNamespaces` install mode
- Webhooks not supported
- No concrete migration strategy from v0 to v1

**Sources:**
- [OLM Documentation](https://olm.operatorframework.io/docs/)
- [Announcing OLM v1 (Red Hat Blog)](https://www.redhat.com/en/blog/announcing-olm-v1-next-generation-operator-lifecycle-management)
- [Manage Operators as ClusterExtensions with OLM v1 (Red Hat Developer)](https://developers.redhat.com/articles/2025/06/02/manage-operators-clusterextensions-olm-v1)
- [Understanding OpenShift's OLM (IBM Community)](https://community.ibm.com/community/user/blogs/manogya-sharma/2025/07/04/understanding-openshifts-operator-lifecycle-manage)
- [OLM v1 Documentation](https://operator-framework.github.io/operator-controller/)

---

## 5. Operator Bundle Format

### Bundle Structure

A bundle is a directory containing exactly one ClusterServiceVersion (CSV) plus associated CRDs and metadata. The standard layout:

```
my-operator/
  0.1.0/
    manifests/
      my-operator.clusterserviceversion.yaml   # Required: exactly one CSV
      my-crd.crd.yaml                          # CRDs for owned APIs
      additional-resources.yaml                 # Optional: other K8s resources
    metadata/
      annotations.yaml                         # Package metadata, format info
  0.2.0/
    manifests/
      ...
    metadata/
      ...
```

### ClusterServiceVersion (CSV)

The CSV is the central metadata document for an operator version. It is analogous to an RPM/DEB package manifest. Contents include:

- **Metadata:** Name, description, version, icon, repository link, labels, maintainers
- **Owned CRDs:** CRDs that the operator manages
- **Required CRDs:** CRDs the operator depends on (provided by other operators)
- **Install strategy:** Deployment spec, container image, environment variables
- **RBAC rules:** ClusterRole permissions needed by the operator
- **Cluster requirements:** Minimum Kubernetes/OpenShift version
- **Update path:** `replaces` field pointing to the previous CSV version
- **Webhooks:** Admission/conversion webhook definitions

### Bundle Images

Bundles are packaged as OCI container images for distribution:

1. Create manifests in the bundle directory structure
2. Build a container image using a Dockerfile
3. Push to a container registry
4. Reference from a catalog (index) image

### Catalog Sources

CatalogSources define references to available operator packages:

- **Index images:** Container images containing a database (SQLite or file-based catalog) of operator bundle references
- OLM pulls and extracts manifests from bundle images referenced in the catalog
- **Default Red Hat catalogs:** OpenShift ships with pre-configured catalog sources for Red Hat, Certified, and Community operators

### Update Modes

Three modes for OLM-managed updates:
1. **semver-mode** (default): Updates follow semantic versioning rules
2. **semver-skippatch-mode**: Skips patch versions during updates
3. **replaces-mode**: Explicit upgrade graph defined via `replaces` fields in CSVs

**Sources:**
- [OLM -- ClusterServiceVersion Documentation](https://olm.operatorframework.io/docs/concepts/crds/clusterserviceversion/)
- [Community Operators -- Operator Structure](https://k8s-operatorhub.github.io/community-operators/packaging-operator/)
- [operator-framework/operator-registry (GitHub)](https://github.com/operator-framework/operator-registry)

---

## 6. OperatorHub and Certification

### OperatorHub

OperatorHub is the marketplace/catalog for discovering and installing operators:

- **Embedded OperatorHub:** Built into OpenShift, accessible via the web console
- **OperatorHub.io:** Public catalog for the broader Kubernetes community (360+ operators)
- **Red Hat Ecosystem Catalog:** Hosts certified and validated operator listings

OpenShift ships three default catalogs:
1. **Red Hat Operators:** Red Hat-built and supported
2. **Certified Operators:** Partner-built, Red Hat-certified
3. **Community Operators:** Community-contributed

### Certification Paths (2025)

Red Hat offers two certification programs:

1. **Red Hat Certification (Full):**
   - Thorough testing using Red Hat's test suite
   - Collaborative joint support between partner and Red Hat
   - Products meet both partner and Red Hat criteria (interoperability, lifecycle management, security, support)
   - Published on Red Hat Ecosystem Catalog and embedded OperatorHub
   - Optional publication to Red Hat Marketplace (powered by IBM)

2. **Partner Validation:**
   - Partners validate using their own criteria and test suite on Red Hat platforms
   - Faster path to publishing
   - May not incorporate all Red Hat integration requirements and best practices

### Certification Prerequisites

- All container images referenced in the operator bundle must be certified first
- Container images must be published in the Red Hat Ecosystem Catalog before operator bundle certification begins

### Certification Workflow

1. Create a product listing on Red Hat Partner Connect portal
2. Certify all required container images
3. Run the certification test suite (uses OpenShift Pipelines / Tekton)
4. Tests produce real-time logs for debugging
5. Upon passing, the pipeline submits a PR to the certified-operators GitHub repository
6. After merge, the operator appears in the Red Hat Ecosystem Catalog and OperatorHub

### Growth Statistics

The Operator SDK has enabled growth from 25 operators in the OpenShift 4.0 catalog to over 450 operators across three catalogs in OpenShift 4.16.

**Sources:**
- [Red Hat OpenShift Operator Certification (Blog)](https://www.redhat.com/en/blog/red-hat-openshift-operator-certification)
- [Red Hat Software Certification Workflow Guide 2025](https://docs.redhat.com/en/documentation/red_hat_software_certification/2025/html-single/red_hat_software_certification_workflow_guide/index)
- [Red Hat OpenShift Operator Certification At-a-Glance](https://connect.redhat.com/en/blog/red-hat-openshift-operator-certification-glance)
- [Certified Operators GitHub Repository](https://github.com/redhat-openshift-ecosystem/certified-operators)

---

## 7. Testing Strategies

### Unit Testing

- Use Go's standard testing framework with **envtest** (from controller-runtime)
- envtest provides a local control plane (etcd + API server) without a full cluster
- SDK scaffolds `suite_test.go` with Ginkgo/Gomega test boilerplate
- Run via `make test` or `go test ./controllers/ -v`
- Test individual reconciliation logic, helper functions, and validation

### Integration Testing

- Also uses envtest to simulate a Kubernetes API server
- Tests controller behavior, webhooks, and component interactions
- Requires KUBECONFIG or default kubeconfig for cluster tests
- Tests components bound to external projects (e.g., OLM integration)

### End-to-End (E2E) Testing

| Operator Type | Recommended E2E Tool |
|---------------|---------------------|
| Go | Go test files with Ginkgo, or Chainsaw |
| Ansible | Molecule (Ansible testing framework) |
| Helm | Chart tests or shell scripts |
| Any | kuttl (declarative YAML), Chainsaw (modern alternative) |

- E2E tests run against real clusters (often kind -- Kubernetes in Docker)
- CI typically runs a matrix across multiple Kubernetes versions
- OpenShift-specific: `openshift-tests` utility for conformance validation
- **OSDe2e:** Comprehensive test framework for Managed OpenShift Clusters

### Scorecard Testing

The Operator SDK scorecard tests operator bundles for OLM compatibility and best practices:

**Built-in tests:**
- `basic-check-spec-test` -- Verifies CRs have a spec section
- `olm-bundle-validation-test` -- Validates bundle format
- `olm-crds-have-validation-test` -- Checks CRD validation rules
- `olm-crds-have-resources-test` -- Verifies CRD resource listings
- `olm-spec-descriptors-test` -- Checks spec descriptor completeness
- `olm-status-descriptors-test` -- Checks status descriptor completeness

**Custom scorecard tests:** Ship custom test logic in container images referenced in the scorecard configuration. Tests travel with the operator bundle, enabling functional testing without source code.

**Kuttl-based scorecard tests:** Declarative test definitions using YAML manifests bundled alongside the operator.

### Recommended Layered Approach

Following the OpenTelemetry Operator team's pattern:
1. **Unit tests** for individual features and functions
2. **envtest-based tests** for controller reconciliation logic
3. **E2E tests** (kuttl/Chainsaw) for end-user scenarios
4. **Scorecard tests** for OLM compliance and packaging validation

**Sources:**
- [Operator SDK -- Testing Your Operator Project](https://sdk.operatorframework.io/docs/building-operators/golang/testing/)
- [Operator SDK -- Scorecard](https://sdk.operatorframework.io/docs/testing-operators/scorecard/)
- [Operator SDK -- Kuttl Scorecard Tests](https://master.sdk.operatorframework.io/docs/testing-operators/scorecard/kuttl-tests/)
- [OpenTelemetry Operator Testing (DeepWiki)](https://deepwiki.com/open-telemetry/opentelemetry-operator/7.2-testing-the-operator)
- [openshift/openshift-tests (GitHub)](https://github.com/openshift/openshift-tests)

---

## 8. Best Practices

### Idempotency

- The reconciliation loop **must** be idempotent: running multiple times produces the same result without side effects or oscillations
- Check current state vs. desired state before making changes
- Never "read-modify-write" live objects blindly; compute desired state and use Server-Side Apply (SSA) with a consistent field manager
- Use SSA to enable GitOps-Operator coordination without reconciliation conflicts
- Avoid writing reconciliation logic tied to specific event types (create/update/delete)

### Status Reporting

- Use the **status subresource** to track and report the state of managed resources
- Implement **status conditions** following Kubernetes API conventions for standardized state representation
- All instantiated CRs should include a status block providing insight into application state
- Integrate with **Prometheus** for metrics collection (reconciliation duration, failure counts, queue depth)
- Use **Kubernetes events** and structured logging to report important actions and errors
- Use structured logging with correlation IDs tied to CR instances

### Finalizers

- Add finalizers when CR deletion requires external cleanup (cloud resources, DNS records, snapshots)
- Controller removes the finalizer only after cleanup completes
- Without finalizers, deleted CRs can leave dangling external resources (costly and insecure)
- Be careful with finalizer race conditions -- a common source of intermittent operator failures

### Leader Election

- Run multiple replicas for HA but ensure only one is actively reconciling via leader election
- Uses a lease-based algorithm: try to create/update a lock object, renew periodically
- If the leader crashes, the lock expires and another replica takes over
- Make leader tasks idempotent so duplicate processing does not cause issues
- Use readinessProbe to mark pods ready only after acquiring leadership
- Log all leadership transitions for debugging
- Kubernetes 1.36 introduces coordinated leader election (beta) for deterministic leader selection

### Upgrade Paths

- Support seamless upgrades of both operator and operand
- Operator upgrade should automatically reconcile operand resources to the new desired state
- Define clear update paths in CSV `replaces` fields or semantic version ranges
- Use OLM channels to provide different stability tracks (stable, fast, candidate)

### Additional Best Practices (2025-2026)

- **Predicates for efficiency:** Filter watch events to avoid unnecessary reconciliation; only reconcile on `metadata.generation` changes
- **CEL validation:** Enforce strong invariants in CRD schemas without admission webhooks
- **Security hardening:** Default to non-root execution, read-only filesystems, dropped Linux capabilities, image signing via Sigstore/Cosign
- **RBAC minimization:** Enforce least-privilege RBAC aligned with actual operator needs
- **Graceful failure handling:** Implement circuit-breakers, retries, and exponential backoff
- **Controller isolation:** One controller per Kind (Single Responsibility Principle)
- **Logging conventions:** Use capitalized messages without periods, active voice, past tense, structured key-value logging per Kubernetes standards

**Sources:**
- [Kubernetes Operators in 2025: Best Practices (OuterByte)](https://outerbyte.com/kubernetes-operators-2025-guide/)
- [Kubernetes Operators Best Practices (openshift.com)](https://www.openshift.com/blog/kubernetes-operators-best-practices)
- [Kubebuilder -- Good Practices](https://book.kubebuilder.io/reference/good-practices)
- [Building Bulletproof Leader Election (Medium)](https://medium.com/@ishaish103/building-bulletproof-leader-election-in-kubernetes-operators-a-deep-dive-4c82879d9d37)
- [Coordinated Leader Election (Kubernetes Docs)](https://kubernetes.io/docs/concepts/cluster-administration/coordinated-leader-election/)

---

## 9. Operator Capability Maturity Model

The Operator Framework defines 5 capability levels that describe the maturity of an operator's lifecycle management features. Each level builds on the previous.

### Level 1 -- Basic Install

- Automated application provisioning and configuration management
- Operator deploys an operand or configures off-cluster resources
- Operator waits for managed resources to reach a healthy state
- All configuration delivered through the CR spec
- Status block conveys application readiness
- Operators that delegate to off-cluster orchestration remain at this level

### Level 2 -- Seamless Upgrades

- Operand upgradeable via operator upgrade or CR changes
- Backward compatibility with older operand versions
- Operator reports inability to manage unsupported versions via status
- Non-disruptive upgrade processes
- Includes schema migrations and other application-specific upgrade steps
- Clear documentation of what is and is not upgraded

### Level 3 -- Full Lifecycle

- Backup and restore capabilities for operand data
- Complex re-configuration orchestration
- Failover and failback support for clustered systems
- Application-aware scaling of the operand
- Kubernetes resilience practices: probes, replicas, PodDisruptionBudgets

### Level 4 -- Deep Insights

- Operator exposes metrics about its own health
- Operator exposes health and performance metrics about the operand
- Custom Kubernetes events emitted for state changes
- Operand sends useful alerts
- Prometheus rules and Grafana dashboards automatically created
- Provides enough insight for users to understand application state

### Level 5 -- Auto Pilot

- Auto-scaling based on operand metrics (horizontal and vertical)
- Automatic healing of unhealthy operands based on metrics/alerts/logs
- Automatic performance tuning and workload optimization
- Abnormality detection against dynamically learned performance baselines
- Workload migration to optimal nodes, storage, or networks
- Minimal manual intervention required

**Key terminology:**
- **Operator:** The custom controller installed on the cluster
- **Operand:** The managed workload provided by the operator as a service

**Sources:**
- [Operator SDK -- Operator Capability Levels](https://sdk.operatorframework.io/docs/overview/operator-capabilities/)
- [Operator Framework -- Operator Capability Levels](https://operatorframework.io/operator-capabilities/)
- [Red Hat Blog -- Operators 101: Your Auto-Pilot for Kubernetes Workloads](https://www.redhat.com/en/blog/operators-101-your-auto-pilot-kubernetes-workloads)

---

## 10. Development Workflow

### From Scaffolding to Deployment

#### Step 1: Initialize the Project

```bash
# For a Go-based operator
operator-sdk init --domain example.com --repo github.com/example/my-operator

# For Ansible-based
operator-sdk init --plugins ansible --domain example.com

# For Helm-based
operator-sdk init --plugins helm --domain example.com
```

Kubebuilder (embedded in the SDK) generates the project structure: Makefile, Dockerfile, go.mod, main.go, config/ directory with Kustomize manifests.

#### Step 2: Create APIs (CRDs + Controllers)

```bash
operator-sdk create api --group myapp --version v1alpha1 --kind MyApp --resource --controller
```

This scaffolds:
- `api/v1alpha1/myapp_types.go` -- CR type definitions (spec, status)
- `controllers/myapp_controller.go` -- Controller with `Reconcile()` stub
- CRD manifests in `config/crd/`

#### Step 3: Define the API

Edit `myapp_types.go` to define the Spec and Status structs for your custom resource. Run `make generate` to update generated code and `make manifests` to regenerate CRD YAML.

#### Step 4: Implement Reconciliation Logic

Edit `myapp_controller.go` to implement the `Reconcile()` function:
- Read the CR
- Compare desired state with actual state
- Create/update/delete owned resources
- Update CR status
- Return result (requeue or done)

Add RBAC annotations and configure watches in `SetupWithManager()`.

#### Step 5: Test Locally

```bash
# Run unit/integration tests
make test

# Run the operator locally against a cluster (uses your kubeconfig)
make install   # Install CRDs
make run       # Run the operator outside the cluster
```

#### Step 6: Build and Push the Operator Image

```bash
make docker-build docker-push IMG=registry.example.com/my-operator:v0.1.0
```

#### Step 7: Deploy to Cluster

```bash
# Direct deployment (non-OLM)
make deploy IMG=registry.example.com/my-operator:v0.1.0

# Or via OLM bundle
make bundle IMG=registry.example.com/my-operator:v0.1.0
make bundle-build bundle-push BUNDLE_IMG=registry.example.com/my-operator-bundle:v0.1.0
operator-sdk run bundle registry.example.com/my-operator-bundle:v0.1.0
```

#### Step 8: Run Scorecard Tests

```bash
operator-sdk scorecard ./bundle
```

#### Step 9: Create a Catalog

```bash
# Build a catalog index containing the bundle
opm index add --bundles registry.example.com/my-operator-bundle:v0.1.0 \
  --tag registry.example.com/my-operator-catalog:latest
docker push registry.example.com/my-operator-catalog:latest
```

Apply a CatalogSource on the cluster pointing to this catalog image.

#### Step 10: Iterate and Certify

- Add new API versions, controllers, and features
- Run comprehensive tests (unit, integration, e2e, scorecard)
- For Red Hat certification: certify container images first, then the operator bundle through the Red Hat Partner Connect portal

### GitOps Integration (Recommended for Production)

- Separate concerns: platform repo (CRDs/Operators) vs. apps repo (team-owned CRs)
- Use Argo CD sync waves (`argocd.argoproj.io/sync-wave`) for ordering
- Configure GitOps to ignore operator-written fields and status subresources
- Use External Secrets Operator to avoid committing credentials
- Create a Job to verify operator installation succeeded before continuing the GitOps workflow

**Sources:**
- [Red Hat -- Developing Operators (OpenShift 4.11 Docs)](https://docs.redhat.com/en/documentation/openshift_container_platform/4.11/html/operators/developing-operators)
- [Red Hat -- OpenShift Operators: Concept and Working Example in Golang](https://www.redhat.com/en/blog/red-hat-openshift-operators-concept-and-working-example-golang)
- [How to Use OpenShift Operators (OneUpTime)](https://oneuptime.com/blog/post/2026-01-28-openshift-operators/view)
- [Demystifying Operator Deployment in OpenShift (IBM)](https://www.ibm.com/products/tutorials/demystifying-operator-deployment-in-openshift)
- [The Future of the Red Hat OpenShift Operator SDK](https://www.redhat.com/en/blog/future-red-hat-openshift-operator-sdk)

---

## Key Takeaways for 2025-2026

1. **The Operator SDK upstream project remains active** despite Red Hat decoupling it from OpenShift releases after 4.18. Continue using the latest upstream SDK binary.

2. **OLM v1 is the future** of operator lifecycle management, with a simplified ClusterExtension API, but currently has limitations. Plan for eventual migration from OLM v0.

3. **Server-Side Apply (SSA)** is the recommended approach for managing owned resources, replacing the traditional read-modify-write pattern.

4. **CEL validation** in CRDs is a must -- it replaces many use cases that previously required admission webhooks.

5. **Security hardening** (non-root, read-only filesystems, least-privilege RBAC, image signing) is now table-stakes for production operators.

6. **Testing should be layered:** unit tests with envtest, integration tests, e2e tests with Chainsaw/kuttl, and scorecard tests for OLM compliance.

7. **GitOps integration** (Argo CD, Flux) is the recommended deployment pattern for production operator management.

8. **Emerging trends:** AI-augmented reconciliation, WebAssembly modules for portable logic, and multi-runtime support are on the horizon.

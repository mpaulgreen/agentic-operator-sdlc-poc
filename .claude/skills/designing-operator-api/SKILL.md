---
name: designing-operator-api
description: >
  Designs Kubernetes CRD types from natural language requirements. Use when user asks to
  define an API, design a CRD, define types, add fields to a spec or status, add status
  conditions, add validation markers, add print columns, add webhooks, or add a new API version.
---

# Designing Operator API Types

Translate user requirements into properly annotated Go types for Kubernetes CRDs. Generates `_types.go` files with kubebuilder validation markers, status conditions, print columns, and optional webhooks — eliminating marker syntax trial-and-error.

## Template Variables

Collect these from the user. Many are derived from the description of what the operator manages.

| Variable | Description | Example | Default |
|----------|-------------|---------|---------|
| `KIND` | CRD kind (PascalCase) | `RedisCluster` | — (required) |
| `API_GROUP` | Short API group name | `cache` | — (required) |
| `DOMAIN` | API group domain | `redis.example.com` | — (required) |
| `API_VERSION` | API version | `v1alpha1` | `v1alpha1` |
| `NAMESPACED` | Whether namespace-scoped | `true` | `true` |

Derived:
- `KIND_LOWER` = lowercase of KIND
- `FULL_GROUP` = `API_GROUP.DOMAIN` (e.g., `cache.redis.example.com`)
- `YEAR` = current year for license header

## Workflow A: Design New CRD Types

Use when the user describes what their operator should manage and needs a complete types file.

1. **Collect requirements** from user — ask about:
   - What does this resource represent? (e.g., "a Redis cluster")
   - What Spec fields are needed? For each field: name, type, validation rules, default value
   - What Status should the user see? (phase, conditions, counters, endpoints)
   - What should `kubectl get` show? (print columns)
   - Namespaced or cluster-scoped?

2. **Map fields to Go types and markers.** For each Spec field, determine:
   - Go type (`int32`, `string`, `bool`, `*string` for optional, nested struct for complex)
   - Validation markers — see `references/validation-markers.md`:
     - Numeric: `+kubebuilder:validation:Minimum=N`, `Maximum=N`
     - Enum: `+kubebuilder:validation:Enum="val1";"val2"`
     - Pattern: `+kubebuilder:validation:Pattern=<regex>`
     - Default: `+kubebuilder:default=VALUE`
     - Required: `+kubebuilder:validation:Required`
   - For complex validation (cross-field), consider CEL rules — see `references/cel-validation-rules.md`

3. **Design Status struct.** Follow conventions in `references/status-conventions.md`:
   - Always include `Conditions []metav1.Condition` with patchStrategy merge
   - Add phase enum if appropriate (Pending/Running/Failed/etc.)
   - Add counters (readyReplicas, availableNodes)
   - Add connection info (endpoint, connectionString)

4. **Add print columns** for important fields:
   ```go
   // +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
   // +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
   // +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
   ```

5. **Generate the types file** (`api/<version>/<kind_lower>_types.go`):
   - License header from `hack/boilerplate.go.txt`
   - Package declaration
   - Imports (metav1, corev1 if using ResourceRequirements)
   - Spec struct with all fields + markers
   - Nested type structs (StorageSpec, BackupSpec, etc.) if needed
   - Status struct with Conditions
   - Root type with `+kubebuilder:object:root=true`, `+kubebuilder:subresource:status`, print columns
   - If cluster-scoped: `+kubebuilder:resource:scope=Cluster`
   - List type
   - `init()` with SchemeBuilder.Register

6. **Update DeepCopy** — regenerate `zz_generated.deepcopy.go` or run `make generate` to handle new nested types with slice/map fields that need deep copying.

7. **Verify** using `scripts/validate-api-types.py`.

## Workflow B: Add/Modify Fields on Existing CRD

Use when the user wants to add fields, change validation, or update status on an existing CRD.

1. **Read** the existing `_types.go` file.
2. **Identify** changes: new Spec fields, new Status fields, new nested types, changed markers.
3. **Apply** changes following the same marker patterns as Workflow A.
4. **Update DeepCopy** if new nested types with slices/maps were added.
5. **Verify** the modified file compiles and passes validation.

## Nested Type Design Patterns

When a group of related fields belongs together, extract into a nested struct:

```go
// StorageSpec defines storage configuration.
type StorageSpec struct {
    // +kubebuilder:validation:Pattern=`^[0-9]+[KMGT]i$`
    Size string `json:"size"`

    StorageClassName *string `json:"storageClassName,omitempty"`
}
```

Common nested patterns — see `references/type-design-patterns.md`:
- **StorageSpec** — size, storageClassName, accessModes
- **ResourceSpec** — embed `corev1.ResourceRequirements` directly
- **BackupSpec** — schedule (cron), retentionDays, destination
- **TLSSpec** — enabled, secretName, certFile, keyFile
- **SecretKeyValue** — name (secret name), key (key within secret)

Use pointers (`*NestedType`) for truly optional nested configs. Use value types for required nested configs.

## Field Type Guidelines

| User says | Go type | Marker |
|-----------|---------|--------|
| "number of replicas, 1-10" | `int32` | `+kubebuilder:validation:Minimum=1` `+kubebuilder:validation:Maximum=10` |
| "version, one of 14/15/16" | `string` | `+kubebuilder:validation:Enum="14";"15";"16"` |
| "storage size like 10Gi" | `string` | `+kubebuilder:validation:Pattern=^[0-9]+[KMGT]i$` |
| "optional backup schedule" | `*string` | `+kubebuilder:validation:Pattern=<cron-regex>` |
| "enabled by default" | `bool` | `+kubebuilder:default=true` |
| "cpu and memory limits" | `corev1.ResourceRequirements` | (embedded K8s type, no custom marker) |
| "reference to a secret" | `SecretKeyValue` (custom) | nested struct with name + key |
| "list of allowed IPs" | `[]string` | (slice, needs DeepCopy) |

## Workflow C: Add Webhooks (Pattern H)

Use when the user needs defaulting logic (set defaults programmatically), validation beyond markers (cross-field rules, external checks), or both.

1. **Determine webhook type** — ask the user:
   - **Defaulting** (`--defaulting`): Set default values in `Default()` method. Use when defaults are computed or conditional.
   - **Validating** (`--programmatic-validation`): Custom validation in `ValidateCreate/Update/Delete()`. Use for cross-field rules, OneOf patterns, business logic.
   - **Both**: Most common — defaulting + validating together.

2. **Generate webhook handler** (`api/<version>/<kind_lower>_webhook.go`):
   - License header
   - `SetupWebhookWithManager()` function registering the webhook
   - Kubebuilder webhook markers (mutating + validating paths)
   - `Default()` method stub — see `references/webhook-patterns.md`
   - `ValidateCreate()`, `ValidateUpdate()`, `ValidateDelete()` stubs
   - See `assets/templates/webhook.go.tmpl` for the template

3. **Generate webhook config files**:
   - `config/webhook/service.yaml` — Service on port 443→9443
   - `config/webhook/kustomization.yaml` — references manifests + service
   - `config/webhook/kustomizeconfig.yaml` — kustomize substitution rules
   - `config/certmanager/certificate.yaml` — self-signed issuer + certificate
   - `config/certmanager/kustomization.yaml` — cert-manager resources
   - `config/certmanager/kustomizeconfig.yaml` — cert-manager substitutions
   - `config/default/manager_webhook_patch.yaml` — adds port 9443 + cert volume mount
   - `config/default/webhookcainjection_patch.yaml` — CA injection annotations
   - `config/crd/patches/webhook_in_<kind_lower>.yaml` — CRD conversion config

4. **Update existing files**:
   - `cmd/main.go` — add `SetupWebhookWithManager()` call:
     ```go
     if err = (&cachev1alpha1.RedisCluster{}).SetupWebhookWithManager(mgr); err != nil {
         setupLog.Error(err, "unable to create webhook", "webhook", "RedisCluster")
         os.Exit(1)
     }
     ```
   - `config/default/kustomization.yaml` — uncomment `[WEBHOOK]` and `[CERTMANAGER]` sections, add `patches` for `manager_webhook_patch.yaml` and `webhookcainjection_patch.yaml`, and add a `replacements` section that maps: (1) webhook Service name/namespace → Certificate dnsNames, (2) Certificate name/namespace → CA injection annotations on MutatingWebhookConfiguration and ValidatingWebhookConfiguration. Without the `replacements` section, the certificate will have literal placeholder DNS names and webhook TLS will fail with x509 errors.
   - `config/crd/kustomization.yaml` — uncomment webhook patch

## Workflow D: Add API Version (Pattern G)

Use when promoting an API from experimental to stable (v1alpha1 → v1beta1 → v1).

1. **Determine version progression**:
   - v1alpha1 → v1beta1: Stabilizing, may still have breaking changes
   - v1beta1 → v1: Stable, no breaking changes expected
   - The newest version becomes the **storage version**

2. **Create new version directory** (`api/<new-version>/`):
   - `groupversion_info.go` — new GroupVersion with same group, new version
   - `<kind_lower>_types.go` — types for new version (may have different fields)
   - `zz_generated.deepcopy.go` — DeepCopy for new version types

3. **Update storage version markers**:
   - Add `+kubebuilder:storageversion` to the ROOT TYPE in the NEW version
   - Remove `+kubebuilder:storageversion` from the old version (if present)

4. **Update main.go**:
   - Add import for new version package
   - Add scheme registration
   - Set up webhook for new version if applicable

5. **For conversion between versions**: Use Workflow C to add a conversion webhook with hub-and-spoke pattern. See `references/api-versioning.md`.

## Files Produced

| Workflow | Files Generated |
|----------|----------------|
| A | `api/<version>/<kind_lower>_types.go` |
| B | Modifies existing `_types.go` |
| C | `<kind_lower>_webhook.go` + 9 config files + updates main.go and kustomizations |
| D | New `api/<version>/` directory with groupversion_info.go + types.go |

If nested types are added, DeepCopy methods in `zz_generated.deepcopy.go` need regeneration.

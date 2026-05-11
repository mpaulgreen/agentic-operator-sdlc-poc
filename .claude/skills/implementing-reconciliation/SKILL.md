---
name: implementing-reconciliation
description: >
  Implements Kubernetes controller reconciliation logic. Use when user asks to
  implement a reconciler, write controller logic, add a resource to reconcile,
  add a finalizer, handle deletion, fix error handling, add status updates,
  or add event recording to an operator controller.
---

# Implementing Reconciliation

Generate controller reconciliation logic with idempotent resource management, proper error handling, status updates, and event recording. This is the core of operator development — the reconciliation loop that creates, updates, and manages Kubernetes resources.

## Three-Phase Reconciliation Pattern

Every reconciler follows this structure:

```
Reconcile(ctx, req)
  ├── PHASE 1: FETCH
  │   ├── Get CR from cluster
  │   ├── If not found → return (deleted)
  │   └── If deleting → handleDeletion()
  ├── PHASE 2: ORCHESTRATE
  │   ├── Add finalizer (if first reconcile)
  │   ├── Set phase to Initializing
  │   ├── reconcileSecret()      ← dependency order
  │   ├── reconcileConfigMap()
  │   ├── reconcileService()
  │   └── reconcileStatefulSet()
  └── PHASE 3: STATUS
      ├── Fetch dependent resource (StatefulSet)
      ├── Copy status fields (readyReplicas, endpoint)
      ├── Update phase (Initializing → Running)
      └── Update conditions (Available, Progressing, Degraded)
```

## Workflow A: Implement Reconciliation for New Controller

Use when the user has a scaffolded controller stub and needs the full reconciliation logic.

1. **Collect requirements** — ask the user:
   - What Kubernetes resources does the controller create? (Secret, ConfigMap, Service, Deployment, StatefulSet, etc.)
   - What's the dependency order? (e.g., Secret before StatefulSet because StatefulSet mounts the Secret)
   - Does the controller need finalizers? (yes if external cleanup needed)
   - What status should be reported? (phase, readyReplicas, conditions)

2. **Generate controller files** — split across multiple files for maintainability:
   - `<kind>_controller.go` — Reconcile() + SetupWithManager() + RBAC markers. See `assets/templates/controller.go.tmpl`.
   - `<kind>_reconcilers.go` — one `reconcileX()` method per resource. See `assets/templates/reconciler-method.go.tmpl`.
   - `<kind>_status.go` — updateStatus() + updatePhase(). See `assets/templates/status-updater.go.tmpl`.
   - `<kind>_conditions.go` — condition type constants + setCondition() + convenience setters. See `assets/templates/conditions.go.tmpl`.
   - `<kind>_helpers.go` — labelsForCluster(), naming conventions. See `assets/templates/helpers.go.tmpl`.

3. **For each resource type**, generate a `reconcileX()` method following the check-create pattern:
   ```
   GET existing → if exists, check-update → if not found, BUILD + SET_OWNER_REF + CREATE + RECORD_EVENT
   ```
   See `references/idempotency-patterns.md` for the full pattern.

4. **Add RBAC annotations** — one marker per managed resource type:
   ```go
   //+kubebuilder:rbac:groups=<group>,resources=<plural>,verbs=get;list;watch;create;update;patch;delete
   ```
   See `references/rbac-annotations.md`.

5. **Update SetupWithManager()** — add `Owns()` for each reconciled resource type:
   ```go
   ctrl.NewControllerManagedBy(mgr).
       For(&v1alpha1.RedisCluster{}).
       Owns(&appsv1.StatefulSet{}).
       Owns(&corev1.Service{}).
       Complete(r)
   ```

6. **Add finalizer lifecycle** if external cleanup needed. See `references/finalizer-lifecycle.md`.

7. **Verify** with `scripts/validate-rbac-annotations.py` and `scripts/check-idempotency.py`.

## Workflow B: Add or Modify Resources in Existing Controller

Use when the user wants to add a new reconciled resource or modify what an existing reconciler produces.

**Adding a new resource** (e.g., "add a ConfigMap for redis.conf"):

1. Add new `reconcileX()` method following the same check-create pattern as existing methods.
2. Add a resource builder function (e.g., `configMapForCluster()`).
3. Call the new method from `Reconcile()` in the correct dependency position.
4. Add RBAC annotation for the new resource type.
5. Add `Owns(&corev1.ConfigMap{})` to `SetupWithManager()`.
6. Update status if the new resource affects status.

**Modifying an existing reconciler** (e.g., "add anti-affinity to the StatefulSet"):

7. When modifying an existing reconciler, **audit the entire check-update section** — not just the field you're adding. Verify that ALL mutable spec fields set in the builder have corresponding comparisons in the check-update path. Fields that were missing check-update coverage before your change are still bugs — fix them while you're in the file. Common fields that need check-update on mutable resources (StatefulSet, Deployment): replicas, image, resources, affinity, tolerations, env vars, volume mounts.
8. For complex nested fields (affinity, tolerations, volumes, resources), use a helper with `reflect.DeepEqual` or struct comparison — see `references/idempotency-patterns.md` "Check-Update for Complex Fields".
9. Batch multiple field comparisons into a single `r.Update()` call to avoid unnecessary API calls.

## Check-Create Idempotency Pattern

The most-repeated pattern in operator development. For each resource type:

```go
func (r *Reconciler) reconcileSecret(ctx context.Context, cr *v1alpha1.MyKind) error {
    name := fmt.Sprintf("%s-credentials", cr.Name)

    // 1. CHECK if exists
    existing := &corev1.Secret{}
    err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, existing)
    if err == nil {
        return nil  // EXISTS — idempotent, nothing to do
    }
    if !errors.IsNotFound(err) {
        return err  // ACTUAL ERROR
    }

    // 2. BUILD desired state
    secret := &corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: cr.Namespace,
            Labels:    labelsForCluster(cr),
        },
        StringData: map[string]string{"password": generatePassword()},
    }

    // 3. SET OWNER REFERENCE (for garbage collection)
    if err := controllerutil.SetControllerReference(cr, secret, r.Scheme); err != nil {
        return err
    }

    // 4. CREATE
    if err := r.Create(ctx, secret); err != nil {
        r.Recorder.Event(cr, corev1.EventTypeWarning, "SecretFailed", err.Error())
        return err
    }

    // 5. RECORD SUCCESS EVENT
    r.Recorder.Event(cr, corev1.EventTypeNormal, "SecretCreated", name)
    return nil
}
```

See `references/idempotency-patterns.md` for check-update, Server-Side Apply, and complex variations.

## Requeue Strategies

| Return | Behavior |
|--------|----------|
| `ctrl.Result{}, nil` | Done, no requeue |
| `ctrl.Result{}, err` | Requeue with exponential backoff (transient failure) |
| `ctrl.Result{RequeueAfter: 10*time.Second}, nil` | Poll after duration |
| `ctrl.Result{}, reconcile.TerminalError(err)` | Permanent failure, no retry |

## File Organization

A well-structured controller splits across 5-7 files:

| File | Content | Lines |
|------|---------|-------|
| `<kind>_controller.go` | Reconcile(), SetupWithManager(), RBAC | ~100 |
| `<kind>_reconcilers.go` | reconcileSecret/ConfigMap/Service/StatefulSet | ~200 |
| `<kind>_status.go` | updateStatus(), updatePhase() | ~80 |
| `<kind>_conditions.go` | condition types, constants, setters | ~120 |
| `<kind>_helpers.go` | labels, naming, utilities | ~40 |

For complex operators (10+ resource types), use a generic `createOrUpdate()` helper instead of individual methods. See `assets/examples/complex-reconciler.go`.

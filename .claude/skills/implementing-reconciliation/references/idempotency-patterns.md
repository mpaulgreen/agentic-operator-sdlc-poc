# Idempotency Patterns

## Check-Create (Most Common)

```go
existing := &corev1.Secret{}
err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, existing)
if err == nil { return nil }                    // EXISTS — idempotent
if !errors.IsNotFound(err) { return err }       // ACTUAL ERROR
// DOES NOT EXIST — create
```

Three states: exists (no-op), not found (create), error (propagate).

## Check-Update (For Mutable Resources)

When the spec changes, update the existing resource:

```go
existing := &appsv1.StatefulSet{}
err := r.Get(ctx, key, existing)
if err != nil {
    if errors.IsNotFound(err) {
        // Create new
        return r.Create(ctx, desired)
    }
    return err
}

// Compare and update if different
if *existing.Spec.Replicas != cr.Spec.Replicas {
    existing.Spec.Replicas = &cr.Spec.Replicas
    return r.Update(ctx, existing)
}
```

Use check-update for: StatefulSet replicas, Deployment image, ConfigMap data.
Use check-create only for: Secrets (credentials shouldn't change), Services (selectors are immutable).

## Check-Update for Complex Fields

When updating nested structs (affinity, tolerations, volumes, env vars), field-by-field comparison is fragile. Use `reflect.DeepEqual` via a helper:

```go
// Helper for comparing complex fields
func affinityEqual(a, b *corev1.Affinity) bool {
    if a == nil && b == nil { return true }
    if a == nil || b == nil { return false }
    return reflect.DeepEqual(a, b)
}
```

Batch multiple field comparisons into a single Update call:

```go
existing := &appsv1.StatefulSet{}
err := r.Get(ctx, key, existing)
if err == nil {
    updated := false
    if *existing.Spec.Replicas != cr.Spec.Replicas {
        existing.Spec.Replicas = &cr.Spec.Replicas
        updated = true
    }
    desiredAffinity := podAffinityForCluster(cr)
    if !affinityEqual(existing.Spec.Template.Spec.Affinity, desiredAffinity) {
        existing.Spec.Template.Spec.Affinity = desiredAffinity
        updated = true
    }
    if updated {
        return r.Update(ctx, existing)
    }
    return nil
}
```

**Key rule**: Every mutable spec field set in the builder MUST have a corresponding comparison in the check-update section. When modifying a reconciler, audit ALL builder fields — not just the one you're adding. Fields that were already missing check-update coverage are bugs; fix them while you're in the file.

**Common mutable fields for StatefulSet/Deployment**: replicas, image, resources (CPU/memory), affinity, tolerations, env vars, volume mounts. All of these can change when the user updates the CR spec and must be reconciled on existing resources.

## Server-Side Apply (Modern Alternative)

SSA reconstructs desired state from scratch each reconciliation:

```go
desired := r.buildDeployment(cr)  // pure function, no API calls
err := r.Patch(ctx, desired, client.Apply, client.FieldOwner("my-controller"), client.ForceOwnership)
```

**Advantages**: No Get-then-Update race. Handles conflicts automatically. Cleaner code.
**Caveats**: Fake client doesn't support SSA. Requires all managed fields in every apply. More complex for partial updates.

## Owner References (Required for All Patterns)

Every created resource must have an owner reference:

```go
if err := controllerutil.SetControllerReference(cr, object, r.Scheme); err != nil {
    return err
}
```

This enables:
- Garbage collection (child deleted when parent deleted)
- `Owns()` watches (child changes trigger parent reconcile)

## Event Recording (Required for All Patterns)

```go
// On success
r.Recorder.Event(cr, corev1.EventTypeNormal, "SecretCreated", name)

// On failure
r.Recorder.Event(cr, corev1.EventTypeWarning, "SecretFailed", err.Error())
```

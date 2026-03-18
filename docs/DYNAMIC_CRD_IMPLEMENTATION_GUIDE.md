# Dynamic CRD Implementation Guide

## Overview

This document describes the implementation of dynamic CRD support for VolumeGroupReplication types in Ramen. The implementation allows Ramen to work with either:
- **volrep CRDs** from `replication.storage.openshift.io` (csi-addons)
- **neutral CRDs** from `replication.storage.io` (replication-storage-io-crds)

The system automatically detects which CRDs are available at runtime and uses the appropriate types, preventing crashes when volrep CRDs are not present.

## Architecture

### Components Created

1. **CRD Detector** (`internal/controller/replication/crd_detector.go`)
   - Detects CRD availability at runtime
   - Caches results for performance
   - Thread-safe implementation

2. **Abstraction Interfaces** (`internal/controller/replication/interface.go`)
   - Common interfaces for both CRD types
   - `VolumeGroupReplicationInterface`
   - `VolumeGroupReplicationClassInterface`
   - `VolumeGroupReplicationContentInterface`

3. **Volrep Wrapper** (`internal/controller/replication/volrep_wrapper.go`)
   - Wraps csi-addons volrep types
   - Implements common interfaces
   - Handles field differences

4. **Neutral Wrapper** (`internal/controller/replication/neutral_wrapper.go`)
   - Wraps replication.storage.io neutral types
   - Implements same interfaces
   - Full feature parity

5. **Factory Pattern** (`internal/controller/replication/factory.go`)
   - Creates appropriate type based on CRD availability
   - Provides type conversion utilities
   - Simplifies object creation

## Implementation Details

### CRD Detection

```go
detector := replication.NewCRDDetector(client)
if detector.IsVolumeGroupReplicationAvailable(ctx) {
    // Use volrep types
} else {
    // Use neutral types
}
```

### Controller Watches

Controllers now conditionally watch CRDs based on availability:

**volumereplicationgroup_controller.go:**
```go
if crdDetector.IsVolumeGroupReplicationAvailable(ctx) {
    ctrlBuilder.Watches(&volrep.VolumeGroupReplication{}, ...)
} else {
    ctrlBuilder.Watches(&neutral.VolumeGroupReplication{}, ...)
}
```

**drclusterconfig_controller.go:**
```go
if crdDetector.IsVolumeGroupReplicationClassAvailable(ctx) {
    ctrlBuilder.Watches(&volrep.VolumeGroupReplicationClass{}, ...)
} else {
    ctrlBuilder.Watches(&neutral.VolumeGroupReplicationClass{}, ...)
}
```

### Factory Usage

```go
factory := replication.NewReplicationFactory(ctx, client)

// Create new object
vgr := factory.NewVolumeGroupReplication(name, namespace)

// Wrap existing object
wrapped := factory.WrapVolumeGroupReplication(obj)

// Check which type is being used
if factory.IsUsingVolrep() {
    // volrep CRDs are available
}
```

## Remaining Work

### 1. Update Direct Type References (84+ occurrences)

Files requiring updates:
- `internal/controller/vrg_volgrouprep.go` - Core VGR logic
- `internal/controller/drpolicy_peerclass.go` - Peer class matching
- `internal/controller/s3utils.go` - S3 operations
- `internal/controller/util/mcv_util.go` - ManagedClusterView utilities

**Pattern to follow:**
```go
// OLD:
vgr := &volrep.VolumeGroupReplication{}
err := client.Get(ctx, namespacedName, vgr)

// NEW:
factory := replication.NewReplicationFactory(ctx, client)
vgrObj := factory.GetVolumeGroupReplicationType()
err := client.Get(ctx, namespacedName, vgrObj)
vgr := factory.WrapVolumeGroupReplication(vgrObj)
```

### 2. Update RBAC Permissions

File: `config/dr-cluster/rbac/role.yaml`

Make CRD permissions conditional or use both:
```yaml
# Volrep CRDs (optional)
- apiGroups:
  - replication.storage.openshift.io
  resources:
  - volumegroupreplications
  - volumegroupreplicationclasses
  - volumegroupreplicationcontents
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch

# Neutral CRDs (fallback)
- apiGroups:
  - replication.storage.io
  resources:
  - volumegroupreplications
  - volumegroupreplicationclasses
  - volumegroupreplicationcontents
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
```

### 3. Add Runtime CRD Checks

Add checks in reconciliation loops:
```go
func (r *VRGInstance) reconcileVolGroupRepsAsPrimary(...) {
    factory := replication.NewReplicationFactory(r.ctx, r.reconciler.Client)
    
    if !factory.IsUsingVolrep() && !factory.IsUsingNeutral() {
        // Neither CRD type available, log warning
        r.log.Info("No VolumeGroupReplication CRDs available")
        return
    }
    
    // Continue with reconciliation
}
```

### 4. Update Tests

Create test scenarios for both CRD types:
```go
func TestVGRWithVolrepCRDs(t *testing.T) {
    // Test with volrep CRDs present
}

func TestVGRWithNeutralCRDs(t *testing.T) {
    // Test with neutral CRDs present
}

func TestVGRWithNoCRDs(t *testing.T) {
    // Test graceful handling when no CRDs present
}
```

## Dependencies

### go.mod Updates

```go
require (
    github.com/ramendr/replication-storage-io-crds/api v0.0.0-00010101000000-000000000000
)

replace github.com/ramendr/replication-storage-io-crds/api => /Users/benamar/projects/github/replication-storage-io-crds/api
```

## Benefits

1. **No Crashes**: Ramen won't crash when volrep CRDs are missing
2. **Flexibility**: Works with either CRD implementation
3. **Backward Compatible**: Existing volrep deployments continue to work
4. **Future-Proof**: Easy to add support for additional CRD types
5. **Clean Abstraction**: Business logic doesn't need to know which CRD type is used

## Migration Path

### For Existing Deployments (with volrep CRDs)
- No changes required
- System automatically detects and uses volrep CRDs
- Behavior remains identical

### For New Deployments (without volrep CRDs)
- Install neutral CRDs from replication-storage-io-crds
- Ramen automatically detects and uses neutral types
- Full functionality maintained

### For Transitioning Deployments
1. Install neutral CRDs alongside volrep CRDs
2. System continues using volrep (detected first)
3. Remove volrep CRDs when ready
4. System automatically switches to neutral types

## Testing Strategy

1. **Unit Tests**: Test each wrapper and interface implementation
2. **Integration Tests**: Test CRD detection and factory creation
3. **E2E Tests**: Test full workflows with both CRD types
4. **Negative Tests**: Test behavior when CRDs are missing

## Performance Considerations

- CRD detection is cached to avoid repeated API calls
- Cache can be cleared if CRDs are installed/removed dynamically
- Minimal overhead from abstraction layer (interface calls)

## Troubleshooting

### Issue: Controller fails to start
**Solution**: Check logs for CRD detection messages. Ensure at least one set of CRDs is installed.

### Issue: Wrong CRD type being used
**Solution**: Clear CRD detector cache or restart controller. Check CRD installation order.

### Issue: Performance degradation
**Solution**: Verify CRD detection cache is working. Check for excessive CRD availability checks.

## Future Enhancements

1. **Dynamic CRD Switching**: Support switching between CRD types without restart
2. **CRD Priority**: Allow configuration of preferred CRD type
3. **Metrics**: Add metrics for CRD type usage and detection
4. **Validation**: Add validation webhooks for both CRD types

## References

- Volrep CRDs: `github.com/csi-addons/kubernetes-csi-addons`
- Neutral CRDs: `github.com/ramendr/replication-storage-io-crds`
- Controller Runtime: `sigs.k8s.io/controller-runtime`
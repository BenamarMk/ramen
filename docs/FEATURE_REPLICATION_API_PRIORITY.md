# Feature: VRG Replication API Priority Annotation

## Summary

Added support for an annotation on VolumeReplicationGroup (VRG) resources that allows explicit control over which replication API to use: the VolumeReplication API from csi-addons (volrep) or the neutral replication API.

## Motivation

Previously, Ramen automatically detected which replication CRDs were available and used volrep if present, otherwise falling back to neutral. This made it difficult to:

1. **Test both APIs** on the same cluster without installing/uninstalling CRDs
2. **Force a specific API** even when both are available
3. **Migrate gradually** from one API to another
4. **Debug API-specific issues** by switching between implementations

## Implementation

### New Annotation

**Key:** `ramendr.openshift.io/replication-api-priority`

**Values:**
- `volrep` - Prefer VolumeReplication API from csi-addons
- `neutral` - Use neutral replication API

### Behavior

The decision logic in `ReplicationFactory.IsUsingVolrep()` now follows this order:

1. **Check annotation** - If present and valid, use the specified priority
   - `neutral` → Always use neutral API
   - `volrep` → Use volrep if CRDs available, else neutral
2. **Auto-detect** - If no annotation or invalid value, detect based on CRD availability

### Code Changes

**Files Modified:**
- `api/v1alpha1/volumereplicationgroup_types.go` - Added annotation constants
- `internal/controller/replication/factory.go` - Added annotation support and updated decision logic
- `internal/controller/volumereplicationgroup_controller.go` - Pass VRG annotations to factory

**Files Added:**
- `docs/replication-api-priority.md` - Comprehensive documentation
- `examples/vrg_with_api_priority.yaml` - Usage examples
- `internal/controller/replication/factory_annotation_test.go` - Tests

## Usage Example

```yaml
apiVersion: ramendr.openshift.io/v1alpha1
kind: VolumeReplicationGroup
metadata:
  name: my-app-vrg
  namespace: my-app
  annotations:
    ramendr.openshift.io/replication-api-priority: "neutral"
spec:
  pvcSelector:
    matchLabels:
      app: my-app
  replicationState: primary
  s3Profiles:
    - s3-profile-1
  async:
    schedulingInterval: "5m"
```

## Benefits

1. **Testing Flexibility** - Test both APIs on the same cluster by changing annotation
2. **Explicit Control** - Override auto-detection when needed
3. **Migration Support** - Gradually migrate workloads between APIs
4. **Debugging** - Isolate API-specific issues
5. **Environment Consistency** - Force same API across different environments

## Backward Compatibility

✅ **Fully backward compatible**

- Existing VRGs without the annotation continue to work with auto-detection
- No changes to default behavior
- No breaking changes to existing APIs

## Testing

- Unit tests verify annotation constants and SetAnnotations() method
- Integration tests can be added to verify behavior with both APIs
- Manual testing confirmed compilation and basic functionality

## Documentation

- Main documentation: `docs/replication-api-priority.md`
- Examples: `examples/vrg_with_api_priority.yaml`
- README updated with reference to new feature

## Future Enhancements

Potential future improvements:
1. Add metrics to track which API is being used
2. Add validation webhook to reject invalid annotation values
3. Add status field to VRG showing which API is active
4. Support per-PVC API selection for mixed workloads

## Related Issues

This feature enables testing and comparison of both replication APIs as discussed in the project requirements.
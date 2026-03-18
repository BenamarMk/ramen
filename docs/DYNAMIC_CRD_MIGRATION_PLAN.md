# Dynamic CRD Migration Execution Plan

## Current Status
- Foundation: ✅ Complete (55%)
- Migration: ⏳ In Progress (0% - just started)
- Import added to vrg_volgrouprep.go but not used yet

## Challenge
The migration is complex because:
1. **103 occurrences** of volrep types across multiple files
2. **Cascading dependencies** - changing one method signature affects many callers
3. **Type conversions** needed between concrete types and interfaces
4. **S3 operations** that serialize/deserialize concrete types

## Revised Strategy: Incremental with Backward Compatibility

Instead of changing method signatures immediately (which breaks many callers), we'll:
1. Keep existing methods working with volrep types
2. Add NEW methods that use interfaces alongside old ones
3. Gradually migrate callers to new methods
4. Remove old methods once all callers migrated

This allows us to:
- Make progress incrementally
- Test each change
- Maintain backward compatibility during migration
- Avoid breaking 103 call sites at once

## Phase 1: Add Runtime CRD Checks (PRIORITY)

**Goal:** Prevent crashes when CRDs don't exist
**Effort:** 1 hour
**Risk:** Low
**Impact:** High - immediate value

### Files to Modify:
1. `internal/controller/vrg_volgrouprep.go`
   - Add checks in `reconcileVolGroupRepsAsPrimary`
   - Add checks in `reconcileVolGroupRepsAsSecondary`
   - Add checks in `restoreVGRsAndVGRCsForVolRep`

### Implementation:
```go
func (v *VRGInstance) reconcileVolGroupRepsAsPrimary(groupPVCs map[types.NamespacedName][]*corev1.PersistentVolumeClaim) {
    // Add at start of method
    if !v.replicationFactory.IsUsingVolrep() && !v.replicationFactory.IsUsingNeutral() {
        v.log.Info("No VolumeGroupReplication CRDs available, skipping VGR reconciliation")
        return
    }
    
    // Rest of existing code unchanged
    ...
}
```

**Benefits:**
- ✅ Prevents crashes immediately
- ✅ No breaking changes to existing code
- ✅ Easy to test
- ✅ Can be done in 1 commit

## Phase 2: Add Wrapper Helper Methods (NEW APPROACH)

**Goal:** Create interface-based helpers WITHOUT breaking existing code
**Effort:** 2 hours
**Risk:** Low
**Impact:** Medium - enables future migration

### Add New Methods (don't modify existing):

```go
// New interface-based method
func (v *VRGInstance) getVGRCFromVGRInterface(vgr replication.VolumeGroupReplicationInterface) (replication.VolumeGroupReplicationContentInterface, error) {
    vgrcName := vgr.GetSpec().GetVolumeGroupReplicationContentName()
    vgrcObjectKey := client.ObjectKey{Name: vgrcName}
    
    vgrcObj := v.replicationFactory.GetVolumeGroupReplicationContentType()
    if err := v.reconciler.Get(v.ctx, vgrcObjectKey, vgrcObj); err != nil {
        return nil, fmt.Errorf("failed to get VGRC %v from VGR %v, %w",
            vgrcObjectKey, client.ObjectKeyFromObject(vgr.GetObject()), err)
    }
    
    return v.replicationFactory.WrapVolumeGroupReplicationContent(vgrcObj), nil
}

// Keep existing method unchanged
func (v *VRGInstance) getVGRCFromVGR(vgr *volrep.VolumeGroupReplication) (volrep.VolumeGroupReplicationContent, error) {
    // Existing implementation unchanged
    ...
}
```

### New Methods to Add:
1. `getVGRCFromVGRInterface()` - interface version of `getVGRCFromVGR()`
2. `getVGRUsingSCLabelInterface()` - interface version of `getVGRUsingSCLabel()`
3. `createOrUpdateVGRInterface()` - interface version of `createOrUpdateVGR()`
4. `deleteVGRInterface()` - interface version of `deleteVGR()`

**Benefits:**
- ✅ No breaking changes
- ✅ Can test new methods independently
- ✅ Gradual migration path
- ✅ Old code continues working

## Phase 3: S3 Operations Abstraction

**Goal:** Handle S3 upload/download for both types
**Effort:** 3 hours
**Risk:** Medium
**Impact:** High - enables full dynamic support

### Challenge:
S3 operations currently serialize concrete volrep types:
```go
func UploadVGR(s ObjectStorer, vgrKeyPrefix, vgrKeySuffix string,
    vgr volrep.VolumeGroupReplication) error
```

### Solution: Type-Agnostic S3 Operations

```go
// New generic upload that works with any type
func UploadVGRGeneric(s ObjectStorer, vgrKeyPrefix, vgrKeySuffix string,
    vgr client.Object) error {
    // Serialize the object (works for any client.Object)
    vgrJSON, err := json.Marshal(vgr)
    if err != nil {
        return fmt.Errorf("failed to marshal VGR: %w", err)
    }
    
    // Rest of upload logic
    ...
}

// Keep existing for backward compatibility
func UploadVGR(s ObjectStorer, vgrKeyPrefix, vgrKeySuffix string,
    vgr volrep.VolumeGroupReplication) error {
    return UploadVGRGeneric(s, vgrKeyPrefix, vgrKeySuffix, &vgr)
}
```

### Files to Modify:
1. `internal/controller/s3utils.go`
   - Add `UploadVGRGeneric()`
   - Add `UploadVGRCGeneric()`
   - Add `downloadVGRsGeneric()`
   - Add `downloadVGRCsGeneric()`
   - Keep existing methods as wrappers

**Benefits:**
- ✅ Works with both volrep and neutral types
- ✅ Backward compatible
- ✅ Enables dynamic restore operations

## Phase 4: Controller Integration

**Goal:** Use factory in reconciliation loops
**Effort:** 2 hours
**Risk:** Medium
**Impact:** High - completes dynamic support

### Approach:
Update reconciliation methods to use factory when creating new VGRs:

```go
func (v *VRGInstance) createVGR(vrNamespacedName types.NamespacedName,
    pvcs []*corev1.PersistentVolumeClaim, state volrep.ReplicationState) error {
    
    // Use factory to create appropriate type
    vgrInterface := v.replicationFactory.NewVolumeGroupReplication(
        vrNamespacedName.Name,
        vrNamespacedName.Namespace,
    )
    
    // Set spec using interface methods
    vgrInterface.GetSpec().SetReplicationState(state)
    vgrInterface.GetSpec().SetAutoResync(v.autoResync(state))
    // ... set other fields
    
    // Get underlying object for Kubernetes operations
    vgrObj := vgrInterface.GetObject()
    
    if err := v.reconciler.Create(v.ctx, vgrObj); err != nil {
        return fmt.Errorf("failed to create VGR: %w", err)
    }
    
    return nil
}
```

**Benefits:**
- ✅ Creates correct type based on available CRDs
- ✅ Seamless switching between implementations
- ✅ No manual type checking needed

## Phase 5: Test Suite

**Goal:** Comprehensive tests for both scenarios
**Effort:** 3 hours
**Risk:** Low
**Impact:** High - ensures reliability

### Tests to Create:
1. `internal/controller/replication/crd_detector_test.go`
2. `internal/controller/replication/factory_test.go`
3. `internal/controller/replication/wrapper_test.go`
4. `internal/controller/vrg_volgrouprep_dynamic_test.go`

### Test Scenarios:
- VRG reconciliation with volrep CRDs
- VRG reconciliation with neutral CRDs
- VRG reconciliation with no CRDs (graceful handling)
- S3 upload/download with both types
- Factory type selection
- Wrapper implementations

## Execution Order (Revised)

### Week 1: Critical Path
1. **Day 1:** Phase 1 - Runtime CRD Checks (1 hour)
   - Immediate crash prevention
   - High value, low risk
   - 1 commit, easy to review

2. **Day 2:** Phase 2 - Wrapper Helpers (2 hours)
   - Add new interface-based methods
   - No breaking changes
   - 1 commit, easy to test

3. **Day 3:** Phase 3 - S3 Operations (3 hours)
   - Generic S3 upload/download
   - Backward compatible
   - 2 commits (upload, then download)

### Week 2: Integration
4. **Day 4-5:** Phase 4 - Controller Integration (2 hours)
   - Update createVGR to use factory
   - Update other reconciliation methods
   - 3-4 commits (one per major method)

5. **Day 6-7:** Phase 5 - Test Suite (3 hours)
   - Comprehensive test coverage
   - Both scenarios validated
   - 2 commits (unit tests, integration tests)

## Success Metrics

### After Phase 1 (Day 1):
- ✅ Ramen doesn't crash when VGR CRDs missing
- ✅ Logs indicate CRDs not available
- ✅ Graceful degradation

### After Phase 2 (Day 2):
- ✅ New interface-based helpers available
- ✅ Old code still works
- ✅ Can start using new methods

### After Phase 3 (Day 3):
- ✅ S3 operations work with both types
- ✅ Can upload/download neutral CRDs
- ✅ Backward compatible

### After Phase 4 (Day 5):
- ✅ New VGRs created using factory
- ✅ Correct type selected automatically
- ✅ Full dynamic support operational

### After Phase 5 (Day 7):
- ✅ Comprehensive test coverage
- ✅ Both scenarios validated
- ✅ Production ready

## Risk Mitigation

### Risk: Breaking existing functionality
**Mitigation:** 
- Keep old methods alongside new ones
- Gradual migration
- Extensive testing at each step

### Risk: S3 serialization issues
**Mitigation:**
- Generic JSON marshaling works for all types
- Test with both implementations
- Backward compatible wrappers

### Risk: Type conversion errors
**Mitigation:**
- Factory handles all conversions
- Wrappers tested independently
- Clear error messages

## Rollback Plan

Each phase is independent and can be rolled back:
- **Phase 1:** Remove CRD checks (1 commit revert)
- **Phase 2:** Remove new methods (1 commit revert)
- **Phase 3:** Remove generic S3 ops (2 commits revert)
- **Phase 4:** Revert controller changes (3-4 commits revert)
- **Phase 5:** Remove tests (2 commits revert)

## Next Immediate Action

**START WITH PHASE 1** - Runtime CRD Checks

This provides immediate value (crash prevention) with minimal risk and no breaking changes. It's the foundation for everything else and can be completed in 1 hour.

File to modify: `internal/controller/vrg_volgrouprep.go`
Methods to update:
1. `reconcileVolGroupRepsAsPrimary()` - line 30
2. `reconcileVolGroupRepsAsSecondary()` - line 70
3. `restoreVGRsAndVGRCsForVolRep()` - line 878

Add 3-5 lines at the start of each method to check CRD availability and return early if none available.
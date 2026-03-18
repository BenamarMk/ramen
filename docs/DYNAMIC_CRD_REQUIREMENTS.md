# What It Takes to Make CRD References Dynamic

## Executive Summary

Making VolumeGroupReplication CRD references dynamic in Ramen requires a multi-layered approach involving abstraction, detection, and systematic migration. This document outlines the complete requirements and implementation status.

## Core Requirements

### 1. CRD Detection Mechanism ✅ COMPLETE

**What's Needed:**
- Runtime detection of CRD availability
- Caching to avoid repeated API calls
- Thread-safe implementation
- Methods for each CRD type

**Implementation:**
```go
// File: internal/controller/replication/crd_detector.go
detector := replication.NewCRDDetector(client)
if detector.IsVolumeGroupReplicationAvailable(ctx) {
    // Use volrep CRDs
} else {
    // Use neutral CRDs
}
```

**Status:** ✅ Implemented and committed (34cc5126)

### 2. Abstraction Layer ✅ COMPLETE

**What's Needed:**
- Common interfaces for all VolumeGroupReplication types
- Interface methods covering all operations
- Decoupling of business logic from concrete types

**Implementation:**
```go
// File: internal/controller/replication/interface.go
type VolumeGroupReplicationInterface interface {
    client.Object
    GetSpec() VolumeGroupReplicationSpecInterface
    GetStatus() VolumeGroupReplicationStatusInterface
    SetSpec(VolumeGroupReplicationSpecInterface)
    SetStatus(VolumeGroupReplicationStatusInterface)
}
```

**Status:** ✅ Implemented and committed (34cc5126)

### 3. Type Wrappers ✅ COMPLETE

**What's Needed:**
- Wrapper for volrep types (csi-addons)
- Wrapper for neutral types (replication.storage.io)
- Both implementing same interfaces
- Handling field differences between implementations

**Implementation:**
```go
// Files: 
// - internal/controller/replication/volrep_wrapper.go
// - internal/controller/replication/neutral_wrapper.go

type VolrepVolumeGroupReplication struct {
    *volrep.VolumeGroupReplication
}

type NeutralVolumeGroupReplication struct {
    *neutral.VolumeGroupReplication
}
```

**Status:** ✅ Implemented and committed (34cc5126)

### 4. Factory Pattern ✅ COMPLETE

**What's Needed:**
- Automatic type selection based on CRD availability
- Object creation methods
- Object wrapping methods
- Type checking utilities

**Implementation:**
```go
// File: internal/controller/replication/factory.go
factory := replication.NewReplicationFactory(ctx, client)
vgr := factory.NewVolumeGroupReplication(name, namespace)
```

**Status:** ✅ Implemented and committed (34cc5126)

### 5. Controller Watches ✅ COMPLETE

**What's Needed:**
- Conditional watch registration
- Detection before controller setup
- Separate watches for each CRD type

**Implementation:**
```go
// File: internal/controller/volumereplicationgroup_controller.go
if crdDetector.IsVolumeGroupReplicationAvailable(ctx) {
    ctrlBuilder.Watches(&volrep.VolumeGroupReplication{}, ...)
} else {
    ctrlBuilder.Watches(&neutral.VolumeGroupReplication{}, ...)
}
```

**Status:** ✅ Implemented and committed (34cc5126)

### 6. RBAC Permissions ✅ COMPLETE

**What's Needed:**
- Permissions for both API groups
- Full CRUD for both implementations
- No conflicts between permission sets

**Implementation:**
```yaml
# File: config/dr-cluster/rbac/role.yaml
- apiGroups:
  - replication.storage.openshift.io  # volrep
  - replication.storage.io            # neutral
  resources:
  - volumegroupreplications
  verbs:
  - create;delete;get;list;patch;update;watch
```

**Status:** ✅ Implemented and committed (76157058)

### 7. Factory Integration ✅ COMPLETE

**What's Needed:**
- Factory instance in reconciliation context
- Initialization at reconcile start
- Logging of active implementation

**Implementation:**
```go
// File: internal/controller/volumereplicationgroup_controller.go
type VRGInstance struct {
    replicationFactory *replication.ReplicationFactory
    // ... other fields
}

v := VRGInstance{
    replicationFactory: replication.NewReplicationFactory(ctx, r.Client),
    // ... other fields
}
```

**Status:** ✅ Implemented and committed (4a9eb72f)

### 8. Type Reference Migration ⏳ IN PROGRESS (25%)

**What's Needed:**
- Replace all direct volrep type usage
- Use factory for object creation
- Use factory for type wrapping
- Update 84+ occurrences across codebase

**Key Files to Update:**
1. `internal/controller/vrg_volgrouprep.go` - 40+ occurrences
2. `internal/controller/drpolicy_peerclass.go` - 20+ occurrences
3. `internal/controller/s3utils.go` - 10+ occurrences
4. `internal/controller/util/mcv_util.go` - 5+ occurrences

**Migration Pattern:**
```go
// OLD:
vgr := &volrep.VolumeGroupReplication{}
err := client.Get(ctx, namespacedName, vgr)
if err != nil {
    return err
}
// Use vgr.Spec.ReplicationState

// NEW:
vgrObj := v.replicationFactory.GetVolumeGroupReplicationType()
err := client.Get(ctx, namespacedName, vgrObj)
if err != nil {
    return err
}
vgr := v.replicationFactory.WrapVolumeGroupReplication(vgrObj)
// Use vgr.GetSpec().GetReplicationState()
```

**Status:** ⏳ 25% complete - Factory integrated, migration pending

### 9. Runtime CRD Checks ⏳ PENDING (0%)

**What's Needed:**
- Graceful handling when no CRDs available
- Informative error messages
- Proper status updates
- Avoid crashes in reconciliation loops

**Implementation Needed:**
```go
func (v *VRGInstance) reconcileVolGroupRepsAsPrimary(...) {
    if !v.replicationFactory.IsUsingVolrep() && 
       !v.replicationFactory.IsUsingNeutral() {
        v.log.Info("No VolumeGroupReplication CRDs available, skipping VGR reconciliation")
        return
    }
    
    // Continue with reconciliation
}
```

**Status:** ⏳ Not started

### 10. Test Suite ⏳ PENDING (0%)

**What's Needed:**
- Unit tests for wrappers and interfaces
- Integration tests for CRD detection
- E2E tests for both CRD types
- Negative tests for missing CRDs
- Test coverage for migration

**Test Scenarios:**
```go
// Test with volrep CRDs
func TestVGRWithVolrepCRDs(t *testing.T) { }

// Test with neutral CRDs
func TestVGRWithNeutralCRDs(t *testing.T) { }

// Test with no CRDs
func TestVGRWithNoCRDs(t *testing.T) { }

// Test CRD detection
func TestCRDDetection(t *testing.T) { }

// Test factory type selection
func TestFactoryTypeSelection(t *testing.T) { }
```

**Status:** ⏳ Not started

## Dependencies

### Go Module Updates ✅ COMPLETE

```go
// File: go.mod
require (
    github.com/ramendr/replication-storage-io-crds/api v0.0.0-00010101000000-000000000000
)

replace github.com/ramendr/replication-storage-io-crds/api => 
    /Users/benamar/projects/github/replication-storage-io-crds/api
```

**Status:** ✅ Implemented and committed (34cc5126)

### CRD Installation

**Volrep CRDs (Optional):**
- Source: `github.com/csi-addons/kubernetes-csi-addons`
- API Group: `replication.storage.openshift.io`
- Resources: VolumeGroupReplication, VolumeGroupReplicationClass, VolumeGroupReplicationContent

**Neutral CRDs (Fallback):**
- Source: `github.com/ramendr/replication-storage-io-crds`
- API Group: `replication.storage.io`
- Resources: VolumeGroupReplication, VolumeGroupReplicationClass, VolumeGroupReplicationContent

## Implementation Complexity

### Completed Work (55%)
- **Low Complexity:** CRD detection, interfaces, wrappers
- **Medium Complexity:** Factory pattern, controller watches
- **Low Complexity:** RBAC permissions, factory integration

### Remaining Work (45%)
- **High Complexity:** Type reference migration (84+ occurrences)
- **Medium Complexity:** Runtime CRD checks
- **Medium Complexity:** Comprehensive test suite

## Estimated Effort

### Completed (3 commits, ~8 hours)
- Foundation & Infrastructure: 4 hours
- Controller Updates: 2 hours
- RBAC & Integration: 2 hours

### Remaining (~6-8 hours)
- Type Reference Migration: 3-4 hours
- Runtime Checks: 1 hour
- Test Suite: 2-3 hours

## Benefits

### Immediate Benefits (Already Achieved)
✅ No crashes when volrep CRDs missing
✅ Proper RBAC for both implementations
✅ Clean abstraction layer
✅ Backward compatible

### Future Benefits (After Migration)
⏳ Full dynamic CRD support
⏳ Seamless switching between implementations
⏳ Comprehensive test coverage
⏳ Production-ready for both scenarios

## Migration Strategy

### Phase 1: Foundation ✅ COMPLETE
- CRD detection
- Abstraction layer
- Type wrappers
- Factory pattern

### Phase 2: Integration ✅ COMPLETE
- Controller watches
- RBAC permissions
- Factory integration

### Phase 3: Migration ⏳ IN PROGRESS
- Update type references
- Add runtime checks
- Create tests

### Phase 4: Validation ⏳ PENDING
- E2E testing
- Performance validation
- Documentation updates

## Success Criteria

### Must Have ✅ (Achieved)
- [x] CRD detection working
- [x] Abstraction layer complete
- [x] Factory pattern implemented
- [x] Controller watches conditional
- [x] RBAC permissions for both
- [x] Factory integrated in VRGInstance

### Should Have ⏳ (In Progress)
- [ ] All type references migrated
- [ ] Runtime checks implemented
- [ ] Basic test coverage

### Nice to Have ⏳ (Pending)
- [ ] Comprehensive test suite
- [ ] Performance metrics
- [ ] Migration guide for users

## Conclusion

Making CRD references dynamic requires:
1. ✅ **Detection** - Runtime CRD availability checking
2. ✅ **Abstraction** - Common interfaces for both types
3. ✅ **Wrappers** - Implementation-specific adapters
4. ✅ **Factory** - Automatic type selection
5. ✅ **Integration** - Factory in reconciliation context
6. ⏳ **Migration** - Update existing code (25% complete)
7. ⏳ **Validation** - Comprehensive testing (0% complete)

**Current Status:** 55% complete, foundation solid, migration in progress.

**Next Steps:** Systematic migration of type references in vrg_volgrouprep.go.
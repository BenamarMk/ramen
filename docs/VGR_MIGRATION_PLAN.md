# VGR Migration Plan: Dynamic API Resolution

## Problem Statement

Currently, the codebase has **154 direct references** to `volrep.VolumeGroupReplication` (legacy ODF/Ceph API). These need to be migrated to use the handler interface for dynamic resolution between:
- **Legacy API**: `replication.storage.openshift.io/v1alpha1` (ODF/Ceph)
- **Neutral API**: `replication.storage.io/v1alpha1` (Community Standard)

## Current State Analysis

### Files with Direct VGR References

1. **VRG Controller** (`volumereplicationgroup_controller.go`)
   - 15+ direct references to `volrep.VolumeGroupReplication`
   - Watches, Owns, and direct CRUD operations

2. **VGR Operations** (`vrg_volgrouprep.go`)
   - 30+ direct references
   - Upload/download to S3, archiving, restoration

3. **S3 Utils** (`s3utils.go`)
   - Upload/download VGR and VGRC to S3 stores
   - Type-specific serialization

4. **DRPolicy PeerClass** (`drpolicy_peerclass.go`)
   - Discovery of VGRClasses from clusters
   - Type: `[]*volrep.VolumeGroupReplicationClass`

5. **DRClusterConfig Controller** (`drclusterconfig_controller.go`)
   - Lists and watches VGRClasses
   - Validates cluster configuration

6. **Test Files** (multiple)
   - 100+ references in test code
   - Mock objects, assertions, test fixtures

## Migration Strategy

### Phase 1: Extend Handler Interface (DONE ✅)
- ✅ Created `VGRHandler` interface in `internal/controller/replication/interface.go`
- ✅ Implemented `LegacyHandler` for ODF/Ceph API
- ✅ Implemented `NeutralHandler` for community standard API
- ✅ Created `HandlerSelector` for dynamic selection

### Phase 2: VRG Controller Migration (IN PROGRESS 🔄)

#### 2.1 Controller Setup
**File**: `volumereplicationgroup_controller.go`

**Current**:
```go
Watches(&volrep.VolumeGroupReplication{}, ...)
Owns(&volrep.VolumeGroupReplication{})
```

**Target**:
```go
// Watch both APIs dynamically
Watches(&unstructured.Unstructured{}, ...).
    WithEventFilter(vgrEventFilter())  // Filter for both GVKs
```

**Implementation**:
- Use `unstructured.Unstructured` for watches
- Add GVK filter to handle both `replication.storage.openshift.io` and `replication.storage.io`
- Update `VGRMapFunc` to work with unstructured objects

#### 2.2 VRG CRUD Operations
**File**: `vrg_volgrouprep.go`

**Current Pattern**:
```go
vgr := &volrep.VolumeGroupReplication{}
err := v.reconciler.Get(v.ctx, namespacedName, vgr)
```

**Target Pattern**:
```go
// Use handler interface
handler, err := v.selectHandler(storageClassName)
if err != nil {
    return err
}

vgrStatus, err := handler.GetVGR(v.ctx, v.reconciler.Client, namespacedName)
```

**Functions to Migrate**:
1. `vgrHandlerGet()` - ✅ Already has TODO comment
2. `vgrHandlerCreate()` - ✅ Already has TODO comment  
3. `vgrHandlerUpdate()` - ✅ Already has TODO comment
4. `vgrHandlerDelete()` - ✅ Already has TODO comment
5. `getVGRUsingSCLabel()` - Needs migration
6. `deleteVGRIfUnused()` - Needs migration
7. `updateVGR()` - Needs migration
8. `deleteVGR()` - Needs migration

### Phase 3: S3 Operations Migration

#### 3.1 S3 Upload/Download
**File**: `s3utils.go`

**Current**:
```go
func UploadVGR(s ObjectStorer, vgrKeyPrefix, vgrKeySuffix string,
    vgr volrep.VolumeGroupReplication) error
```

**Target**:
```go
func UploadVGR(s ObjectStorer, vgrKeyPrefix, vgrKeySuffix string,
    vgrData map[string]interface{}) error  // Use unstructured data
```

**Alternative** (Better):
```go
// Keep type-agnostic by serializing to JSON
func UploadVGRJSON(s ObjectStorer, vgrKeyPrefix, vgrKeySuffix string,
    vgrJSON []byte) error
```

#### 3.2 VGR Archiving
**File**: `vrg_volgrouprep.go`

**Functions**:
- `uploadVGRandVGRCtoS3Stores()` - Convert to use handler
- `UploadVGRandVGRCtoS3Store()` - Convert to use handler
- `UploadVGRAndVGRCtoS3()` - Convert to use handler
- `getVGRCFromVGR()` - Convert to use handler

**Strategy**:
- Handler returns VGR as `map[string]interface{}` or JSON
- S3 operations work with serialized data
- Restoration deserializes based on detected API

### Phase 4: PeerClass Discovery Migration

#### 4.1 DRPolicy PeerClass Discovery
**File**: `drpolicy_peerclass.go` (Line 36)

**Current**:
```go
type classLists struct {
    sClasses   []*storagev1.StorageClass
    vrClasses  []*volrep.VolumeReplicationClass
    vgrClasses []*volrep.VolumeGroupReplicationClass  // TODO: Support neutral API
    vgsClasses []*groupsnapv1beta1.VolumeGroupSnapshotClass
}
```

**Target**:
```go
type classLists struct {
    sClasses   []*storagev1.StorageClass
    vrClasses  []*volrep.VolumeReplicationClass
    vgrClasses []VGRClassInfo  // Use interface type from handler
    vgsClasses []*groupsnapv1beta1.VolumeGroupSnapshotClass
}
```

**Implementation**:
```go
func getVGRClassesFromCluster(
    u *drpolicyUpdater,
    m util.ManagedClusterViewGetter,
    clusterName string,
) ([]VGRClassInfo, error) {
    // Try neutral API first
    neutralHandler := replication.NewNeutralHandler()
    if available, _ := neutralHandler.IsAvailable(u.ctx, u.client); available {
        return neutralHandler.DiscoverVGRClasses(u.ctx, u.client, "", "", "")
    }
    
    // Fallback to legacy API
    legacyHandler := replication.NewLegacyHandler()
    return legacyHandler.DiscoverVGRClasses(u.ctx, u.client, "", "", "")
}
```

#### 4.2 DRClusterConfig Validation
**File**: `drclusterconfig_controller.go`

**Current**:
```go
vgrClasses := &volrep.VolumeGroupReplicationClassList{}
if err := r.Client.List(ctx, vgrClasses); err != nil {
```

**Target**:
```go
// Use handler for discovery
handler, _ := r.selectHandler(ctx)
vgrClasses, err := handler.DiscoverVGRClasses(ctx, r.Client, "", "", "")
```

### Phase 5: Test Migration

#### 5.1 Unit Tests
**Strategy**: Keep typed imports in tests for convenience
- Tests can use `volrep.VolumeGroupReplication` directly
- Tests verify handler interface works correctly
- No need to refactor test code extensively

#### 5.2 Integration Tests
**File**: `vrg_volgrouprep_integration_test.go`

**Current**: Tests only legacy API
**Target**: Test both APIs
- Create test cases for legacy API
- Create test cases for neutral API
- Verify handler selection works correctly

### Phase 6: Controller Watches Migration

#### 6.1 Dynamic GVK Watching
**Challenge**: Controller-runtime `Watches()` expects typed objects

**Solution**: Use unstructured with GVK filtering

```go
// Helper to create GVK filter
func vgrGVKFilter() predicate.Predicate {
    return predicate.Funcs{
        CreateFunc: func(e event.CreateEvent) bool {
            return isVGRObject(e.Object)
        },
        UpdateFunc: func(e event.UpdateEvent) bool {
            return isVGRObject(e.ObjectNew)
        },
        DeleteFunc: func(e event.DeleteEvent) bool {
            return isVGRObject(e.Object)
        },
    }
}

func isVGRObject(obj client.Object) bool {
    gvk := obj.GetObjectKind().GroupVersionKind()
    return (gvk.Group == "replication.storage.openshift.io" || 
            gvk.Group == "replication.storage.io") &&
           gvk.Kind == "VolumeGroupReplication"
}
```

## Implementation Phases

### Phase 2A: VRG Controller Core Operations (Week 1)
- [ ] Migrate `vgrHandlerGet()` to use handler interface
- [ ] Migrate `vgrHandlerCreate()` to use handler interface
- [ ] Migrate `vgrHandlerUpdate()` to use handler interface
- [ ] Migrate `vgrHandlerDelete()` to use handler interface
- [ ] Add handler selection logic based on StorageClass
- [ ] Update unit tests

### Phase 2B: VRG Controller Extended Operations (Week 2)
- [ ] Migrate `getVGRUsingSCLabel()` 
- [ ] Migrate `deleteVGRIfUnused()`
- [ ] Migrate `updateVGR()`
- [ ] Migrate `deleteVGR()`
- [ ] Update integration tests

### Phase 3: S3 Operations (Week 3)
- [ ] Refactor S3 upload/download to use JSON serialization
- [ ] Update `uploadVGRandVGRCtoS3Stores()`
- [ ] Update `UploadVGRandVGRCtoS3Store()`
- [ ] Update `UploadVGRAndVGRCtoS3()`
- [ ] Update `getVGRCFromVGR()`
- [ ] Test S3 operations with both APIs

### Phase 4: PeerClass Discovery (Week 4)
- [ ] Update `classLists` struct to use `VGRClassInfo`
- [ ] Implement `getVGRClassesFromCluster()` with dual API support
- [ ] Update DRPolicy controller to use new discovery
- [ ] Update DRClusterConfig controller
- [ ] Test peerClass discovery with both APIs

### Phase 5: Controller Watches (Week 5)
- [ ] Implement GVK filter for VGR objects
- [ ] Update controller setup to watch unstructured
- [ ] Update `VGRMapFunc` to handle unstructured
- [ ] Test watch functionality with both APIs

### Phase 6: Testing & Validation (Week 6)
- [ ] Create comprehensive integration tests
- [ ] Test migration scenarios (legacy → neutral)
- [ ] Test dual API scenarios (both installed)
- [ ] Performance testing
- [ ] Documentation updates

## Key Design Decisions

### 1. Handler Selection Strategy
**Decision**: Select handler based on StorageClass `offloaded` label
- `offloaded=true` → Use neutral API
- `offloaded=false` or missing → Use legacy API

**Rationale**: Aligns with existing peerClass mechanism

### 2. S3 Serialization Format
**Decision**: Use JSON for S3 storage (API-agnostic)
- Store VGR as JSON bytes
- Deserialize based on detected API during restoration

**Rationale**: Avoids type-specific serialization issues

### 3. Controller Watches
**Decision**: Use unstructured with GVK filtering
- Watch both API groups simultaneously
- Filter events based on GVK

**Rationale**: Allows dynamic API support without controller restart

### 4. Test Strategy
**Decision**: Keep typed imports in tests
- Tests use concrete types for clarity
- Handler interface tested separately

**Rationale**: Test code doesn't need to be API-agnostic

## Migration Risks & Mitigation

### Risk 1: Breaking Existing Deployments
**Mitigation**: 
- Maintain backward compatibility
- Legacy API remains default
- Gradual rollout with feature flags

### Risk 2: S3 Data Compatibility
**Mitigation**:
- Version S3 data format
- Support reading both old and new formats
- Migration tool for existing S3 data

### Risk 3: Performance Impact
**Mitigation**:
- Cache handler selection results
- Minimize API discovery calls
- Benchmark before/after

### Risk 4: Test Coverage Gaps
**Mitigation**:
- Comprehensive integration tests
- Test both APIs in CI/CD
- Canary deployments

## Success Criteria

1. ✅ All VGR operations use handler interface
2. ✅ Both legacy and neutral APIs supported
3. ✅ No breaking changes to existing deployments
4. ✅ All tests pass (unit + integration)
5. ✅ Performance within 5% of baseline
6. ✅ Documentation complete

## Timeline

- **Week 1-2**: VRG Controller migration
- **Week 3**: S3 operations migration
- **Week 4**: PeerClass discovery migration
- **Week 5**: Controller watches migration
- **Week 6**: Testing & validation

**Total**: 6 weeks for complete migration

## Next Steps

1. Review and approve this migration plan
2. Create detailed implementation tasks
3. Set up feature branch for migration work
4. Begin Phase 2A implementation
5. Regular progress reviews

---

**Status**: Draft - Awaiting Approval
**Author**: Bob (AI Assistant)
**Date**: 2026-03-17
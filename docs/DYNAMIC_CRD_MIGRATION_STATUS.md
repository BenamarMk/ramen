# Dynamic CRD Migration Status

## Current Implementation Status: 55% Complete

### ✅ Completed Work (4 Commits)

#### Commit 1: Foundation (34cc5126)
**Files Created:**
- `internal/controller/replication/crd_detector.go` - CRD detection with caching
- `internal/controller/replication/interface.go` - Abstraction interfaces
- `internal/controller/replication/volrep_wrapper.go` - Volrep type wrappers
- `internal/controller/replication/neutral_wrapper.go` - Neutral type wrappers
- `internal/controller/replication/factory.go` - Factory pattern
- `docs/DYNAMIC_CRD_IMPLEMENTATION_GUIDE.md` - Implementation guide

**Files Modified:**
- `go.mod` - Added replication-storage-io-crds dependency
- `internal/controller/volumereplicationgroup_controller.go` - Conditional VGR watches
- `internal/controller/drclusterconfig_controller.go` - Conditional VGRClass watches

#### Commit 2: RBAC (76157058)
**Files Modified:**
- `config/dr-cluster/rbac/role.yaml` - Added permissions for neutral CRDs

#### Commit 3: Factory Integration (4a9eb72f)
**Files Modified:**
- `internal/controller/volumereplicationgroup_controller.go` - Added replicationFactory to VRGInstance

#### Commit 4: Documentation (ed4c5d7c)
**Files Created:**
- `docs/DYNAMIC_CRD_REQUIREMENTS.md` - Comprehensive requirements document

### ⏳ Remaining Work

#### 1. Type Reference Migration (25% → 100%)

**Scope:** 84+ occurrences across 4 main files

##### File: `internal/controller/vrg_volgrouprep.go` (40+ occurrences)

**Methods requiring signature changes (12 methods):**
```go
// Current signatures using concrete types:
func (v *VRGInstance) isVGRandVGRCArchivedAlready(vgr *volrep.VolumeGroupReplication, log logr.Logger) bool
func (v *VRGInstance) UploadVGRandVGRCtoS3Store(s3ProfileName string, vgr *volrep.VolumeGroupReplication) error
func (v *VRGInstance) UploadVGRandVGRCtoS3Stores(vgr *volrep.VolumeGroupReplication, log logr.Logger) ([]string, error)
func (v *VRGInstance) getVGRCFromVGR(vgr *volrep.VolumeGroupReplication) (volrep.VolumeGroupReplicationContent, error)
func (v *VRGInstance) getVGRUsingSCLabel(pvc *corev1.PersistentVolumeClaim) (*volrep.VolumeGroupReplication, error)
func (v *VRGInstance) deleteVGRIfUnused(vgr *volrep.VolumeGroupReplication) error
func (v *VRGInstance) addArchivedAnnotationForVGRandVGRC(vgr *volrep.VolumeGroupReplication, log logr.Logger) error
func (v *VRGInstance) validateExistingVGRC(vgrc *volrep.VolumeGroupReplicationContent) error
func (v *VRGInstance) validateExistingVGR(vgr *volrep.VolumeGroupReplication) error
func (v *VRGInstance) cleanupVGRCForRestore(vgrc *volrep.VolumeGroupReplicationContent) error
func (v *VRGInstance) cleanupVGRForRestore(vgr *volrep.VolumeGroupReplication) error
func (v *VRGInstance) processVGRCSecrets(vgrc *volrep.VolumeGroupReplicationContent) error

// Should become (using interfaces):
func (v *VRGInstance) isVGRandVGRCArchivedAlready(vgr replication.VolumeGroupReplicationInterface, log logr.Logger) bool
func (v *VRGInstance) UploadVGRandVGRCtoS3Store(s3ProfileName string, vgr replication.VolumeGroupReplicationInterface) error
// ... etc
```

**Direct instantiations requiring factory usage (~30 occurrences):**
```go
// Pattern 1: Get operations
vgr := &volrep.VolumeGroupReplication{}
err := v.reconciler.Get(v.ctx, namespacedName, vgr)

// Should become:
vgrObj := v.replicationFactory.GetVolumeGroupReplicationType()
err := v.reconciler.Get(v.ctx, namespacedName, vgrObj)
vgr := v.replicationFactory.WrapVolumeGroupReplication(vgrObj)

// Pattern 2: Create operations
volRep := &volrep.VolumeGroupReplication{
    ObjectMeta: metav1.ObjectMeta{...},
    Spec: volrep.VolumeGroupReplicationSpec{...},
}
err := v.reconciler.Create(v.ctx, volRep)

// Should become:
vgr := v.replicationFactory.NewVolumeGroupReplication(name, namespace)
vgr.SetSpec(spec) // Using interface methods
err := v.reconciler.Create(v.ctx, vgr)

// Pattern 3: List operations
volGroupRepList := &volrep.VolumeGroupReplicationList{}
err := k8sClient.List(context.TODO(), volGroupRepList, listOptions)

// Requires new factory method for list types
```

##### File: `internal/controller/drpolicy_peerclass.go` (20+ occurrences)

**Struct fields:**
```go
type classLists struct {
    vgrClasses []*volrep.VolumeGroupReplicationClass  // Line 28
}

// Should become:
type classLists struct {
    vgrClasses []replication.VolumeGroupReplicationClassInterface
}
```

**Methods:**
```go
func getVGRClassesFromManagedCluster(...) ([]*volrep.VolumeGroupReplicationClass, error)

// Should become:
func getVGRClassesFromManagedCluster(...) ([]replication.VolumeGroupReplicationClassInterface, error)
```

##### File: `internal/controller/s3utils.go` (10+ occurrences)

**Function signatures:**
```go
func UploadVGRC(s ObjectStorer, vgrcKeyPrefix, vgrcKeySuffix string,
    vgrc volrep.VolumeGroupReplicationContent) error

func UploadVGR(s ObjectStorer, vgrKeyPrefix, vgrKeySuffix string,
    vgr volrep.VolumeGroupReplication) error

func downloadVGRCs(s ObjectStorer, vgrcKeyPrefix string) (
    vgrcList []volrep.VolumeGroupReplicationContent, err error)

func downloadVGRs(s ObjectStorer, vgrKeyPrefix string) (
    vgrList []volrep.VolumeGroupReplication, err error)

// Should become (using interfaces):
func UploadVGRC(s ObjectStorer, vgrcKeyPrefix, vgrcKeySuffix string,
    vgrc replication.VolumeGroupReplicationContentInterface) error
// ... etc
```

##### File: `internal/controller/util/mcv_util.go` (5+ occurrences)

**Interface and implementation:**
```go
type MCVGetter interface {
    GetVGRClassFromManagedCluster(resourceName, managedCluster string,
        annotations map[string]string) (*volrep.VolumeGroupReplicationClass, error)
}

func (m *ManagedClusterViewGetter) GetVGRClassFromManagedCluster(...) (
    *volrep.VolumeGroupReplicationClass, error)

// Should become:
type MCVGetter interface {
    GetVGRClassFromManagedCluster(resourceName, managedCluster string,
        annotations map[string]string) (replication.VolumeGroupReplicationClassInterface, error)
}
```

#### 2. Runtime CRD Checks (0% → 100%)

**Add to key reconciliation methods:**
```go
func (v *VRGInstance) reconcileVolGroupRepsAsPrimary(...) {
    // Add at start of method
    if !v.replicationFactory.IsUsingVolrep() && !v.replicationFactory.IsUsingNeutral() {
        v.log.Info("No VolumeGroupReplication CRDs available, skipping VGR reconciliation")
        return
    }
    
    // Existing logic...
}

func (v *VRGInstance) reconcileVolGroupRepsAsSecondary(...) {
    // Same check
}
```

**Locations to add checks:**
- `reconcileVolGroupRepsAsPrimary` (line 30)
- `reconcileVolGroupRepsAsSecondary` (line 70)
- `processVGRAsPrimary` (line 623)
- `processVGRAsSecondary` (line 629)

#### 3. Test Suite (0% → 100%)

**Test files to create:**
- `internal/controller/replication/crd_detector_test.go`
- `internal/controller/replication/factory_test.go`
- `internal/controller/replication/wrappers_test.go`
- `internal/controller/vrg_volgrouprep_dynamic_test.go`

**Test scenarios:**
```go
// CRD Detection Tests
func TestCRDDetectorWithVolrepCRDs(t *testing.T) {}
func TestCRDDetectorWithNeutralCRDs(t *testing.T) {}
func TestCRDDetectorWithNoCRDs(t *testing.T) {}
func TestCRDDetectorCaching(t *testing.T) {}

// Factory Tests
func TestFactoryTypeSelection(t *testing.T) {}
func TestFactoryObjectCreation(t *testing.T) {}
func TestFactoryObjectWrapping(t *testing.T) {}

// Wrapper Tests
func TestVolrepWrapperImplementsInterface(t *testing.T) {}
func TestNeutralWrapperImplementsInterface(t *testing.T) {}
func TestWrapperFieldAccess(t *testing.T) {}

// Integration Tests
func TestVRGReconcileWithVolrepCRDs(t *testing.T) {}
func TestVRGReconcileWithNeutralCRDs(t *testing.T) {}
func TestVRGReconcileWithNoCRDs(t *testing.T) {}
```

### Migration Strategy

#### Phase 1: Helper Methods (Low Risk)
Start with methods that don't have many dependencies:
1. `isVGRandVGRCArchivedAlready`
2. `deleteVGRIfUnused`
3. `cleanupVGRForRestore`
4. `cleanupVGRCForRestore`

#### Phase 2: S3 Operations (Medium Risk)
Update S3-related functions:
1. `UploadVGRC` and `UploadVGR` in s3utils.go
2. `downloadVGRCs` and `downloadVGRs` in s3utils.go
3. `UploadVGRandVGRCtoS3Store`
4. `UploadVGRandVGRCtoS3Stores`

#### Phase 3: Core Reconciliation (High Risk)
Update main reconciliation methods:
1. `createVGR`
2. `updateVGR`
3. `deleteVGR`
4. `reconcileVGRAsSecondary`
5. `processVGRAsPrimary`
6. `processVGRAsSecondary`

#### Phase 4: Peer Class Operations (Medium Risk)
Update drpolicy_peerclass.go:
1. `classLists` struct
2. `getVGRClassesFromManagedCluster`
3. Related helper methods

#### Phase 5: MCV Operations (Low Risk)
Update util/mcv_util.go:
1. `MCVGetter` interface
2. `GetVGRClassFromManagedCluster` implementation

### Estimated Effort

| Phase | Files | Methods | Effort | Risk |
|-------|-------|---------|--------|------|
| Phase 1 | 1 | 4 | 1 hour | Low |
| Phase 2 | 2 | 6 | 1.5 hours | Medium |
| Phase 3 | 1 | 6 | 2 hours | High |
| Phase 4 | 1 | 5 | 1 hour | Medium |
| Phase 5 | 1 | 2 | 0.5 hours | Low |
| **Total** | **6** | **23** | **6 hours** | **Mixed** |

### Testing Strategy

After each phase:
1. Run unit tests
2. Run integration tests
3. Manual testing with both CRD types
4. Commit changes

### Success Criteria

- [ ] All 84+ type references migrated
- [ ] All methods use interfaces instead of concrete types
- [ ] Runtime checks added to key methods
- [ ] Comprehensive test suite created
- [ ] No compilation errors
- [ ] All existing tests pass
- [ ] New tests for both CRD types pass
- [ ] Manual testing confirms both implementations work

### Current Blockers

None - Foundation is complete and ready for migration.

### Next Immediate Steps

1. Start with Phase 1 (Helper Methods)
2. Update `isVGRandVGRCArchivedAlready` method signature
3. Update callers of that method
4. Test and commit
5. Repeat for other Phase 1 methods
6. Move to Phase 2

### Notes

- The factory is already integrated in VRGInstance
- All infrastructure is in place
- Remaining work is systematic and well-defined
- Each phase can be committed independently
- Risk is managed by starting with low-risk changes
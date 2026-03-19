# vrg_volgrouprep.go Refactoring Guide

## Strategy
Replace all concrete `volrep.*` types with factory interface types throughout the file.

## Key Pattern Changes

### Pattern 1: Function Parameters
**Before:**
```go
func (v *VRGInstance) someFunc(vgr *volrep.VolumeGroupReplication) error
```

**After:**
```go
func (v *VRGInstance) someFunc(vgr replication.VolumeGroupReplicationInterface) error
```

### Pattern 2: Creating VGR Objects
**Before:**
```go
vgr := &volrep.VolumeGroupReplication{}
err := v.reconciler.Get(v.ctx, namespacedName, vgr)
```

**After:**
```go
vgrObj := v.replicationFactory.GetVolumeGroupReplicationType()
err := v.reconciler.Get(v.ctx, namespacedName, vgrObj)
vgr := v.replicationFactory.WrapVolumeGroupReplication(vgrObj)
```

### Pattern 3: Creating New VGR
**Before:**
```go
volRep := &volrep.VolumeGroupReplication{
    ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
    Spec: volrep.VolumeGroupReplicationSpec{
        ReplicationState: state,
        VolumeGroupReplicationClassName: className,
        External: offloaded,
        Source: volrep.VolumeGroupReplicationSource{Selector: selector},
    },
}
```

**After:**
```go
volRep := v.replicationFactory.NewVolumeGroupReplication(name, namespace)
spec := volRep.GetSpec()
spec.SetReplicationState(replication.ReplicationState(state))
spec.SetVolumeGroupReplicationClassName(className)
spec.SetExternal(offloaded)
source := spec.GetSource()
source.SetSelector(selector)
```

### Pattern 4: Accessing Fields
**Before:**
```go
state := vgr.Spec.ReplicationState
className := vgr.Spec.VolumeGroupReplicationClassName
pvcList := vgr.Status.PersistentVolumeClaimsRefList
```

**After:**
```go
state := vgr.GetSpec().GetReplicationState()
className := vgr.GetSpec().GetVolumeGroupReplicationClassName()
pvcList := vgr.GetStatus().GetPersistentVolumeClaimsRefList()
```

### Pattern 5: Type Assertions (Remove)
**Before:**
```go
vgr, ok := obj.(*volrep.VolumeGroupReplication)
if !ok {
    return nil, fmt.Errorf("not a VGR")
}
```

**After:**
```go
vgr := v.replicationFactory.WrapVolumeGroupReplication(obj)
if vgr == nil {
    return nil, fmt.Errorf("not a VGR")
}
```

## Functions to Update (in order of dependency)

1. `getVGRCFromVGR()` - Returns VGRC, needs interface
2. `getVGRUsingSCLabel()` - Returns VGR, needs interface  
3. `isPVCInVGR()` - Takes VGR param, needs interface
4. `deleteVGRIfUnused()` - Takes VGR param, needs interface
5. `isVGRandVGRCArchivedAlready()` - Takes VGR param, needs interface
6. `uploadVGRandVGRCtoS3Stores()` - Takes VGR param, needs interface
7. `UploadVGRandVGRCtoS3Store()` - Takes VGR param, needs interface
8. `UploadVGRAndVGRCtoS3()` - Takes VGR/VGRC params, needs interface
9. `UploadVGRandVGRCtoS3Stores()` - Takes VGR param, needs interface
10. `addArchivedAnnotationForVGRandVGRC()` - Takes VGR param, needs interface
11. `processVGRAsPrimary()` - Creates/gets VGR, needs interface
12. `reconcileVGRAsSecondary()` - Gets VGR, needs interface
13. `updateVGR()` - Takes VGR param, needs interface
14. `createVGR()` - Creates VGR, needs interface
15. `deleteVGR()` - Deletes VGR, needs interface
16. `checkVGRCClusterData()` - Takes VGRC list, needs interface
17. `validateExistingVGRC()` - Takes VGRC param, needs interface
18. `validateExistingVGR()` - Takes VGR param, needs interface
19. `cleanupVGRCForRestore()` - Takes VGRC param, needs interface
20. `cleanupVGRForRestore()` - Takes VGR param, needs interface
21. `processVGRCSecrets()` - Takes VGRC param, needs interface

## Implementation Notes

- S3 upload/download functions will need special handling (see s3utils.go refactor)
- Type conversions for ReplicationState enum
- Careful with nil checks when wrapping objects
- Test thoroughly after each function update
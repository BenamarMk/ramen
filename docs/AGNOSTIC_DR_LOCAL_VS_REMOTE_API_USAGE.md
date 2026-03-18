# Agnostic DR: Local vs Remote API Usage

## Overview

This document explains why `grpReplClassList` in `VRGInstance` uses the legacy `volrep.VolumeGroupReplicationClassList` type and how this fits into the overall Agnostic DR architecture.

**Date**: 2026-03-18  
**Status**: Design Decision Documented  
**Related**: Agnostic DR Phase 1 Implementation

---

## The Question

Why does `grpReplClassList` still use the legacy type when we've implemented dual API support?

```go
grpReplClassList: &volrep.VolumeGroupReplicationClassList{}, // Legacy type
```

---

## The Answer: Local vs Remote Operations

### Two Different Contexts

The Agnostic DR implementation distinguishes between two operational contexts:

#### 1. **Local Cluster Operations** (VRGInstance)
- **Location**: Hub or spoke cluster where VRG controller runs
- **Purpose**: List VGRClasses available on the LOCAL cluster
- **API Control**: Controlled by operator installation on that cluster
- **Current Implementation**: Uses legacy type directly

#### 2. **Remote Cluster Operations** (ManagedClusterView)
- **Location**: Cross-cluster discovery via OCM
- **Purpose**: Discover VGRClasses on REMOTE managed clusters
- **API Control**: Unknown - could be neutral, legacy, or both
- **Current Implementation**: Dual API support with automatic fallback

---

## Why This Design Makes Sense

### Local Operations (VRGInstance)

**Current Approach**: Use legacy type directly
```go
// In VRGInstance initialization
grpReplClassList: &volrep.VolumeGroupReplicationClassList{}

// In listing operation
v.reconciler.List(v.ctx, v.grpReplClassList, listOptions...)
```

**Rationale**:
1. **Controlled Environment**: The local cluster's API is controlled by the operator installation
2. **Simplicity**: Direct type usage is simpler and more efficient
3. **No Discovery Needed**: We know which API is installed locally
4. **Client.List Requirement**: Kubernetes client.List() requires concrete types implementing `client.ObjectList`

### Remote Operations (ManagedClusterView)

**Current Approach**: Dual API support with automatic fallback
```go
// In GetVGRClassFromManagedCluster
// 1. Try neutral API first
err := m.getResourceFromManagedCluster(..., "replication.storage.io", ...)
if err == nil {
    return vgrc, nil
}

// 2. Fall back to legacy API
if k8serrors.IsNotFound(err) || isNoMatchError(err) {
    legacyErr := m.getResourceFromManagedCluster(..., volrep.GroupVersion.Group, ...)
    return legacyVgrc, legacyErr
}
```

**Rationale**:
1. **Unknown Environment**: Remote clusters may have different APIs installed
2. **Flexibility Required**: Must support both neutral and legacy APIs
3. **Smooth Transition**: Enables gradual migration across clusters
4. **MCV Abstraction**: ManagedClusterView provides API abstraction layer

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         Hub Cluster                              │
│                                                                  │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ VRG Controller (VRGInstance)                           │    │
│  │                                                         │    │
│  │  grpReplClassList: *volrep.VolumeGroupReplicationClassList│
│  │  ↓                                                      │    │
│  │  reconciler.List(ctx, grpReplClassList, ...)          │    │
│  │  ↓                                                      │    │
│  │  Lists LOCAL VGRClasses (legacy API)                  │    │
│  └────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ DRPolicy Controller                                     │    │
│  │                                                         │    │
│  │  GetVGRClassFromManagedCluster(name, cluster, ...)    │    │
│  │  ↓                                                      │    │
│  │  Try neutral API → Fall back to legacy API            │    │
│  │  ↓                                                      │    │
│  │  Discovers REMOTE VGRClasses (dual API support)       │    │
│  └────────────────────────────────────────────────────────┘    │
│                                                                  │
└──────────────────────────┬───────────────────────────────────────┘
                           │ ManagedClusterView
                           │ (API abstraction)
                           ↓
┌─────────────────────────────────────────────────────────────────┐
│                    Managed Cluster (Spoke)                       │
│                                                                  │
│  May have:                                                       │
│  - Neutral API (replication.storage.io) ✓                      │
│  - Legacy API (replication.storage.openshift.io) ✓            │
│  - Both APIs ✓                                                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Current State Summary

### ✅ What Works Now

| Operation | API Support | Status |
|-----------|-------------|--------|
| Local VGRClass listing (VRGInstance) | Legacy only | ✅ Working |
| Remote VGRClass discovery (MCV) | Neutral + Legacy | ✅ Implemented |
| Cross-cluster PeerClass matching | Neutral + Legacy | ✅ Implemented |
| VGR operations via handler | Neutral + Legacy | ✅ Available |

### 📋 Future Enhancements (Phase 2)

| Enhancement | Priority | Complexity |
|-------------|----------|------------|
| Use replicationHandler.DiscoverVGRClasses() for local listing | Medium | Medium |
| Support unstructured types in VRGInstance | Low | High |
| Full API-agnostic VGRClass handling | Low | High |

---

## Why Not Change Local Operations Now?

### Technical Challenges

1. **Client.List Requirement**
   ```go
   // Kubernetes client requires concrete types
   func (c *Client) List(ctx context.Context, list client.ObjectList, opts ...ListOption) error
   ```
   - `client.ObjectList` interface requires specific methods
   - Legacy type already implements this interface
   - Changing would require unstructured types or interface wrappers

2. **Code Complexity**
   - Current code is simple and direct
   - Changing would add abstraction layers
   - Benefit is minimal for local operations

3. **No Immediate Need**
   - Local cluster API is controlled by installation
   - No cross-cluster compatibility issues
   - Works reliably with legacy type

### Benefits of Current Approach

1. **Simplicity**: Direct type usage is clear and maintainable
2. **Performance**: No additional abstraction overhead
3. **Reliability**: Well-tested legacy code path
4. **Focus**: Allows focus on critical cross-cluster discovery

---

## Migration Path (Phase 2)

When we're ready to make local operations fully API-agnostic:

### Option 1: Use ReplicationHandler (Recommended)

```go
// Instead of direct List
v.reconciler.List(v.ctx, v.grpReplClassList, listOptions...)

// Use handler's discovery method
vgrClasses, err := v.replicationHandler.DiscoverVGRClasses(
    v.ctx,
    v.reconciler.Client,
    storageClassName,
    storageID,
    schedule,
)
```

**Pros**:
- Uses existing handler infrastructure
- API-agnostic
- Consistent with remote operations

**Cons**:
- Requires refactoring existing code
- Different data structure ([]VGRClassInfo vs *VolumeGroupReplicationClassList)

### Option 2: Unstructured Types

```go
// Use unstructured for API-agnostic listing
vgrClassList := &unstructured.UnstructuredList{}
vgrClassList.SetGroupVersionKind(schema.GroupVersionKind{
    Group:   apiGroup, // Determined by discovery
    Version: "v1alpha1",
    Kind:    "VolumeGroupReplicationClassList",
})
v.reconciler.List(v.ctx, vgrClassList, listOptions...)
```

**Pros**:
- Fully API-agnostic
- Works with any API group

**Cons**:
- More complex code
- Type safety lost
- Requires manual field extraction

### Option 3: Hybrid Approach

```go
// Try neutral API first, fall back to legacy
neutralList := &unstructured.UnstructuredList{}
// ... set GVK for neutral API
err := v.reconciler.List(v.ctx, neutralList, listOptions...)
if err != nil && isNoMatchError(err) {
    // Fall back to legacy
    legacyList := &volrep.VolumeGroupReplicationClassList{}
    err = v.reconciler.List(v.ctx, legacyList, listOptions...)
}
```

**Pros**:
- Supports both APIs
- Gradual migration path

**Cons**:
- Most complex
- Duplicate code paths

---

## Recommendation

### For Now (Phase 1) ✅
**Keep using legacy type for local operations**

Reasons:
- Simple and reliable
- No immediate benefit to change
- Focus on critical cross-cluster discovery (already implemented)
- Allows time to evaluate best migration approach

### For Phase 2 📋
**Migrate to ReplicationHandler.DiscoverVGRClasses()**

Reasons:
- Consistent with handler architecture
- API-agnostic
- Cleaner abstraction
- Better long-term maintainability

---

## Code Comments Added

### In VRGInstance initialization:
```go
// Note: grpReplClassList uses legacy type for local cluster operations.
// This is acceptable because:
// 1. Local cluster API is controlled by the operator installation
// 2. Cross-cluster discovery (via MCV) already supports both APIs
// 3. The replicationHandler provides API abstraction for VGR operations
// TODO (Phase 2): Migrate to replicationHandler.DiscoverVGRClasses()
grpReplClassList: &volrep.VolumeGroupReplicationClassList{},
```

### In VRGInstance struct:
```go
// grpReplClassList uses legacy type for LOCAL cluster VGRClass listing.
// This is acceptable because:
// 1. Local cluster API is controlled by the operator installation
// 2. Cross-cluster discovery (via MCV) already supports both APIs
// 3. The replicationHandler provides API abstraction for VGR operations
// TODO (Phase 2): Migrate to replicationHandler.DiscoverVGRClasses()
grpReplClassList *volrep.VolumeGroupReplicationClassList
```

---

## Conclusion

The use of legacy type for `grpReplClassList` is a **deliberate design decision**, not an oversight. It reflects the distinction between:

1. **Local operations**: Controlled environment, use direct types
2. **Remote operations**: Unknown environment, use dual API support

This approach:
- ✅ Maintains simplicity where it matters
- ✅ Provides flexibility where it's needed
- ✅ Enables smooth transition to neutral API
- ✅ Focuses effort on critical cross-cluster discovery

The cross-cluster discovery (via `GetVGRClassFromManagedCluster`) **already supports both APIs**, which is the critical requirement for the Agnostic DR initiative.

---

## References

- Implementation: `internal/controller/volumereplicationgroup_controller.go`
- Cross-cluster discovery: `internal/controller/util/mcv_util.go`
- Handler infrastructure: `internal/controller/replication/`
- Design document: `docs/Agnostic-dr-changes-design.docx`

---

**Status**: ✅ Design Decision Documented  
**Last Updated**: 2026-03-18  
**Next Review**: Phase 2 Planning
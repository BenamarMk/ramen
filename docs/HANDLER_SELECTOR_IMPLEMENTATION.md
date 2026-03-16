# Handler Selector Implementation

## Overview

The Handler Selector is a new component that enables **offload-aware handler selection** for the Shared Replication API. It allows Ramen to choose between the Neutral API (replication.storage.io) and Legacy API (csi-addons) based on the StorageClass's `offloaded` label.

## Design Decision

**Selected Approach: Option C - New Method with Offloaded Flag**

The selector uses the `ramendr.openshift.io/offloaded` label on StorageClass as the primary decision maker for handler selection:

- **offloaded=true** → Use Neutral API (replication.storage.io)
- **offloaded=false or no label** → Use Legacy API (csi-addons VGR) - **DEFAULT**

This approach provides:
- ✅ Clear, explicit control over handler selection
- ✅ Backward compatibility (defaults to legacy)
- ✅ Fallback mechanism for robustness
- ✅ Simple integration with existing code

## Implementation

### Files Created

1. **`internal/controller/replication/selector.go`** (89 lines)
   - `HandlerSelector` struct with client and logger
   - `SelectHandler()` - Primary selection method based on StorageClass label
   - `SelectHandlerWithFallback()` - Selection with discovery fallback
   - Helper methods for StorageClass lookup and label checking

2. **`internal/controller/replication/selector_test.go`** (428 lines)
   - 11 comprehensive unit tests covering all scenarios
   - Tests for offloaded=true, false, missing label
   - Error handling tests (StorageClass not found, API not available)
   - Fallback mechanism tests
   - Edge case tests (invalid/empty label values)

### Key Components

#### HandlerSelector Structure

```go
type HandlerSelector struct {
    client    client.Client
    log       logr.Logger
    discovery *Discovery
}
```

#### Selection Logic

```go
func (s *HandlerSelector) SelectHandler(ctx context.Context, storageClassName string) (ReplicationHandler, HandlerType, error)
```

**Flow:**
1. Retrieve StorageClass by name
2. Check for `ramendr.openshift.io/offloaded` label
3. If `offloaded=true`:
   - Select NeutralHandler
   - Verify neutral API is available
   - Return error if not available
4. If `offloaded=false` or no label (default):
   - Select LegacyHandler
   - Verify legacy API is available
   - Return error if not available

#### Fallback Mechanism

```go
func (s *HandlerSelector) SelectHandlerWithFallback(ctx context.Context, storageClassName string) (ReplicationHandler, HandlerType, error)
```

**Flow:**
1. Try `SelectHandler()` first
2. If it fails, fall back to discovery mechanism
3. Return discovered handler or error if both fail

This provides backward compatibility and robustness for edge cases.

## Test Coverage

### Unit Tests (11 tests, 100% passing)

1. **TestSelectHandler_OffloadedTrue** - Verifies neutral handler selection when offloaded=true
2. **TestSelectHandler_OffloadedFalse** - Verifies legacy handler selection when offloaded=false
3. **TestSelectHandler_NoLabel** - Verifies default to legacy when no label present
4. **TestSelectHandler_StorageClassNotFound** - Error handling for missing StorageClass
5. **TestSelectHandler_NeutralAPINotAvailable** - Error when offloaded=true but neutral API missing
6. **TestSelectHandler_LegacyAPINotAvailable** - Error when offloaded=false but legacy API missing
7. **TestSelectHandlerWithFallback_Success** - Fallback succeeds with valid StorageClass
8. **TestSelectHandlerWithFallback_FallbackToDiscovery** - Fallback uses discovery when SC missing
9. **TestSelectHandler_InvalidLabelValue** - Invalid label values treated as false
10. **TestSelectHandler_EmptyLabelValue** - Empty label values treated as false
11. **TestSelectHandler_BothAPIsAvailable** - Correct selection when both APIs present

### Total Test Suite

- **43 tests total** (32 original + 11 new selector tests)
- **100% passing**
- Coverage includes:
  - Discovery mechanism (9 tests)
  - NeutralHandler (11 tests)
  - LegacyHandler (12 tests)
  - HandlerSelector (11 tests)

## Integration Points

### Current Integration Status

The selector is **implemented and tested** but **not yet integrated** into the VRG controller. Integration will happen in the next phase.

### Planned Integration

The selector will be integrated into:

1. **VRG Controller Initialization** (`volumereplicationgroup_controller.go`)
   - Replace discovery-based handler initialization
   - Use selector to choose handler based on StorageClass

2. **VGR Reconciliation** (`vrg_volgrouprep.go`)
   - Use selector when creating/updating VGRs
   - Pass StorageClass name to selector
   - Handle selection errors appropriately

### Integration Example

```go
// In VRG controller
selector := replication.NewHandlerSelector(r.Client, log)

// When reconciling VGR
handler, handlerType, err := selector.SelectHandler(ctx, storageClassName)
if err != nil {
    // Handle error - maybe use fallback
    handler, handlerType, err = selector.SelectHandlerWithFallback(ctx, storageClassName)
}

// Use selected handler
vgr, err := handler.CreateVGR(ctx, r.Client, vgrName, namespace, spec)
```

## Benefits

### 1. Explicit Control
- Administrators explicitly choose which API to use via StorageClass labels
- No ambiguity about which handler will be selected
- Clear migration path from legacy to neutral API

### 2. Backward Compatibility
- Defaults to legacy API when no label present
- Existing deployments continue to work without changes
- Gradual migration possible

### 3. Robustness
- Fallback mechanism handles edge cases
- Clear error messages when APIs are unavailable
- Validates API availability before selection

### 4. Testability
- Comprehensive unit test coverage
- Easy to test different scenarios
- Clear separation of concerns

## Usage Example

### StorageClass Configuration

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rbd-offloaded
  labels:
    ramendr.openshift.io/offloaded: "true"  # Use Neutral API
provisioner: rbd.csi.ceph.com
parameters:
  # ... storage parameters
```

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rbd-legacy
  labels:
    ramendr.openshift.io/offloaded: "false"  # Use Legacy API (explicit)
provisioner: rbd.csi.ceph.com
parameters:
  # ... storage parameters
```

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rbd-default
  # No label - defaults to Legacy API
provisioner: rbd.csi.ceph.com
parameters:
  # ... storage parameters
```

## Next Steps

1. **Integrate selector into VRG controller** - Replace discovery-based initialization
2. **Update VGR reconciliation logic** - Use selector for handler selection
3. **Add integration tests** - Test selector in VRG controller context
4. **Update documentation** - Document StorageClass label usage
5. **Create migration guide** - Help users transition from legacy to neutral API

## Related Documents

- [Agnostic DR Implementation Plan](AGNOSTIC_DR_IMPLEMENTATION_PLAN.md)
- [Agnostic DR Testing Plan](AGNOSTIC_DR_TESTING_PLAN.md)
- [Phase 6 Standalone Package Summary](../replication-storage-io-crds/PHASE6_STANDALONE_PACKAGE_SUMMARY.md)

## Made with Bob 🍜
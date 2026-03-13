# Replication Abstraction Layer

This package provides an abstraction layer for volume group replication operations, enabling Ramen to work with both legacy and neutral replication APIs seamlessly.

## Overview

The replication package implements the **Bridge Pattern** to support:
- **Legacy API**: `replication.storage.openshift.io` (csi-addons/ODF)
- **Neutral API**: `replication.storage.io` (vendor-agnostic standard)

This allows for a smooth transition from vendor-specific to vendor-neutral replication without breaking existing deployments.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    VRG Controller                        │
│                  (Ramen Orchestrator)                    │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              ReplicationHandler Interface                │
│  (Abstraction: CreateVGR, GetVGR, UpdateVGR, etc.)     │
└────────────┬───────────────────────────┬────────────────┘
             │                           │
             ▼                           ▼
┌────────────────────────┐  ┌──────────────────────────┐
│    LegacyHandler       │  │    NeutralHandler        │
│ (csi-addons wrapper)   │  │ (replication.storage.io) │
└────────────────────────┘  └──────────────────────────┘
             │                           │
             ▼                           ▼
┌────────────────────────┐  ┌──────────────────────────┐
│ replication.storage.   │  │ replication.storage.io   │
│ openshift.io (Legacy)  │  │ (Neutral/Standard)       │
└────────────────────────┘  └──────────────────────────┘
```

## Components

### 1. ReplicationHandler Interface (`interface.go`)

Defines the contract for all replication operations:

```go
type ReplicationHandler interface {
    GetAPIGroup() string
    GetAPIVersion() string
    IsAvailable(ctx context.Context, client client.Client) (bool, error)
    DiscoverVGRClasses(...) ([]VGRClassInfo, error)
    CreateVGR(...) error
    GetVGR(...) (*VGRStatus, error)
    UpdateVGR(...) error
    DeleteVGR(...) error
    IsVGRReady(...) (bool, error)
    GetVGRConditions(...) ([]metav1.Condition, error)
}
```

### 2. LegacyHandler (`legacy_handler.go`)

Wraps the existing `replication.storage.openshift.io` API (csi-addons):
- Maintains backward compatibility with ODF/Ceph
- Translates interface calls to legacy API calls
- Handles legacy-specific quirks and limitations

### 3. NeutralHandler (`neutral_handler.go`)

Implements the new `replication.storage.io` API:
- Vendor-agnostic implementation
- Follows Kubernetes API conventions
- Enables multi-vendor support

### 4. Discovery (`discovery.go`)

Automatically detects which API is available:
- **Priority**: Neutral API > Legacy API
- Enables smooth transition during migration
- Per-PeerClass API selection support

## Usage

### Basic Usage

```go
import "github.com/ramendr/ramen/internal/controller/replication"

// Create discovery
discovery := replication.NewDiscovery(client, log)

// Discover available handler
handler, handlerType, err := discovery.DiscoverHandler(ctx)
if err != nil {
    return err
}

log.Info("Using replication handler", "type", handlerType, "apiGroup", handler.GetAPIGroup())

// Use handler for operations
err = handler.CreateVGR(ctx, client, namespacedName, spec)
```

### Explicit Handler Selection

```go
// Create specific handler
factory := replication.NewHandlerFactory()

// Use neutral handler explicitly
neutralHandler := factory.CreateNeutralHandler()

// Use legacy handler explicitly
legacyHandler := factory.CreateLegacyHandler()
```

### Check API Availability

```go
discovery := replication.NewDiscovery(client, log)

// Check which APIs are available
apis, err := discovery.GetAvailableAPIs(ctx)
if err != nil {
    return err
}

if apis["replication.storage.io"] {
    log.Info("Neutral API is available")
}

if apis["replication.storage.openshift.io"] {
    log.Info("Legacy API is available")
}
```

## Migration Strategy

### Phase 1: Coexistence (Current)
- Both APIs can coexist in the same cluster
- Discovery automatically selects the best available API
- Legacy deployments continue working unchanged

### Phase 2: Transition
- New deployments use neutral API
- Existing deployments gradually migrate
- Both handlers remain available

### Phase 3: Deprecation
- Legacy API marked as deprecated
- Migration tools provided
- Neutral API becomes default

### Phase 4: Removal
- Legacy handler removed
- Only neutral API supported
- Full vendor independence achieved

## Handler Selection Logic

```
1. Check if neutral API (replication.storage.io) is available
   ├─ YES → Use NeutralHandler
   └─ NO  → Continue to step 2

2. Check if legacy API (replication.storage.openshift.io) is available
   ├─ YES → Use LegacyHandler
   └─ NO  → Return error (no replication API available)
```

## Testing

### Unit Tests

```go
// Test handler interface compliance
var _ replication.ReplicationHandler = (*replication.LegacyHandler)(nil)
var _ replication.ReplicationHandler = (*replication.NeutralHandler)(nil)

// Test discovery
discovery := replication.NewDiscovery(fakeClient, log)
handler, handlerType, err := discovery.DiscoverHandler(ctx)
```

### Integration Tests

```go
// Test with real cluster
handler, _, err := discovery.DiscoverHandler(ctx)
require.NoError(t, err)

// Create VGR
err = handler.CreateVGR(ctx, client, namespacedName, spec)
require.NoError(t, err)

// Verify VGR status
status, err := handler.GetVGR(ctx, client, namespacedName)
require.NoError(t, err)
assert.True(t, status.Ready)
```

## API Compatibility

### Common Fields (Both APIs)

| Field | Legacy | Neutral | Notes |
|-------|--------|---------|-------|
| ReplicationState | ✅ | ✅ | primary, secondary, resync |
| VGRClassName | ✅ | ✅ | Reference to VGRC |
| Source.Selector | ✅ | ✅ | PVC label selector |
| AutoResync | ✅ | ✅ | Auto-resync flag |
| Status.State | ✅ | ✅ | Current state |
| Status.Conditions | ✅ | ✅ | Condition list |

### Differences

| Feature | Legacy | Neutral | Handler Behavior |
|---------|--------|---------|------------------|
| ReplicationHandle | ❌ | ✅ | Ignored in legacy |
| External flag | ❌ | ✅ | Not supported in legacy |
| API Group | openshift.io | storage.io | Detected automatically |

## Error Handling

```go
handler, handlerType, err := discovery.DiscoverHandler(ctx)
if err != nil {
    if errors.Is(err, replication.ErrNoAPIAvailable) {
        // No replication API installed
        return fmt.Errorf("replication API not available: %w", err)
    }
    return err
}

// Use handler...
```

## Best Practices

1. **Always use Discovery**: Let the system auto-detect the best API
2. **Handle both APIs**: Don't assume which API is available
3. **Log handler type**: Help with debugging and migration tracking
4. **Test both paths**: Ensure code works with both handlers
5. **Graceful degradation**: Handle missing features in legacy API

## Future Enhancements

- [ ] Add metrics for handler usage
- [ ] Implement handler caching
- [ ] Add handler health checks
- [ ] Support custom handler plugins
- [ ] Add handler performance profiling

## References

- [Design Document](../../../docs/Agnostic-dr-changes-design.docx)
- [Neutral API](../../../api/replication.storage.io/README.md)
- [csi-addons (Legacy)](https://github.com/csi-addons/kubernetes-csi-addons)
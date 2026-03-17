# Making Neutral API Runtime-Optional

## Your Vision (Correct!)

You want this deployment flow:

### Step 1: Build Ramen (No Neutral API Dependency)
```bash
make docker-build IMG=<image>
# Should build WITHOUT needing replication-storage-io-crds
# Ramen binary has NO compile-time dependency on neutral API
```

### Step 2: Deploy Ramen (Legacy Only)
```bash
make deploy IMG=<image>
# Ramen runs with ONLY legacy API support
# Works perfectly with existing ODF/Ceph deployments
```

### Step 3: Install Neutral API CRDs (Optional)
```bash
cd replication-storage-io-crds
make install
# Installs neutral API CRDs on cluster
# Ramen automatically detects them at runtime
# Now supports BOTH legacy and neutral APIs
```

## Current Problem

The current implementation has a **compile-time dependency** on neutral API types:

```go
// internal/controller/replication/neutral_handler.go
import (
    replicationv1alpha1 "github.com/ramendr/replication-storage-io-crds/api/v1alpha1"
)

func (h *NeutralHandler) CreateVGR(...) error {
    vgr := &replicationv1alpha1.VolumeGroupReplication{  // Compile-time type
        // ...
    }
    return client.Create(ctx, vgr)
}
```

This means:
- ❌ Can't build Ramen without neutral API types
- ❌ Can't deploy Ramen without neutral API CRDs
- ❌ Neutral API is required, not optional

## Solution: Use Dynamic Client (Unstructured)

### Approach: Runtime Type Discovery

Instead of importing typed structs, use Kubernetes `unstructured.Unstructured`:

```go
// internal/controller/replication/neutral_handler.go
import (
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

var neutralVGRGVK = schema.GroupVersionKind{
    Group:   "replication.storage.io",
    Version: "v1alpha1",
    Kind:    "VolumeGroupReplication",
}

func (h *NeutralHandler) CreateVGR(...) error {
    // Build VGR as unstructured (no compile-time dependency)
    vgr := &unstructured.Unstructured{}
    vgr.SetGroupVersionKind(neutralVGRGVK)
    vgr.SetName(name)
    vgr.SetNamespace(namespace)
    
    // Set spec fields dynamically
    spec := map[string]interface{}{
        "replicationState": string(spec.ReplicationState),
        "volumeGroupReplicationClassName": spec.VGRClassName,
        "pvcSelector": spec.PVCSelector,
    }
    vgr.Object["spec"] = spec
    
    // Create using dynamic client (works if CRD exists, fails gracefully if not)
    return client.Create(ctx, vgr)
}
```

### Benefits

✅ **No Compile-Time Dependency**
- Ramen builds without neutral API types
- No import of `replication-storage-io-crds/api`
- Smaller binary, faster builds

✅ **Runtime Optional**
- If neutral CRDs installed → neutral API works
- If neutral CRDs not installed → only legacy API works
- Discovery mechanism detects availability

✅ **Graceful Degradation**
- Discovery returns "not available" if CRDs missing
- Selector falls back to legacy handler
- No errors, just works with what's available

✅ **True Decoupling**
- Ramen and neutral API are independent
- Can version separately
- Can deploy separately

## Implementation Changes Needed

### 1. Remove Typed Imports

**Files to change:**
- `internal/controller/replication/neutral_handler.go`
- `internal/controller/replication/neutral_handler_test.go`
- `internal/controller/replication/discovery.go`
- `internal/controller/replication/discovery_test.go`
- `internal/controller/replication/selector_test.go`
- `internal/controller/suite_test.go`
- `internal/controller/vrg_volgrouprep_integration_test.go`

**Change from:**
```go
import neutralv1alpha1 "github.com/ramendr/replication-storage-io-crds/api/v1alpha1"

vgr := &neutralv1alpha1.VolumeGroupReplication{...}
```

**Change to:**
```go
import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

vgr := &unstructured.Unstructured{}
vgr.SetGroupVersionKind(neutralVGRGVK)
```

### 2. Update go.mod

**Remove:**
```go
replace github.com/ramendr/replication-storage-io-crds/api => ./replication-storage-io-crds/api

require (
    github.com/ramendr/replication-storage-io-crds/api v0.0.0-00010101000000-000000000000
)
```

**Result:**
- No dependency on neutral API package
- Builds without it

### 3. Update Dockerfile

**Remove:**
```dockerfile
COPY replication-storage-io-crds/api/ replication-storage-io-crds/api/
```

**Result:**
- Smaller build context
- Faster builds
- No coupling

### 4. Update Discovery Mechanism

```go
// internal/controller/replication/discovery.go
func (d *Discovery) detectNeutralAPI() bool {
    // Try to list neutral VGR CRD
    gvk := schema.GroupVersionKind{
        Group:   "replication.storage.io",
        Version: "v1alpha1",
        Kind:    "VolumeGroupReplication",
    }
    
    // Check if CRD exists
    _, err := d.client.Resource(gvk).List(ctx, metav1.ListOptions{Limit: 1})
    if err != nil {
        if errors.IsNotFound(err) || meta.IsNoMatchError(err) {
            return false  // CRD not installed
        }
    }
    return true  // CRD exists
}
```

### 5. Update Tests

Tests can still use typed structs for convenience:

```go
// Test files can import neutral types for test setup
import neutralv1alpha1 "github.com/ramendr/replication-storage-io-crds/api/v1alpha1"

// But production code uses unstructured
```

Or use unstructured in tests too for consistency.

## Deployment Flow (After Changes)

### Scenario 1: Legacy Only

```bash
# 1. Build Ramen (no neutral API needed)
make docker-build IMG=quay.io/user/ramen:v1.0

# 2. Deploy Ramen
make deploy IMG=quay.io/user/ramen:v1.0

# 3. Verify legacy API works
kubectl get volumegroupreplication.replication.storage.openshift.io
# Works! ✅

# 4. Try neutral API
kubectl get volumegroupreplication.replication.storage.io
# Error: resource not found (expected, CRDs not installed)

# 5. Check Ramen logs
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator
# "Neutral API not available, using legacy only"
```

### Scenario 2: Both APIs

```bash
# 1-2. Same as above (build and deploy Ramen)

# 3. Install neutral API CRDs
cd replication-storage-io-crds
make install

# 4. Restart Ramen (or wait for discovery refresh)
kubectl rollout restart deployment -n ramen-system ramen-dr-cluster-operator

# 5. Verify both APIs work
kubectl get volumegroupreplication.replication.storage.openshift.io
# Works! ✅

kubectl get volumegroupreplication.replication.storage.io
# Works! ✅

# 6. Check Ramen logs
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator
# "Neutral API available, supporting both APIs"
```

## Benefits of This Approach

### For Development
- ✅ Faster builds (no external dependency)
- ✅ Easier testing (can test legacy-only)
- ✅ Simpler CI/CD (one build, multiple configs)

### For Deployment
- ✅ Backward compatible (works without neutral API)
- ✅ Forward compatible (works with neutral API)
- ✅ Gradual rollout (add neutral API when ready)
- ✅ No breaking changes (existing deployments work)

### For Users
- ✅ Choice (use legacy, neutral, or both)
- ✅ Flexibility (install neutral API when needed)
- ✅ Safety (can test legacy first, add neutral later)

## Comparison

### Current (Compile-Time Dependency)

```
Ramen Build
    ↓ (requires)
Neutral API Types
    ↓ (requires)
Neutral API CRDs
    ↓
Deployment
```

**Problem:** Can't build or deploy without neutral API

### Proposed (Runtime Optional)

```
Ramen Build (standalone)
    ↓
Deployment (legacy only)
    ↓ (optional)
Install Neutral CRDs
    ↓
Ramen detects and uses both
```

**Benefit:** Can build, deploy, and use without neutral API

## Implementation Effort

### Files to Modify
1. `internal/controller/replication/neutral_handler.go` - Use unstructured
2. `internal/controller/replication/discovery.go` - Runtime detection
3. `go.mod` - Remove dependency
4. `Dockerfile` - Remove COPY line
5. Tests - Update to use unstructured or keep typed for convenience

### Estimated Effort
- **Code changes:** 2-3 hours
- **Testing:** 1-2 hours
- **Documentation:** 1 hour
- **Total:** 4-6 hours

### Risk
- **Low:** Changes are isolated to replication package
- **Backward compatible:** Legacy API unchanged
- **Testable:** Can test both scenarios

## Recommendation

**YES, implement this approach!**

Your vision is correct and aligns with best practices:
1. Ramen should build without neutral API
2. Neutral API should be runtime-optional
3. Discovery should detect availability
4. Deployment should work in stages

This is how Kubernetes extensions should work - optional, discoverable, composable.

## Next Steps

1. **Confirm approach** with you
2. **Refactor neutral_handler.go** to use unstructured
3. **Update discovery.go** for runtime detection
4. **Remove go.mod dependency**
5. **Update Dockerfile**
6. **Test both scenarios** (legacy-only, both APIs)
7. **Update documentation**

Would you like me to proceed with this refactoring?
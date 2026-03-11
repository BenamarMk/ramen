# DRPC Test Quick Reference Card

## Import
```go
import "github.com/ramendr/ramen/internal/controller/testutils"
```

## Common Constants
```go
testutils.DRPCCommonName        // "drpc-name"
testutils.DefaultDRPCNamespace  // "drpc-namespace"
testutils.ApplicationNamespace  // "vrg-namespace"
testutils.East1ManagedCluster   // "east1-cluster"
testutils.East2ManagedCluster   // "east2-cluster"
testutils.West1ManagedCluster   // "west1-cluster"
testutils.AsyncDRPolicyName     // "my-async-dr-peers"
testutils.SyncDRPolicyName      // "my-sync-dr-peers"
```

## Quick Fixtures

### Clusters
```go
testutils.GetWest1Cluster()     // Returns west1 cluster
testutils.GetEast1Cluster()     // Returns east1 cluster
testutils.GetEast2Cluster()     // Returns east2 cluster
testutils.GetAsyncClusters()    // Returns [west1, east1]
testutils.GetSyncClusters()     // Returns [east1, east2]
```

### Policies
```go
testutils.GetAsyncDRPolicy()    // Async policy (1h interval)
testutils.GetSyncDRPolicy()     // Sync policy (no interval)
```

### Other
```go
testutils.GetDefaultVRG(ns, s3Profile)  // Default VRG
testutils.GetDefaultApplicationSet()     // Default AppSet
testutils.GetTestNamespaces()            // All test namespaces
testutils.GetTestCIDRs()                 // Test CIDR ranges
```

## Builders

### Cluster
```go
testutils.NewClusterBuilder("name").
    WithLabel("key", "value").
    Build()
```

### DRPolicy
```go
testutils.NewDRPolicyBuilder("name").
    WithClusters("c1", "c2").
    WithSchedulingInterval("5m").
    Build()
```

### DRPC
```go
testutils.NewDRPCBuilder("name", "namespace").
    WithPlacementRef("placement", "Placement").
    WithDRPolicyRef("policy").
    WithPreferredCluster("cluster1").
    WithFailoverCluster("cluster2").
    WithAction(rmn.ActionFailover).
    WithPVCSelector(metav1.LabelSelector{
        MatchLabels: map[string]string{"app": "myapp"},
    }).
    Build()
```

### VRG
```go
testutils.NewVRGBuilder("name", "namespace").
    WithReplicationState(rmn.Primary).
    WithAsync("5m").
    WithPVCSelector(metav1.LabelSelector{
        MatchLabels: map[string]string{"app": "myapp"},
    }).
    WithS3Profiles("profile1").
    Build()
```

### Namespace
```go
testutils.NewNamespaceBuilder("name").Build()
```

### ApplicationSet
```go
testutils.NewApplicationSetBuilder("name", "namespace").
    WithPlacementRef("placement").
    WithDestinationNamespace("dest-ns").
    Build()
```

## Helper Functions

### Wait & Verify
```go
// Wait for phase
testutils.WaitForDRPCPhase(ctx, client, name, ns, rmn.Deployed, timeout)

// Verify phase
testutils.VerifyDRPCPhase(ctx, client, name, ns, rmn.Deployed)

// Verify condition
testutils.VerifyDRPCCondition(ctx, client, name, ns, 
    rmn.ConditionAvailable, metav1.ConditionTrue)
```

### DRPC Operations
```go
// Get latest DRPC
drpc, err := testutils.GetLatestDRPC(ctx, client, name, ns)

// Update spec
testutils.UpdateDRPCSpec(ctx, client, name, ns, 
    "preferred", "failover", rmn.ActionFailover)

// Clear status
testutils.ClearDRPCStatus(ctx, client, name, ns)

// Get condition
idx, cond := testutils.GetDRPCCondition(&drpc.Status, rmn.ConditionAvailable)
```

### Namespace Operations
```go
// Create
testutils.CreateNamespace(ctx, client, ns)

// Delete
testutils.DeleteNamespace(ctx, client, "name")

// Ensure exists
testutils.EnsureNamespaceExists(ctx, client, "name")
```

### Test State
```go
state := testutils.NewTestState()

// PV restore
state.SetRestorePVsComplete()
state.SetRestorePVsIncomplete()
state.IsRestorePVsComplete()

// Cluster state
state.SetClusterDown("cluster")
state.ResetClusterDown()

// UID checks
state.SetToggleUIDChecks()
state.ResetToggleUIDChecks()
```

### Utilities
```go
// Get function name
name := testutils.GetFunctionNameAtIndex(1)

// Build VRG
vrg := testutils.BuildVRG("name", "ns", "cluster", rmn.VRGActionFailover)
```

## Common Patterns

### Basic Test Setup
```go
var _ = Describe("My Test", func() {
    var ctx context.Context
    
    BeforeEach(func() {
        ctx = context.Background()
        
        // Create namespaces
        for _, ns := range testutils.GetTestNamespaces() {
            Expect(testutils.CreateNamespace(ctx, k8sClient, ns)).To(Succeed())
        }
        
        // Create clusters
        for _, cluster := range testutils.GetAsyncClusters() {
            Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
        }
        
        // Create policy
        policy := testutils.GetAsyncDRPolicy()
        Expect(k8sClient.Create(ctx, policy)).To(Succeed())
    })
    
    It("Should work", func() {
        // Test code
    })
})
```

### Table-Driven Test
```go
type testCase struct {
    name     string
    input    string
    expected string
}

testCases := []testCase{
    {name: "case1", input: "a", expected: "b"},
    {name: "case2", input: "c", expected: "d"},
}

for _, tc := range testCases {
    tc := tc // Capture
    It(tc.name, func() {
        // Test with tc.input and tc.expected
    })
}
```

### Deploy → Failover → Relocate
```go
It("Should deploy", func() {
    drpc := testutils.NewDRPCBuilder("drpc", "ns").
        WithPreferredCluster(testutils.East1ManagedCluster).
        WithDRPolicyRef(testutils.AsyncDRPolicyName).
        Build()
    Expect(k8sClient.Create(ctx, drpc)).To(Succeed())
    
    testutils.WaitForDRPCPhase(ctx, k8sClient, "drpc", "ns", 
        rmn.Deployed, time.Minute*2)
})

It("Should failover", func() {
    testutils.UpdateDRPCSpec(ctx, k8sClient, "drpc", "ns",
        testutils.East1ManagedCluster,
        testutils.West1ManagedCluster,
        rmn.ActionFailover)
    
    testutils.WaitForDRPCPhase(ctx, k8sClient, "drpc", "ns",
        rmn.FailedOver, time.Minute*2)
})

It("Should relocate", func() {
    testutils.UpdateDRPCSpec(ctx, k8sClient, "drpc", "ns",
        testutils.East1ManagedCluster,
        "",
        rmn.ActionRelocate)
    
    testutils.WaitForDRPCPhase(ctx, k8sClient, "drpc", "ns",
        rmn.Relocated, time.Minute*2)
})
```

## Tips

### ✅ Do
- Use builders for all test objects
- Use pre-configured fixtures when possible
- Use helper functions for common operations
- Write table-driven tests for similar scenarios
- Keep test files focused (< 500 lines)

### ❌ Don't
- Create structs directly
- Duplicate test code
- Mix test concerns in one file
- Use global variables (use TestState)
- Write tests without using fixtures

## Resources

- **Full Guide:** `DRPC_TEST_REFACTORING.md`
- **Summary:** `REFACTORING_SUMMARY.md`
- **Example:** `drpc_deployment_test_example.go.txt`
- **Fixtures:** `testutils/drpc_fixtures.go`
- **Helpers:** `testutils/drpc_helpers.go`

## Quick Commands

```bash
# Run tests
go test ./internal/controller/...

# Run with coverage
go test -cover ./internal/controller/...

# Run specific test
go test -run TestDRPC ./internal/controller/...

# Verbose output
go test -v ./internal/controller/...
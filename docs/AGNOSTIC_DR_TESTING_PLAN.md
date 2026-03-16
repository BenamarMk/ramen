# Agnostic DR Testing Plan

## Overview

This document outlines the testing strategy for the Shared Replication API implementation. The goal is to validate dual-API support (neutral `replication.storage.io` and legacy `replication.storage.openshift.io`) without breaking existing functionality.

## Testing Principles

1. **Zero Breaking Changes**: All existing ODF/Ceph tests must continue to pass
2. **Dual API Support**: Tests must validate both neutral and legacy APIs
3. **Discovery Validation**: Handler discovery mechanism must be thoroughly tested
4. **Fallback Testing**: Legacy fallback must work when neutral API unavailable
5. **Integration Coverage**: End-to-end scenarios for both API groups

---

## Phase 5.1: Unit Tests for Abstraction Layer

### Test File: `internal/controller/replication/discovery_test.go`

#### Test Cases:

1. **TestDiscoverReplicationHandler_NeutralAvailable**
   - Setup: Install neutral API CRDs
   - Expected: Returns NeutralHandler
   - Validates: Priority selection (neutral > legacy)

2. **TestDiscoverReplicationHandler_LegacyOnly**
   - Setup: Install only legacy API CRDs
   - Expected: Returns LegacyHandler
   - Validates: Fallback mechanism

3. **TestDiscoverReplicationHandler_BothAvailable**
   - Setup: Install both API CRDs
   - Expected: Returns NeutralHandler (priority)
   - Validates: Correct priority ordering

4. **TestDiscoverReplicationHandler_NoneAvailable**
   - Setup: No replication CRDs installed
   - Expected: Returns error
   - Validates: Graceful failure

5. **TestDiscoverReplicationHandler_CacheInvalidation**
   - Setup: Change available APIs during runtime
   - Expected: Discovery refreshes correctly
   - Validates: Dynamic discovery

### Test File: `internal/controller/replication/neutral_handler_test.go`

#### Test Cases:

1. **TestNeutralHandler_CreateVGR**
   - Validates: VGR creation with neutral API
   - Checks: Correct GVK, namespace, labels

2. **TestNeutralHandler_GetVGR**
   - Validates: VGR retrieval by name
   - Checks: Correct object returned

3. **TestNeutralHandler_UpdateVGR**
   - Validates: VGR spec/status updates
   - Checks: Changes persisted correctly

4. **TestNeutralHandler_DeleteVGR**
   - Validates: VGR deletion
   - Checks: Finalizers handled properly

5. **TestNeutralHandler_ListVGR**
   - Validates: VGR listing with label selectors
   - Checks: Correct filtering

### Test File: `internal/controller/replication/legacy_handler_test.go`

#### Test Cases:

1. **TestLegacyHandler_CreateVGR**
   - Validates: VGR creation with legacy API
   - Checks: Correct GVK (replication.storage.openshift.io)

2. **TestLegacyHandler_BackwardCompatibility**
   - Validates: Existing ODF VGR operations
   - Checks: No behavioral changes

3. **TestLegacyHandler_StatusMapping**
   - Validates: Status field mapping
   - Checks: Conditions translated correctly

---

## Phase 5.2: Integration Tests for VRG Controller

### Test File: `internal/controller/volumereplicationgroup_controller_test.go`

#### Test Cases:

1. **TestVRGReconcile_WithNeutralAPI**
   - Setup: Install neutral API CRDs
   - Action: Create VRG resource
   - Expected: VGR created using neutral API
   - Validates: Handler selection and VGR creation

2. **TestVRGReconcile_WithLegacyAPI**
   - Setup: Install only legacy API CRDs
   - Action: Create VRG resource
   - Expected: VGR created using legacy API
   - Validates: Fallback mechanism works

3. **TestVRGReconcile_HandlerSwitching**
   - Setup: Start with legacy, add neutral API
   - Action: Reconcile existing VRG
   - Expected: New VGRs use neutral API
   - Validates: Runtime switching without restart

4. **TestVRGReconcile_MixedEnvironment**
   - Setup: Some clusters with neutral, some with legacy
   - Action: Create VRG across clusters
   - Expected: Correct API used per cluster
   - Validates: Per-cluster handler selection

5. **TestVRGReconcile_StatusAggregation**
   - Setup: VGRs created with both APIs
   - Action: Update VGR statuses
   - Expected: VRG status aggregates correctly
   - Validates: Status translation layer

### Test File: `internal/controller/vrg_volgrouprep_test.go`

#### Test Cases:

1. **TestCreateOrUpdateVR_NeutralAPI**
   - Validates: Wrapper method uses neutral handler
   - Checks: Correct API group in created objects

2. **TestCreateOrUpdateVR_LegacyAPI**
   - Validates: Wrapper method uses legacy handler
   - Checks: Backward compatibility maintained

3. **TestDeleteVR_BothAPIs**
   - Validates: Deletion works with both handlers
   - Checks: Finalizers and cleanup

4. **TestGetVRStatus_Translation**
   - Validates: Status retrieval from both APIs
   - Checks: Consistent status format

---

## Phase 5.3: E2E Test Scenarios

### Test File: `e2e/agnostic_dr_test.go`

#### Test Scenarios:

1. **Scenario: Neutral API Deployment**
   ```
   Given: Fresh cluster with neutral API installed
   When: Deploy application with DR protection
   Then: VGR created using replication.storage.io
   And: Replication works correctly
   ```

2. **Scenario: Legacy API Deployment (ODF)**
   ```
   Given: Cluster with ODF/Ceph installed
   When: Deploy application with DR protection
   Then: VGR created using replication.storage.openshift.io
   And: Existing ODF functionality preserved
   ```

3. **Scenario: Migration from Legacy to Neutral**
   ```
   Given: Application protected with legacy API
   When: Install neutral API and trigger reconciliation
   Then: New VGRs use neutral API
   And: Existing VGRs continue working
   And: No data loss or downtime
   ```

4. **Scenario: Multi-Cluster with Mixed APIs**
   ```
   Given: Hub cluster with Ramen
   And: Cluster A with neutral API
   And: Cluster B with legacy API (ODF)
   When: Deploy application across both clusters
   Then: Cluster A uses neutral API
   And: Cluster B uses legacy API
   And: Failover works between clusters
   ```

5. **Scenario: Vendor Integration**
   ```
   Given: Third-party storage vendor controller
   And: Neutral API installed
   When: Create VGR for vendor storage
   Then: Vendor controller reconciles VGR
   And: Ramen orchestrates DR operations
   And: No OpenShift dependencies required
   ```

---

## Phase 5.4: Test Infrastructure Updates

### Mock Objects Required

1. **Mock Neutral API CRDs**
   - Location: `hack/test/replication.storage.io_*.yaml`
   - Content: VGR, VGRC, VGRContent CRDs

2. **Mock Legacy API CRDs**
   - Location: `hack/test/replication.storage.openshift.io_*.yaml`
   - Content: Existing ODF CRDs

3. **Test Fixtures**
   - Location: `internal/controller/testutils/replication_fixtures.go`
   - Content: Sample VGR objects for both APIs

### Test Environment Setup

```go
// Example test setup helper
func SetupTestEnvironmentWithNeutralAPI(t *testing.T) *envtest.Environment {
    env := &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "config", "crd", "bases"),
            filepath.Join("..", "..", "hack", "test"),
        },
    }
    // Install neutral API CRDs
    // Return configured environment
}

func SetupTestEnvironmentWithLegacyAPI(t *testing.T) *envtest.Environment {
    // Similar but only legacy CRDs
}

func SetupTestEnvironmentWithBothAPIs(t *testing.T) *envtest.Environment {
    // Both neutral and legacy CRDs
}
```

---

## Phase 5.5: Continuous Integration Updates

### CI Pipeline Changes

1. **Add Neutral API Test Job**
   ```yaml
   - name: Test Neutral API
     run: |
       make test-neutral-api
       make test-integration-neutral
   ```

2. **Maintain Legacy API Test Job**
   ```yaml
   - name: Test Legacy API (ODF)
     run: |
       make test-legacy-api
       make test-integration-legacy
   ```

3. **Add Mixed Environment Test**
   ```yaml
   - name: Test Mixed APIs
     run: |
       make test-mixed-environment
   ```

### Test Coverage Goals

- **Unit Tests**: 80% coverage for abstraction layer
- **Integration Tests**: All VRG reconciliation paths
- **E2E Tests**: Critical DR scenarios with both APIs

---

## Phase 5.6: Test Execution Plan

### Week 1: Unit Tests
- Day 1-2: Discovery mechanism tests
- Day 3-4: Handler implementation tests
- Day 5: Test review and fixes

### Week 2: Integration Tests
- Day 1-2: VRG controller tests
- Day 3-4: Wrapper method tests
- Day 5: Integration test review

### Week 3: E2E Tests
- Day 1-2: Neutral API scenarios
- Day 3: Legacy API scenarios
- Day 4: Mixed environment scenarios
- Day 5: E2E test review and documentation

---

## Success Criteria

✅ All existing tests pass without modification
✅ New tests cover both neutral and legacy APIs
✅ Discovery mechanism validated in all scenarios
✅ Handler switching works without restart
✅ No breaking changes to existing deployments
✅ Test coverage meets or exceeds 80%
✅ CI pipeline includes all test scenarios

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Breaking existing ODF tests | Run legacy tests first, fix before proceeding |
| Test environment complexity | Use envtest with CRD mocking |
| Flaky integration tests | Add retry logic and proper cleanup |
| E2E test duration | Parallelize where possible |
| Mock API drift | Sync mocks with actual CRD definitions |

---

## Next Steps After Testing

1. Document test results
2. Create test coverage report
3. Update CI/CD pipeline
4. Proceed to Phase 6: CRD Installation & OLM Integration

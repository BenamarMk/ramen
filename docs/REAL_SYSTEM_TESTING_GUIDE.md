# Real System Testing Guide: Shared Replication API

## Overview

This guide explains how to test the Shared Replication API implementation in a real Kubernetes cluster with actual storage backends.

## Prerequisites

### Required Components

1. **Kubernetes Cluster** (v1.24+)
   - Two clusters for DR testing (primary and secondary)
   - Or single cluster for basic testing

2. **Storage Backend** (Choose one or both)
   - **Legacy Path:** Ceph/ODF with csi-addons
   - **Neutral Path:** Storage array with external replication controller

3. **Ramen Operator**
   - Built from `agnostic-dr` branch
   - Deployed on hub cluster

4. **OCM (Open Cluster Management)**
   - For multi-cluster management
   - Required for DR scenarios

## Testing Scenarios

### Scenario 1: Legacy API (Existing Behavior)
Test that existing ODF/Ceph deployments continue to work without changes.

### Scenario 2: Neutral API (New Capability)
Test storage arrays with external replication using the new neutral API.

### Scenario 3: Mixed Environment
Test both APIs running simultaneously in the same cluster.

---

## Step-by-Step Testing Guide

## Part 1: Build and Deploy Ramen

### Step 1: Build Ramen from agnostic-dr Branch

```bash
# Clone and checkout the branch
git clone https://github.com/ramendr/ramen.git
cd ramen
git checkout agnostic-dr

# Build the operator image
make docker-build IMG=quay.io/<your-org>/ramen:agnostic-dr

# Push to registry
make docker-push IMG=quay.io/<your-org>/ramen:agnostic-dr
```

### Step 2: Install Neutral API CRDs

```bash
# Install the neutral API CRDs first
cd replication-storage-io-crds
make install

# Verify CRDs are installed
kubectl get crd | grep replication.storage.io
# Expected output:
# volumegroupreplicationclasses.replication.storage.io
# volumegroupreplicationcontents.replication.storage.io
# volumegroupreplications.replication.storage.io
```

### Step 3: Deploy Ramen Operator

```bash
# Return to ramen root
cd ..

# Deploy using your custom image
make deploy IMG=quay.io/<your-org>/ramen:agnostic-dr

# Verify deployment
kubectl get pods -n ramen-system
# Expected: ramen-hub-operator and ramen-dr-cluster-operator pods running
```

---

## Part 2: Test Legacy API (Existing ODF/Ceph)

### Step 1: Verify Legacy CRDs Exist

```bash
kubectl get crd | grep replication.storage.openshift.io
# Expected output:
# volumegroupreplicationclasses.replication.storage.openshift.io
# volumegroupreplicationcontents.replication.storage.openshift.io
# volumegroupreplications.replication.storage.openshift.io
```

### Step 2: Create StorageClass (Legacy - No Label)

```yaml
# legacy-storageclass.yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rbd-csi-ceph
  # NO offloaded label - will use legacy API
provisioner: rbd.csi.ceph.com
parameters:
  clusterID: openshift-storage
  pool: replicapool
reclaimPolicy: Delete
volumeBindingMode: Immediate
```

```bash
kubectl apply -f legacy-storageclass.yaml
```

### Step 3: Create VolumeGroupReplicationClass (Legacy)

```yaml
# legacy-vgrc.yaml
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplicationClass
metadata:
  name: vgrc-ceph
  labels:
    ramendr.openshift.io/replicationid: ceph-repl
spec:
  provisioner: rbd.csi.ceph.com
  parameters:
    replication.storage.openshift.io/replication-secret-name: rook-csi-rbd-provisioner
    replication.storage.openshift.io/replication-secret-namespace: openshift-storage
```

```bash
kubectl apply -f legacy-vgrc.yaml
```

### Step 4: Create Test Application with VRG

```yaml
# test-app-legacy.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: test-legacy
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-pvc-1
  namespace: test-legacy
  labels:
    appname: test-app
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: rbd-csi-ceph
---
apiVersion: ramendr.openshift.io/v1alpha1
kind: VolumeReplicationGroup
metadata:
  name: test-vrg-legacy
  namespace: test-legacy
spec:
  pvcSelector:
    matchLabels:
      appname: test-app
  replicationState: primary
  s3Profiles:
    - s3-profile-1
```

```bash
kubectl apply -f test-app-legacy.yaml
```

### Step 5: Verify Legacy API is Used

```bash
# Check VRG status
kubectl get vrg -n test-legacy test-vrg-legacy -o yaml

# Check that VolumeGroupReplication was created using LEGACY API
kubectl get volumegroupreplication.replication.storage.openshift.io -n test-legacy
# Should see VGR created

# Check Ramen operator logs for handler selection
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator | grep "Selected replication handler"
# Expected: "handlerType":"legacy", "apiGroup":"replication.storage.openshift.io"
```

---

## Part 3: Test Neutral API (New Storage Array)

### Step 1: Deploy External Replication Controller

**Note:** This is a mock controller for testing. In production, your storage vendor provides this.

```yaml
# external-replication-controller.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-replication-controller
  namespace: storage-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: external-replication
  template:
    metadata:
      labels:
        app: external-replication
    spec:
      containers:
      - name: controller
        image: quay.io/<your-org>/external-replication-controller:latest
        # This controller watches replication.storage.io VGRs
        # and manages replication on the storage array
```

### Step 2: Create StorageClass with Offloaded Label

```yaml
# neutral-storageclass.yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: pure-flasharray
  labels:
    ramendr.openshift.io/offloaded: "true"  # KEY: This triggers neutral API
provisioner: pure-csi
parameters:
  backend: flasharray-1
  replication.storage.io/replication-mode: async
reclaimPolicy: Delete
volumeBindingMode: Immediate
```

```bash
kubectl apply -f neutral-storageclass.yaml
```

### Step 3: Create VolumeGroupReplicationClass (Neutral)

```yaml
# neutral-vgrc.yaml
apiVersion: replication.storage.io/v1alpha1
kind: VolumeGroupReplicationClass
metadata:
  name: vgrc-pure
  labels:
    ramendr.openshift.io/replicationid: pure-repl
spec:
  provisioner: pure-csi
  parameters:
    replication.storage.io/replication-secret-name: pure-replication-secret
    replication.storage.io/replication-secret-namespace: storage-system
```

```bash
kubectl apply -f neutral-vgrc.yaml
```

### Step 4: Create Test Application with VRG

```yaml
# test-app-neutral.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: test-neutral
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-pvc-1
  namespace: test-neutral
  labels:
    appname: test-app
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: pure-flasharray  # Uses offloaded storage
---
apiVersion: ramendr.openshift.io/v1alpha1
kind: VolumeReplicationGroup
metadata:
  name: test-vrg-neutral
  namespace: test-neutral
spec:
  pvcSelector:
    matchLabels:
      appname: test-app
  replicationState: primary
  s3Profiles:
    - s3-profile-1
```

```bash
kubectl apply -f test-app-neutral.yaml
```

### Step 5: Verify Neutral API is Used

```bash
# Check VRG status
kubectl get vrg -n test-neutral test-vrg-neutral -o yaml

# Check that VolumeGroupReplication was created using NEUTRAL API
kubectl get volumegroupreplication.replication.storage.io -n test-neutral
# Should see VGR created with neutral API

# Check Ramen operator logs for handler selection
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator | grep "Selected replication handler"
# Expected: "handlerType":"neutral", "apiGroup":"replication.storage.io"
```

---

## Part 4: Test Mixed Environment

### Verify Both APIs Work Simultaneously

```bash
# List all VGRs (both APIs)
kubectl get volumegroupreplication.replication.storage.openshift.io --all-namespaces
kubectl get volumegroupreplication.replication.storage.io --all-namespaces

# Both should show their respective VGRs
```

### Test Handler Selection Logic

```bash
# Check logs for handler selection decisions
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator -f | grep -E "(Selected replication handler|offloaded)"

# You should see:
# - Legacy handler selected for rbd-csi-ceph StorageClass
# - Neutral handler selected for pure-flasharray StorageClass
```

---

## Part 5: Test DR Scenarios

### Failover Test (Legacy API)

```bash
# 1. Update VRG to secondary on primary cluster
kubectl patch vrg test-vrg-legacy -n test-legacy --type=merge -p '{"spec":{"replicationState":"secondary"}}'

# 2. Wait for replication to sync
kubectl wait --for=condition=DataReady vrg/test-vrg-legacy -n test-legacy --timeout=300s

# 3. On secondary cluster, create VRG as primary
kubectl apply -f - <<EOF
apiVersion: ramendr.openshift.io/v1alpha1
kind: VolumeReplicationGroup
metadata:
  name: test-vrg-legacy
  namespace: test-legacy
spec:
  pvcSelector:
    matchLabels:
      appname: test-app
  replicationState: primary
  s3Profiles:
    - s3-profile-1
EOF

# 4. Verify application comes up on secondary
kubectl get pods -n test-legacy
```

### Failover Test (Neutral API)

```bash
# Same steps as above, but for neutral namespace
kubectl patch vrg test-vrg-neutral -n test-neutral --type=merge -p '{"spec":{"replicationState":"secondary"}}'
# ... follow same pattern
```

---

## Part 6: Validation and Troubleshooting

### Validation Checklist

- [ ] Both API CRDs installed
- [ ] Ramen operator running
- [ ] Legacy StorageClass creates legacy VGRs
- [ ] Neutral StorageClass creates neutral VGRs
- [ ] Handler selection logs show correct API choice
- [ ] VRG status shows DataReady condition
- [ ] Failover works for both APIs
- [ ] No errors in operator logs

### Common Issues and Solutions

#### Issue 1: VGR Not Created

**Symptom:** VRG exists but no VGR created

**Check:**
```bash
# Check VRG status
kubectl get vrg -n <namespace> <vrg-name> -o yaml | grep -A 10 conditions

# Check operator logs
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator | grep -i error
```

**Solution:**
- Verify StorageClass exists
- Verify VolumeGroupReplicationClass exists
- Check if PVCs have correct labels

#### Issue 2: Wrong API Used

**Symptom:** Expected neutral API but legacy API was used

**Check:**
```bash
# Verify StorageClass has offloaded label
kubectl get storageclass <name> -o yaml | grep offloaded

# Check handler selection logs
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator | grep "Selected replication handler"
```

**Solution:**
- Add `ramendr.openshift.io/offloaded: "true"` label to StorageClass
- Restart Ramen operator if needed

#### Issue 3: Neutral API CRDs Not Found

**Symptom:** Error about missing CRDs

**Check:**
```bash
kubectl get crd | grep replication.storage.io
```

**Solution:**
```bash
cd replication-storage-io-crds
make install
```

### Debug Commands

```bash
# Get all replication resources
kubectl get volumegroupreplication --all-namespaces -A
kubectl get volumegroupreplicationclass -A

# Check VRG status in detail
kubectl describe vrg -n <namespace> <vrg-name>

# Watch operator logs in real-time
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator -f

# Check events
kubectl get events -n <namespace> --sort-by='.lastTimestamp'
```

---

## Part 7: Performance Testing

### Test Replication Performance

```bash
# Create multiple PVCs
for i in {1..10}; do
  kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-pvc-$i
  namespace: test-neutral
  labels:
    appname: test-app
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: pure-flasharray
EOF
done

# Measure time to create VGR
time kubectl wait --for=condition=DataReady vrg/test-vrg-neutral -n test-neutral --timeout=300s
```

### Monitor Resource Usage

```bash
# Check operator resource usage
kubectl top pod -n ramen-system

# Check API server load
kubectl get --raw /metrics | grep apiserver_request_duration
```

---

## Part 8: Cleanup

```bash
# Delete test applications
kubectl delete namespace test-legacy
kubectl delete namespace test-neutral

# Delete VolumeGroupReplicationClasses
kubectl delete volumegroupreplicationclass --all

# Delete StorageClasses (if needed)
kubectl delete storageclass rbd-csi-ceph pure-flasharray

# Uninstall Ramen (if needed)
make undeploy

# Uninstall neutral API CRDs (if needed)
cd replication-storage-io-crds
make uninstall
```

---

## Summary

### What You Tested

1. ✅ Legacy API (ODF/Ceph) continues to work
2. ✅ Neutral API (storage arrays) works with offloaded label
3. ✅ Both APIs can coexist in same cluster
4. ✅ Handler selection works correctly
5. ✅ DR failover works for both APIs

### Key Takeaways

- **StorageClass label controls API selection**
  - No label or `offloaded: "false"` → Legacy API
  - `offloaded: "true"` → Neutral API

- **Both APIs work simultaneously**
  - No conflicts
  - Independent operation
  - Gradual migration possible

- **Backward compatible**
  - Existing deployments unaffected
  - No breaking changes
  - Opt-in for new API

### Next Steps

1. Test with your actual storage backend
2. Validate DR scenarios in your environment
3. Measure performance and resource usage
4. Plan migration strategy for production
5. Provide feedback to Ramen community

---

## Additional Resources

- **Design Document:** `docs/Agnostic-dr-changes-design.docx`
- **Implementation Plan:** `docs/AGNOSTIC_DR_IMPLEMENTATION_PLAN.md`
- **Handler Selector:** `docs/HANDLER_SELECTOR_IMPLEMENTATION.md`
- **API Location Strategy:** `docs/NEUTRAL_API_LOCATION_STRATEGY.md`
- **Test Analysis:** `docs/PHASE6.9_TEST_ANALYSIS.md`

## Support

For issues or questions:
1. Check operator logs first
2. Review this guide's troubleshooting section
3. Open issue on GitHub with logs and configuration
4. Contact Ramen community on Slack
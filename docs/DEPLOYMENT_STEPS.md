# Deployment Steps for Testing Shared Replication API

## Overview

This guide provides the exact steps to deploy and test the Shared Replication API implementation from the `agnostic-dr` branch.

---

## Prerequisites

### Required
- Kubernetes cluster (v1.24+)
- `kubectl` configured and working
- Docker or Podman for building images
- Access to a container registry (quay.io, docker.io, etc.)
- `make` installed

### Optional (for full DR testing)
- Two Kubernetes clusters (primary and secondary)
- OCM (Open Cluster Management) installed
- Storage backend (Ceph/ODF or storage array)

---

## Part 1: Prepare Your Environment

### Step 1: Clone and Checkout Branch

```bash
# If you haven't cloned yet
git clone https://github.com/ramendr/ramen.git
cd ramen

# Checkout the agnostic-dr branch
git checkout agnostic-dr

# Verify you're on the right branch
git branch --show-current
# Should show: agnostic-dr

# Check latest commit
git log --oneline -1
# Should show Phase 6.9 commit
```

### Step 2: Set Environment Variables

```bash
# Set your container registry
export REGISTRY=quay.io/<your-username>
# Or use docker.io, ghcr.io, etc.

# Set image name and tag
export IMG=${REGISTRY}/ramen:agnostic-dr

# Verify
echo "Will build and push to: $IMG"
```

---

## Part 2: Build Ramen Operator

### Step 3: Build the Operator Image

```bash
# Build the operator image
make docker-build IMG=$IMG

# This will:
# - Compile the Go code
# - Create a container image
# - Tag it as $IMG

# Expected output:
# Successfully built <image-id>
# Successfully tagged quay.io/<your-username>/ramen:agnostic-dr
```

### Step 4: Push to Registry

```bash
# Login to your registry (if needed)
docker login quay.io
# Or: podman login quay.io

# Push the image
make docker-push IMG=$IMG

# Verify the image exists
docker images | grep ramen
```

---

## Part 3: Install Neutral API CRDs

### Step 5: Install replication.storage.io CRDs

```bash
# Navigate to the standalone CRD package
cd replication-storage-io-crds

# Install the CRDs
make install

# This installs:
# - volumegroupreplications.replication.storage.io
# - volumegroupreplicationclasses.replication.storage.io
# - volumegroupreplicationcontents.replication.storage.io

# Verify installation
kubectl get crd | grep replication.storage.io

# Expected output:
# volumegroupreplicationclasses.replication.storage.io
# volumegroupreplicationcontents.replication.storage.io
# volumegroupreplications.replication.storage.io
```

### Step 6: Return to Ramen Root

```bash
cd ..
pwd
# Should show: /path/to/ramen
```

---

## Part 4: Deploy Ramen Operator

### Step 7: Deploy to Kubernetes

```bash
# Deploy Ramen using your custom image
make deploy IMG=$IMG

# This will:
# - Create ramen-system namespace
# - Install legacy CRDs (replication.storage.openshift.io)
# - Deploy ramen-hub-operator
# - Deploy ramen-dr-cluster-operator
# - Create necessary RBAC resources

# Wait for deployment to complete (may take 1-2 minutes)
```

### Step 8: Verify Deployment

```bash
# Check namespace
kubectl get namespace ramen-system

# Check pods
kubectl get pods -n ramen-system

# Expected output:
# NAME                                          READY   STATUS    RESTARTS   AGE
# ramen-hub-operator-<hash>                     2/2     Running   0          1m
# ramen-dr-cluster-operator-<hash>              2/2     Running   0          1m

# Wait for pods to be ready
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=ramen -n ramen-system --timeout=300s
```

### Step 9: Verify CRDs

```bash
# Check all replication CRDs are installed
kubectl get crd | grep replication

# Expected output (both APIs):
# volumegroupreplicationclasses.replication.storage.io           <-- NEUTRAL
# volumegroupreplicationclasses.replication.storage.openshift.io <-- LEGACY
# volumegroupreplicationcontents.replication.storage.io          <-- NEUTRAL
# volumegroupreplicationcontents.replication.storage.openshift.io<-- LEGACY
# volumegroupreplications.replication.storage.io                 <-- NEUTRAL
# volumegroupreplications.replication.storage.openshift.io       <-- LEGACY
# volumereplicationclasses.replication.storage.openshift.io      <-- LEGACY
# volumereplications.replication.storage.openshift.io            <-- LEGACY
```

### Step 10: Check Operator Logs

```bash
# View operator logs
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator -f

# Look for:
# - "Starting EventSource" messages
# - "Replication handler initialized" messages
# - No error messages

# Press Ctrl+C to stop following logs
```

---

## Part 5: Quick Smoke Test

### Step 11: Create Test Namespace

```bash
kubectl create namespace ramen-test
```

### Step 12: Create a Simple StorageClass (Legacy)

```bash
# Create a test StorageClass without offloaded label
kubectl apply -f - <<EOF
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: test-legacy-sc
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
EOF

# Verify
kubectl get storageclass test-legacy-sc
```

### Step 13: Create VolumeGroupReplicationClass (Legacy)

```bash
kubectl apply -f - <<EOF
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplicationClass
metadata:
  name: test-vgrc-legacy
  labels:
    ramendr.openshift.io/replicationid: test-repl
spec:
  provisioner: kubernetes.io/no-provisioner
  parameters:
    replication.storage.openshift.io/replication-secret-name: test-secret
    replication.storage.openshift.io/replication-secret-namespace: ramen-test
EOF

# Verify
kubectl get volumegroupreplicationclass test-vgrc-legacy
```

### Step 14: Create Test PVC

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-pvc
  namespace: ramen-test
  labels:
    appname: test-app
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: test-legacy-sc
EOF

# Verify (will be Pending without actual storage)
kubectl get pvc -n ramen-test
```

### Step 15: Create VolumeReplicationGroup

```bash
kubectl apply -f - <<EOF
apiVersion: ramendr.openshift.io/v1alpha1
kind: VolumeReplicationGroup
metadata:
  name: test-vrg
  namespace: ramen-test
spec:
  pvcSelector:
    matchLabels:
      appname: test-app
  replicationState: primary
  s3Profiles:
    - test-profile
EOF

# Verify VRG was created
kubectl get vrg -n ramen-test test-vrg
```

### Step 16: Check VRG Status and Logs

```bash
# Check VRG status
kubectl get vrg -n ramen-test test-vrg -o yaml | grep -A 20 status

# Check operator logs for handler selection
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator | grep -E "(Selected replication handler|test-vrg)"

# Expected in logs:
# "Selected replication handler for StorageClass"
# "storageClass":"test-legacy-sc"
# "handlerType":"legacy"
# "apiGroup":"replication.storage.openshift.io"
```

---

## Part 6: Test Neutral API (Optional)

### Step 17: Create StorageClass with Offloaded Label

```bash
kubectl apply -f - <<EOF
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: test-neutral-sc
  labels:
    ramendr.openshift.io/offloaded: "true"  # KEY: Triggers neutral API
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
EOF
```

### Step 18: Create VolumeGroupReplicationClass (Neutral)

```bash
kubectl apply -f - <<EOF
apiVersion: replication.storage.io/v1alpha1
kind: VolumeGroupReplicationClass
metadata:
  name: test-vgrc-neutral
  labels:
    ramendr.openshift.io/replicationid: test-repl-neutral
spec:
  provisioner: kubernetes.io/no-provisioner
  parameters:
    replication.storage.io/replication-secret-name: test-secret
    replication.storage.io/replication-secret-namespace: ramen-test
EOF
```

### Step 19: Create PVC and VRG for Neutral API

```bash
# Create PVC
kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-pvc-neutral
  namespace: ramen-test
  labels:
    appname: test-app-neutral
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: test-neutral-sc
EOF

# Create VRG
kubectl apply -f - <<EOF
apiVersion: ramendr.openshift.io/v1alpha1
kind: VolumeReplicationGroup
metadata:
  name: test-vrg-neutral
  namespace: ramen-test
spec:
  pvcSelector:
    matchLabels:
      appname: test-app-neutral
  replicationState: primary
  s3Profiles:
    - test-profile
EOF
```

### Step 20: Verify Neutral API is Used

```bash
# Check logs for neutral handler selection
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator | grep -E "(test-vrg-neutral|test-neutral-sc)"

# Expected in logs:
# "Selected replication handler for StorageClass"
# "storageClass":"test-neutral-sc"
# "handlerType":"neutral"
# "apiGroup":"replication.storage.io"
```

---

## Part 7: Verification

### Step 21: Verify Both APIs Work

```bash
# List VGRs from both APIs
echo "=== Legacy API VGRs ==="
kubectl get volumegroupreplication.replication.storage.openshift.io -n ramen-test

echo "=== Neutral API VGRs ==="
kubectl get volumegroupreplication.replication.storage.io -n ramen-test

# Both should show their respective VGRs (if PVCs are bound)
```

### Step 22: Check Handler Selection Summary

```bash
# Get summary of handler selections
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator | \
  grep "Selected replication handler" | \
  tail -10

# You should see:
# - Legacy handler for test-legacy-sc
# - Neutral handler for test-neutral-sc
```

---

## Part 8: Cleanup (Optional)

### Step 23: Remove Test Resources

```bash
# Delete VRGs
kubectl delete vrg --all -n ramen-test

# Delete PVCs
kubectl delete pvc --all -n ramen-test

# Delete VGRCs
kubectl delete volumegroupreplicationclass --all

# Delete StorageClasses
kubectl delete storageclass test-legacy-sc test-neutral-sc

# Delete namespace
kubectl delete namespace ramen-test
```

### Step 24: Uninstall Ramen (if needed)

```bash
# Uninstall Ramen operator
make undeploy

# Uninstall neutral API CRDs
cd replication-storage-io-crds
make uninstall
cd ..

# Verify cleanup
kubectl get namespace ramen-system
# Should show: NotFound
```

---

## Troubleshooting

### Issue: Pods Not Starting

```bash
# Check pod status
kubectl describe pod -n ramen-system -l app.kubernetes.io/name=ramen

# Check events
kubectl get events -n ramen-system --sort-by='.lastTimestamp'

# Common causes:
# - Image pull errors (check registry access)
# - Resource constraints (check node resources)
# - RBAC issues (check service account permissions)
```

### Issue: CRDs Not Installing

```bash
# Manually install CRDs
kubectl apply -f replication-storage-io-crds/config/crd/bases/

# Verify
kubectl get crd | grep replication.storage.io
```

### Issue: VGR Not Created

```bash
# Check VRG status
kubectl describe vrg -n ramen-test <vrg-name>

# Check operator logs
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator | grep -i error

# Common causes:
# - PVC not bound
# - StorageClass not found
# - VGRC not found
# - Missing labels on PVC
```

---

## Success Criteria

✅ **Deployment Successful If:**
1. Both operators running in ramen-system namespace
2. Both API CRDs installed (legacy and neutral)
3. No errors in operator logs
4. VRGs can be created
5. Handler selection logs show correct API choice

✅ **Functionality Verified If:**
1. Legacy StorageClass uses legacy API
2. Neutral StorageClass (with offloaded label) uses neutral API
3. Both APIs can coexist
4. VRG status updates correctly

---

## Next Steps

After successful deployment:

1. **Test with Real Storage**
   - Use actual Ceph/ODF for legacy API
   - Use storage array for neutral API

2. **Test DR Scenarios**
   - Failover between clusters
   - Failback to primary
   - Data consistency validation

3. **Performance Testing**
   - Multiple PVCs
   - Large volumes
   - Concurrent operations

4. **Integration Testing**
   - With OCM
   - With application workloads
   - With backup/restore

---

## Quick Reference

### Key Commands

```bash
# Check deployment status
kubectl get pods -n ramen-system

# View logs
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator -f

# List VRGs
kubectl get vrg --all-namespaces

# Check handler selection
kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator | grep "Selected replication handler"

# Verify CRDs
kubectl get crd | grep replication
```

### Key Files

- **Main Branch:** `agnostic-dr`
- **Neutral API Package:** `replication-storage-io-crds/`
- **Handler Code:** `internal/controller/replication/`
- **Documentation:** `docs/`

### Support

- **Logs:** `kubectl logs -n ramen-system deployment/ramen-dr-cluster-operator`
- **Events:** `kubectl get events -n ramen-system`
- **Status:** `kubectl describe vrg -n <namespace> <name>`

---

## Summary

This deployment guide provides:
- ✅ Step-by-step instructions
- ✅ Verification at each step
- ✅ Smoke tests for both APIs
- ✅ Troubleshooting guidance
- ✅ Success criteria

Follow these steps in order, and you'll have a working deployment of the Shared Replication API ready for testing!
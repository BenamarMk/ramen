# VRG Replication API Priority Annotation

## Overview

The VolumeReplicationGroup (VRG) now supports an annotation that allows you to explicitly prioritize which replication API to use: the VolumeReplication API from csi-addons (volrep) or the neutral replication API.

## Annotation

**Key:** `ramendr.openshift.io/replication-api-priority`

**Valid Values:**
- `volrep` - Prioritize the VolumeReplication API from csi-addons (replication.storage.openshift.io)
- `neutral` - Prioritize the neutral replication API (replication.storage.io)

## Behavior

### When annotation is set to "neutral"
The VRG will use the neutral replication API regardless of whether volrep CRDs are available.

### When annotation is set to "volrep"
The VRG will use the volrep API only if the CRDs are available. If volrep CRDs are not available, it will fall back to the neutral API.

### When annotation is not set or has an invalid value
The VRG will automatically detect which CRDs are available and use volrep if available, otherwise neutral.

## Usage Examples

### Example 1: Force use of neutral API

```yaml
apiVersion: ramendr.openshift.io/v1alpha1
kind: VolumeReplicationGroup
metadata:
  name: my-app-vrg
  namespace: my-app
  annotations:
    ramendr.openshift.io/replication-api-priority: "neutral"
spec:
  pvcSelector:
    matchLabels:
      app: my-app
  replicationState: primary
  s3Profiles:
    - s3-profile-1
  async:
    schedulingInterval: "5m"
    replicationClassSelector:
      matchLabels:
        ramendr.openshift.io/replicationid: storage-replication-id
```

### Example 2: Prefer volrep API

```yaml
apiVersion: ramendr.openshift.io/v1alpha1
kind: VolumeReplicationGroup
metadata:
  name: my-app-vrg
  namespace: my-app
  annotations:
    ramendr.openshift.io/replication-api-priority: "volrep"
spec:
  pvcSelector:
    matchLabels:
      app: my-app
  replicationState: primary
  s3Profiles:
    - s3-profile-1
  async:
    schedulingInterval: "5m"
    replicationClassSelector:
      matchLabels:
        ramendr.openshift.io/replicationid: storage-replication-id
```

### Example 3: Auto-detect (no annotation)

```yaml
apiVersion: ramendr.openshift.io/v1alpha1
kind: VolumeReplicationGroup
metadata:
  name: my-app-vrg
  namespace: my-app
spec:
  pvcSelector:
    matchLabels:
      app: my-app
  replicationState: primary
  s3Profiles:
    - s3-profile-1
  async:
    schedulingInterval: "5m"
    replicationClassSelector:
      matchLabels:
        ramendr.openshift.io/replicationid: storage-replication-id
```

## Use Cases

### Testing Both APIs
During development or testing, you can use this annotation to test both APIs on the same cluster without needing to install/uninstall CRDs:

1. Create a VRG with `replication-api-priority: "volrep"` to test volrep behavior
2. Update the same VRG with `replication-api-priority: "neutral"` to test neutral behavior
3. Compare the behavior and performance

### Migration Scenarios
When migrating from one API to another:

1. Start with the current API explicitly set via annotation
2. Verify everything works as expected
3. Gradually migrate workloads by changing the annotation
4. Monitor and validate the migration

### Environment-Specific Configuration
Different environments may have different API availability:

- **Development**: Use `neutral` for consistent behavior across all dev clusters
- **Staging**: Use `volrep` to match production if volrep is available
- **Production**: Use auto-detect or explicit `volrep` based on your storage backend

## Logging

When a VRG is reconciled, the controller logs which API is being used:

```
Using VolumeGroupReplication CRDs from csi-addons (replication.storage.openshift.io)
```

or

```
Using VolumeGroupReplication CRDs from neutral implementation (replication.storage.io)
```

This helps verify that the correct API is being used based on your annotation.

## Implementation Details

The annotation is checked in the `ReplicationFactory.IsUsingVolrep()` method, which is called throughout the VRG reconciliation process to determine which API objects to create and manage.

The decision flow is:
1. Check if annotation exists and has a valid value
2. If "neutral", return false (use neutral API)
3. If "volrep", check if volrep CRDs are available
4. If no annotation or invalid value, fall back to CRD detection

## Constants

The annotation key and values are defined as constants in `api/v1alpha1/volumereplicationgroup_types.go`:

```go
const (
    VRGReplicationAPIPriorityAnnotation = "ramendr.openshift.io/replication-api-priority"
)

const (
    ReplicationAPIPriorityVolrep  = "volrep"
    ReplicationAPIPriorityNeutral = "neutral"
)
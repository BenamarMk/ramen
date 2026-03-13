# Neutral Replication API (replication.storage.io)

This package contains the **vendor-neutral** replication API definitions for Kubernetes storage replication.

## Overview

The `replication.storage.io` API group provides a standardized, vendor-agnostic interface for volume group replication in Kubernetes. This API is designed to be consumed by DR orchestrators (like Ramen) and implemented by storage vendors.

## Architecture: The Three-Tier Model

### Tier 1: API Provider (Neutral) - **This Package**
- **Repository**: `github.com/ramendr/ramen/api/replication.storage.io`
- **API Group**: `replication.storage.io`
- **Responsibility**: Defines the schema and validation logic for replication operations

### Tier 2: Orchestrator (Ramen)
- **Responsibility**: Discovers neutral VGRCs, matches them to PeerClasses, creates VGR instances
- **Logic**: Consumes the `replication.storage.io` contract, agnostic to storage backend

### Tier 3: Implementation (Storage Vendors)
- **Responsibility**: Watches neutral VGR objects and executes storage-level replication
- **Examples**: csi-addons, ODF, Dell, NetApp, Pure Storage, etc.

## API Resources

### VolumeGroupReplication (VGR)
Represents a replication operation on a group of volumes.

**Key Fields:**
- `spec.replicationState`: Desired state (`primary`, `secondary`, `resync`)
- `spec.volumeGroupReplicationClassName`: Reference to VGRC
- `spec.source.selector`: Label selector for PVCs to replicate
- `status.state`: Current replication state
- `status.conditions`: Detailed status conditions

### VolumeGroupReplicationClass (VGRC)
Defines the replication parameters and provisioner.

**Key Fields:**
- `spec.provisioner`: Name of the CSI driver/storage provisioner
- `spec.parameters`: Storage-specific configuration (schedule, etc.)

### VolumeGroupReplicationContent (VGRC)
Represents the actual storage-level replication snapshot/handle.

**Key Fields:**
- `spec.volumeGroupReplicationRef`: Reference to parent VGR
- `spec.provisioner`: CSI driver name
- `spec.volumeGroupReplicationHandle`: Storage system identifier

## Usage

### For DR Orchestrators (Ramen)

```go
import replicationv1alpha1 "github.com/ramendr/ramen/api/replication.storage.io/v1alpha1"

// Discover VGRCs
vgrcList := &replicationv1alpha1.VolumeGroupReplicationClassList{}
err := client.List(ctx, vgrcList)

// Create VGR
vgr := &replicationv1alpha1.VolumeGroupReplication{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "my-app-replication",
        Namespace: "my-app",
    },
    Spec: replicationv1alpha1.VolumeGroupReplicationSpec{
        ReplicationState: replicationv1alpha1.Primary,
        VolumeGroupReplicationClassName: "ceph-rbd-vgrc",
        Source: replicationv1alpha1.VolumeGroupReplicationSource{
            Selector: &metav1.LabelSelector{
                MatchLabels: map[string]string{
                    "app": "my-app",
                },
            },
        },
    },
}
err = client.Create(ctx, vgr)
```

### For Storage Vendors

Implement a controller that:
1. Watches `VolumeGroupReplication` resources
2. Reconciles replication state with your storage backend
3. Updates `VolumeGroupReplication.Status` with current state

```go
import replicationv1alpha1 "github.com/ramendr/ramen/api/replication.storage.io/v1alpha1"

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    vgr := &replicationv1alpha1.VolumeGroupReplication{}
    if err := r.Get(ctx, req.NamespacedName, vgr); err != nil {
        return ctrl.Result{}, err
    }
    
    // Implement your storage-specific replication logic
    switch vgr.Spec.ReplicationState {
    case replicationv1alpha1.Primary:
        // Promote volumes to primary
    case replicationv1alpha1.Secondary:
        // Demote volumes to secondary
    case replicationv1alpha1.Resync:
        // Trigger resync operation
    }
    
    // Update status
    vgr.Status.State = replicationv1alpha1.PrimaryState
    return ctrl.Result{}, r.Status().Update(ctx, vgr)
}
```

## Labels and Annotations

### Required Labels on VGRC

- `ramendr.openshift.io/storageid`: Unique identifier for the storage backend
- `ramendr.openshift.io/replicationid`: Unique identifier for the replication relationship
- `ramendr.openshift.io/groupreplicationid`: Unique identifier for group replication capability

### Required Parameters in VGRC

- `schedulingInterval`: Replication schedule (e.g., `5m`, `1h`, `1d`)

## Migration from Legacy API

If you're currently using `replication.storage.openshift.io` (csi-addons), the neutral API is designed to be compatible:

1. **API Structure**: Nearly identical spec/status fields
2. **Behavior**: Same replication semantics
3. **Coexistence**: Both APIs can run simultaneously during transition

### Key Differences

| Aspect | Legacy (`replication.storage.openshift.io`) | Neutral (`replication.storage.io`) |
|--------|---------------------------------------------|-------------------------------------|
| API Group | `replication.storage.openshift.io` | `replication.storage.io` |
| Governance | csi-addons project | Ramen community (transitioning to K8s SIG) |
| Vendor Perception | OpenShift-specific | Kubernetes standard |
| Adoption | ODF/Ceph primarily | Multi-vendor support |

## Development

### Generate Code

```bash
# From repository root
make controller-gen
./bin/controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./api/replication.storage.io/..."
```

### Run Tests

```bash
cd api/replication.storage.io
go test ./...
```

## Roadmap

### Phase 1: Incubation (Current)
- ✅ API hosted in Ramen repository
- ✅ Neutral API group (`replication.storage.io`)
- 🔄 Ramen implements dual-mode support (legacy + neutral)

### Phase 2: Decoupling
- Move API to dedicated GitHub organization
- Independent versioning and releases
- Ramen and csi-addons update imports

### Phase 3: Standardization
- Propose as Kubernetes SIG project
- CNCF-managed CRD set
- Industry-wide adoption

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](../../CONTRIBUTING.md) for guidelines.

## License

Apache 2.0 - See [LICENSE](../../LICENSE) for details.

## References

- [Design Document](../../docs/Agnostic-dr-changes-design.docx)
- [Ramen Project](https://github.com/ramendr/ramen)
- [csi-addons (Legacy API)](https://github.com/csi-addons/kubernetes-csi-addons)
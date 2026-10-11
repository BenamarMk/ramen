<!--
SPDX-FileCopyrightText: The RamenDR authors
SPDX-License-Identifier: Apache-2.0
-->

# Test Failover Into Generated Test Namespaces

**Status:** iteration 12 — implemented. §3 records every decision. §13 holds
implementation notes. Sections amended after a first run on a real cluster
are marked **[from implementation]**.

## 1. Scope

Ramen always restores protected Kubernetes resources back into the namespace
they were captured from. Every restore is an in-place restore.

Test Failover needs the opposite: the test copy of the application must come up
**beside** the real workload, in namespaces Ramen generates, so that the test
cluster can be returned to Secondary while the test copy keeps running for the
user to inspect.

### In scope for the first release

- **`spec.dryRun` is its own mode.** Setting it starts a test failover. The
  user does not set `spec.action`, and whatever `spec.action` holds is left
  alone (decision 22).
- **VolRep only.** VolSync-protected workloads are refused outright.
- **Non-CephFS only.** Workloads with any CephFS PVC are refused outright.
- **Async (Regional) DR only.** Sync/Metro is refused — decision 26.
- **Discovered Apps only** — `DRPC.spec.protectedNamespaces` non-empty, VRG in
  the Ramen ops namespace (`isDiscoveredApp`, `drplacementcontrol.go:1074`).
- One generated test namespace per protected namespace, with the application's
  PVCs and kube objects restored into it.
- The test copy survives the automatic revert and runs until the user
  clears `spec.dryRun`, which takes it down — decision 38.

### Deferred to a later release

- VolSync and CephFS volumes; Sync/Metro DR.
- Managed (ACM-deployed) applications.
- Recipe hooks during a test restore (§5.8).
- Static IP translation for the test copy (§5.8).
- Automatic failure handling of any kind — no timeout, no `Failed` state
  (§5.9, decision 21).
- Detecting that a previous run's test namespaces were never cleaned up
  (§5.10, decision 20).
- Demoting volumes before the test copy is built (§12, decision 19).

### Out of scope entirely

- Rewriting namespace references *inside* restored object bodies (FQDNs in
  environment variables, webhook `clientConfig` service references, namespaces
  hardcoded in ConfigMap payloads). Velero rewrites `metadata.namespace` only.
- Changing the Recipe CRD. The Recipe API lives in an external module
  (`github.com/ramendr/recipe`); this design avoids depending on a change
  there.
- **Ramen writing to `DRPC.spec`.** Every spec edit is the user's.

## 2. Target Flow

```
 HUB                                    TEST CLUSTER (current Secondary)
 ───                                    ───────────────────────────────
 user: spec.dryRun = true
       spec.action untouched
      │
      │ executeAction() → RunDryRunFailover()
      │ pre-flight: idle? discovered app? async? no VolSync?
      │ annotate  dry-run: SettingUp
      │ PeerReady = false
      ▼
 VRG ManifestWork ─────────────────────► VRG: Secondary → Primary
 (replicationState: Primary,               dryRun=true, action=Failover
  dryRun: true, action: Failover)          │
                                   ① restore PV/PVC from S3 into the
                                     protected namespaces      (existing)
                                           │
                                   ② promote VolumeReplications (existing)
                                           │
                                   ③ snapshot every protected PVC
                                     ensureSnapshotsForDryRun() (existing)
                                           │
                                   ┌───────┴──────────────────────────┐
                                   │   internal/controller/dryrun     │
                                   │                                  │
                                   │ ④ stall if CephFS                │
                                   │ ⑤ materialise test namespaces    │
                                   │   workload1 → workload1-test-ID  │
                                   │   workload2 → workload2-test-ID  │
                                   │ ⑥ hand each snapshot to its test │
                                   │   namespace, create the test PVC │
                                   │ ⑦ velero restore of kube objects │
                                   │   into the test namespaces ONLY  │
                                   │ ⑧ status.dryRun.state=Completed  │
                                   └───────┬──────────────────────────┘
                                           │
      ◄────────── ManagedClusterView ──────┘
      │
      │ annotate  dry-run: Reverting
      ▼
 VRG ManifestWork ─────────────────────► VRG: Primary → Secondary
 (replicationState: Secondary,             │
  dryRun: false)                   ⑨ HandOff: give the storage snapshots
                                     to the test namespaces
                                           │
                                   ⑩ delete the PVCs restored in ① -- Ramen
                                     created them, so Ramen removes them --
                                     then demote, via the Secondary path
                                           │
                                   ⑪ test namespaces, their PVCs and the
                                     restored application are LEFT RUNNING
      │
      │ target VRG confirmed Secondary
      │ PeerReady = true
      │ annotate  dry-run: Completed      ← terminal
      ▼
 DRPC status lists the test namespaces.
 Another dry run requires deleting the annotation; the user is
 expected to delete the test namespaces too (§12).
```

Meanwhile, on the **source cluster**, nothing at all happens. Its VRG is never
touched, the application keeps running, and the user Placement is never moved.

Four properties define the design:

**The dry run is a mode, not an action.** `spec.action` keeps whatever value it
had — empty, `Failover` or `Relocate` — and is neither read nor written by the
dry-run path, except to notice that the user changed it (§5.4). The DRPC's
`status.phase`, `LastAppDeploymentCluster`, `DRPCLastAction` and user Placement
are all left exactly as they were.

**The real application never moves and is never restored anywhere.** The
in-place kube object restore is skipped on the test cluster. Only PVCs and PVs
are restored there, and only so that VolumeReplications can be promoted and
snapshotted. They are torn down on revert.

**The revert is automatic, and Ramen still never writes `spec`.** These stopped
being in tension the moment the dry run stopped borrowing `spec.action`: there
is nothing in the spec to put back, so reverting is purely an internal
transition.

**Nothing Ramen creates in a test namespace may be reachable from the VRG
teardown path** — no `ownerReference` to the VRG, and no label the cleanup
code selects on.

## 3. Resolved Decisions

| #  | Decision                                                             |
| -- | -------------------------------------------------------------------- |
| 1  | Ramen never writes `DRPC.spec`; the revert is internal and automatic |
| 2  | Lifecycle is driven by a **dry-run state annotation**, §5.3          |
| 3  | `ProgressionTestingFailover` **kept but deprecated**, no longer set  |
| 4  | Test namespace suffix is **shared** across namespaces, §5.5          |
| 5  | `includeClusterResources` stays **true**; consequences in §12        |
| 6  | Ramen is granted **RBAC write** on the managed cluster, §6           |
| 7  | A run that cannot proceed **stays stuck**; no automatic failure      |
| 8  | A second dry run is gated by the **annotation**, which Ramen removes |
|    | once the copy is torn down, §5.3                                     |
| 9  | Managed-cluster logic lives in a **separate package**, §5.1          |
| 10 | The two S3 naming helpers are **shared, not duplicated**, §5.1       |
| 11 | Recipe hooks are **skipped** during a test restore, §5.8             |
| 12 | Static IP translation is **not applied** to the test copy, §5.8      |
| 13 | There is **no `Failed` state** — stuck runs log and requeue, §5.9    |
| 14 | VolSync workloads are **refused**, not partially tested, §5.10       |
| 15 | Test namespaces are reported in **DRPC status**, §5.9                |
| 16 | CephFS workloads are **refused**, for the same reason as VolSync     |
| 17 | DRPC deleted mid-run **leaves the test copy**; no new finalizer,     |
|    | but the VRG delete path must run `HandOff`, §5.4                     |
| 18 | States are `SettingUp → Reverting → Completed`, §5.3                 |
| 19 | Volumes stay promoted for the whole build; RPO window accepted       |
| 20 | `runID` is **derived from the DRPC UID**; a repeat run **reuses      |
|    | whatever is there**. No leftover detection. §5.5, §12                |
| 21 | **No timeout.** A stalled run waits for a human indefinitely, §5.9   |
| 22 | **`spec.dryRun` alone starts the test.** `spec.action` is not read   |
|    | to decide *what* to do and is never written, §5.2                    |
| 23 | The dry run **never moves Placement** and never changes              |
|    | `status.phase`, `LastAppDeploymentCluster` or `DRPCLastAction`       |
| 24 | The **source cluster is never touched** — no demote, no cleanup      |
| 25 | The test VRG always gets `Action: Failover` + `DryRun: true`         |
|    | regardless of `DRPC.spec.action`, §5.2                               |
| 26 | **Sync/Metro DR is refused** — a test must never fence a live peer   |
| 27 | DRPC-side dry-run code is consolidated into one file, §5.1, §8       |
| 28 | The test target is **derived** as the peer that is not Primary;      |
|    | `spec.failoverCluster` is not consulted, §5.2.1                      |
| 29 | `executeAction` branches on `dryRun && !dryRunFinished()`, §5.2      |
| 30 | A real action requested mid-run **pre-empts** the dry run, §5.4      |
| 31 | `dryRun` cleared mid-run **hands over to a real failover** when the  |
|    | action asks for one, otherwise aborts, §5.4                          |
| 32 | The revert reuses **`EnsureCleanup`**, not the wrapper above it,     |
|    | §5.2 — the wrapper writes to the source cluster                      |
| 33 | A real action arriving during `Reverting` **waits**; no second       |
|    | pre-emption path, §5.4                                               |
| 34 | The two-Primary window is accepted, budgeted at **~5 minutes**, §12  |
| 35 | The legacy dry-run annotations are **deleted**, not carried, §8.1   |
| 36 | The revert **deletes the PVCs step ① restored**; nothing else will, |
|    | §5.7.1. Guarded on `status.dryRun`, never on the annotation         |
| 37 | **Auto-resync stays on through every dry-run phase**, including    |
|    | the terminal one, §5.7.2                                            |
| 38 | Clearing `spec.dryRun` **tears the test copy down**, and Ramen then |
|    | removes its own annotation, §5.3.1. Supersedes manual teardown      |

## 4. Current Implementation

Line references verified against `main` at `c7d361cb`.

### 4.1 The Velero wire already exists — and is dead

`kubeobjects.RecoverSpec` already carries a `NamespaceMapping`, already passed
through to Velero:

```go
// internal/controller/kubeobjects/requests.go:59
NamespaceMapping map[string]string `json:"namespaceMapping,omitempty"`

// internal/controller/kubeobjects/velero/requests.go:781
NamespaceMapping: recoverSpec.NamespaceMapping,
```

**Nothing in the repository ever sets it.** Both producers of `RecoverSpec` —
`convertRecipeGroupToRecoverSpec` (`vrg_kubeobjects.go:1496`) and
`recoverWorkflowDefault` (`vrg_recipe.go:60`) — leave it nil.

`RecoverSpec.ExistingResourcePolicy` (`requests.go:63`) is left empty, so
Velero's default applies: objects that already exist in the target namespace
are **skipped**, not overwritten. This matters for repeat runs — see §12.

### 4.2 The primitives `RunDryRunFailover` needs already exist

- `createVRGManifestWorkAsPrimary(cluster)` — `drplacementcontrol.go:1504`.
  Creates the VRG ManifestWork as Primary, or flips an existing Secondary VRG
  to Primary.
- `updateVRGState(cluster, Secondary)` — `drplacementcontrol.go:2583`. The
  reverse.
- `EnsureCleanup(clusterToSkip)` — `drplacementcontrol.go:2321`. Demotes every
  peer that is not `clusterToSkip`, waits for each to reach Secondary, and
  sets `PeerReady=True` when they all have.
- `MCVGetter.GetVRGFromManagedCluster` — `drplacementcontrol.go:571`. Reads the
  test cluster's VRG status on the hub.

A dry run is these three in sequence. It needs none of
`switchToFailoverCluster` (`:621`), `ensureActionTransition` (`:1078`),
`ensurePlacement` (`:2048`), `updateUserPlacementRule`, `setDRState` or
`retainClusterDecisionAsFailover`.

### 4.3 `setVRGAction` derives the VRG action from the DRPC action

```go
// drplacementcontrol.go:2087
func (d *DRPCInstance) setVRGAction(vrg *rmn.VolumeReplicationGroup) {
    action := vrgAction(d.instance.Spec.Action)
    ...
    if action == "" {
        return
    }
    vrg.Spec.Action = action
```

With `spec.action` empty — now the common case for a dry run —
`vrg.Spec.Action` is never set. The managed cluster's
`shouldTakeDryRunSnapshots` (`vrg_volrep.go:3642`) requires
`Action == VRGActionFailover`, so **no snapshots would ever be taken and the
run would stall forever.** Decision 25 exists because of this: the dry-run path
sets the test VRG's action explicitly rather than deriving it.

### 4.4 DRPC deletion already drains both Primaries

`finalizeDRPC` (`drplacementcontrol_controller.go:961`) deletes Secondary VRGs
first and then Primary VRGs, and `processDeletion` holds `DRPCFinalizer` until
`len(vrgs) == 0`.

During a dry run both clusters hold Primary VRGs, so the Secondary pass is a
no-op and the Primary pass deletes both. **The existing finalizer therefore
already prevents the test cluster being stranded Primary** — the dangerous
outcome worried about in iteration 9. No new finalizer is needed; what is
needed is that the VRG delete path hands the snapshots over first (§5.4).

### 4.5 Dry-run snapshots already exist

`ensureSnapshotsForDryRun` (`vrg_volrep.go:3666`), called from
`reconcileVolRepsAsPrimary` when the VRG is Primary, `dryRun` is true and the
action is Failover. It creates one `VolumeSnapshot` named `<pvc>-snapshot` per
non-CephFS **VolRep** PVC, in the PVC's own namespace (`createSnapshot`,
`vrg_volrep.go:3831`), labelled `ramendr.openshift.io/dry-run-snapshot=true`
and `ramendr.openshift.io/dry-run-vrg=<vrgName>`.

It does not wait for `ReadyToUse`; the dry-run package gates on that itself. It
treats `AlreadyExists` as success (`vrg_volrep.go:3853`).

### 4.6 Capture is already suppressed during dry-run

`kubeObjectsProtectPrimary` (`vrg_kubeobjects.go:71`) returns early when
`spec.DryRun` is set, so a test restore can never be captured back into S3.

### 4.7 What the hub can and cannot see

Through `MCVGetter.GetVRGFromManagedCluster` the hub gets:

- **VolSync, yes.** `ProtectedPVC.ProtectedByVolSync`
  (`volumereplicationgroup_types.go:341`), already read on the hub
  (`drplacementcontrolvolsync.go:265`).
- **CephFS, no.** `ProtectedPVC` carries only `StorageClassName`; resolving it
  to a provisioner needs the StorageClass on the managed cluster.
- **Namespaces, no.** No view of arbitrary managed-cluster objects without a
  `ManagedClusterView` per object.

This is why §5.10 splits the guards across both clusters.

### 4.8 Velero capabilities confirmed

Against the vendored Velero `v1.15.0`:

- `conditions.groupResource` is matched as a **glob**
  (`resourcemodifiers/resource_modifiers.go:122`), so `"*"` matches everything.
- `mergePatches` is supported (`resource_modifiers.go:57`).
- `GetResourceModifiersFromConfig` rejects a ConfigMap with more than one data
  key (`resource_modifiers.go:69`).

## 5. Architecture

### 5.1 Where the code lives

Two homes, one per cluster role.

**Hub — `internal/controller/drplacementcontrol_dryrun.go`**, a new file in
`package controllers`, mirroring how `drplacementcontrolvolsync.go` is
organised. It needs deep access to `DRPCInstance`, so a separate package would
buy nothing. Every piece of dry-run logic currently scattered through
`drplacementcontrol.go` moves here and the scattered conditionals are deleted —
see §8.

**Managed cluster — `internal/controller/dryrun/`**, a new package. The VRG
reconcile reaches it through one `if` and one adapter, so the production
restore path takes a zero-line diff. Duplicating a few helpers is accepted in
exchange for a cleanly deletable unit.

```
internal/controller/dryrun/
├── dryrun.go      entry point and state machine
├── identity.go    run ID, target namespace naming
├── eligibility.go CephFS refusal
├── namespaces.go  namespace materialisation and policy copy
├── volumes.go     VolumeSnapshotContent hand-off, test PVC creation
├── kubeobjects.go Velero restore into the test namespaces
├── modifier.go    ownerReferences-strip ResourceModifier ConfigMap
└── handoff.go     source-side snapshot hand-off and request cleanup
```

```go
package dryrun

type Input struct {
    Client, APIReader, Log, VRG, RecipeElements,
    VolRepPVCs, S3Accessors, KubeObjects, VeleroNamespace ...
}

type Output struct {
    State            ramen.DryRunState   // InProgress | Completed
    NamespaceMapping map[string]string
    Message          string              // why it is stuck, if it is
    Requeue          bool
}

// Reconcile drives steps ④–⑧ of §2. Idempotent. It never reports failure:
// anything it cannot do yet leaves State InProgress, logs the reason and
// sets Requeue.
func Reconcile(ctx context.Context, in Input) (Output, error)

// HandOff gives the storage snapshots to the test namespaces and deletes the
// dry-run Velero request objects. Replaces cleanupDryRunSnapshots for the
// dry-run case; tolerates partial state. Called on revert, on promotion, and
// from the VRG delete path (§5.4).
func HandOff(ctx context.Context, in Input) error
```

A thin adapter `vrg_dryrun.go` in `package controllers` builds `Input` from
`VRGInstance` and applies `Output`. In `processAsPrimary`
(`volumereplicationgroup_controller.go:1497`):

```go
if v.instance.Spec.DryRun {
    return v.reconcileDryRun()        // adapter → dryrun.Reconcile
}

if v.shouldRestoreKubeObjects() {
    ...unchanged...
}
```

That single `if` is what keeps the real application off the test cluster.

**The one shared piece.** The S3 object layout is defined by
`kubeObjectsCapturePathNamesAndNamePrefix` (`vrg_kubeobjects.go:45`) and
`kubeObjectsCaptureName` (`:59`). Copying them would mean that any later drift
between the copies breaks restores *silently* — no error, just an empty test
namespace. They are **exported into a shared helper**, candidate location
`internal/controller/kubeobjects/naming.go`, consumed by both paths. This is
the single exception to the duplication-is-acceptable rule and it lands in
Phase 0.

### 5.2 Hub: `RunDryRunFailover`

`executeAction` gains a dry-run branch ahead of the action switch:

```go
func (d *DRPCInstance) executeAction() (bool, error) {
    if d.instance.Spec.DryRun && !d.dryRunFinished() {
        return d.RunDryRunFailover()
    }

    switch d.instance.Spec.Action {
    case rmn.ActionFailover:
        return d.RunFailover()
    case rmn.ActionRelocate:
        return d.RunRelocate()
    }

    return d.RunInitialDeployment()
}
```

`dryRunFinished()` is `annotation == Completed` (decision 29). Without it a
DRPC left with `dryRun: true` after a completed test could never perform a real
failover — `executeAction` would keep returning to a `RunDryRunFailover` that
has nothing left to do. The consequence is that `spec.dryRun` alone is not the
state; the annotation is.

`RunDryRunFailover` is a three-state machine driven entirely by the annotation:

```go
func (d *DRPCInstance) RunDryRunFailover() (bool, error) {
    const done = true

    switch d.dryRunState() {
    case "":                      return d.dryRunStart()
    case rmn.DryRunSettingUp:     return d.dryRunBuild()
    case rmn.DryRunReverting:     return d.dryRunRevert()
    case rmn.DryRunCompleted:     return done, nil
    }
}
```

**`dryRunStart`** — runs the hub pre-flight (§5.10). On success it records the
spec it started against, writes the `SettingUp` annotation, sets
`PeerReady=False`, and requeues so the next reconcile sees the persisted
annotation. On failure it writes a condition and an event and writes **no**
annotation, so the user can correct and retry.

```go
d.instance.Status.DryRun = &rmn.DryRunStatus{
    RunID:                  runID,
    TargetCluster:          target,
    State:                  rmn.DryRunInProgress,
    StartTime:              now(),
    ActionAtStart:          d.instance.Spec.Action,           // §5.4
    FailoverClusterAtStart: d.instance.Spec.FailoverCluster,  // §5.4
}
```

**`dryRunBuild`** — promote and wait:

```go
if yield, next := d.dryRunShouldYield(); yield {    // §5.4
    d.setDryRunState(next)
    return !done, nil
}

target := d.instance.Status.DryRun.TargetCluster

// Promote, forcing the dry-run shape on the VRG regardless of spec.action.
d.createVRGManifestWorkAsPrimary(target)   // with the overrides below
//   vrg.Spec.ReplicationState = Primary
//   vrg.Spec.Action           = rmn.VRGActionFailover   // decision 25
//   vrg.Spec.DryRun           = true
//   vrg.Annotations[dry-run]  = SettingUp

// Mirror the test cluster's progress into DRPC status.
vrg := d.reconciler.MCVGetter.GetVRGFromManagedCluster(...)
d.mirrorDryRunStatus(vrg)

if vrg.Status.DryRun == nil || vrg.Status.DryRun.State != Completed {
    return !done, nil                      // requeue; may stall forever
}

d.setDryRunState(rmn.DryRunReverting)
return !done, nil
```

**`dryRunRevert`** — demote and finish, reusing the existing cleanup
machinery rather than inventing a third secondary-readiness check:

```go
// homeCluster is unchanged throughout the dry run, so the only peer
// EnsureCleanup acts on is the test cluster.
if err := d.EnsureCleanup(homeCluster); err != nil {
    return !done, nil                      // still waiting for Secondary
}

d.setDryRunState(rmn.DryRunCompleted)
d.instance.Status.DryRun.CompletionTime = now()
return done, nil
```

`EnsureCleanup` (`drplacementcontrol.go:2321`) already does everything this
step needs: `cleanupSecondary` calls `updateVRGState(peer, Secondary)` —
which clears `vrg.Spec.DryRun` and so triggers `dryrun.HandOff` — waits for
the peer to report Secondary, and sets `PeerReady=True` itself once it has.
There is no `vrgIsSecondaryOnCluster` helper to write.

**Only `EnsureCleanup`, not its wrapper** (decision 32). The enclosing
`ensureCleanupAndSecondaryReplicationSetup` (`:1106`) is the wrong call here:
`ResetVolSyncRDOnPrimary` and `ResetVMStaticIPSpecOnPrimary` both write to the
**source** cluster's VRG, and `EnsureSecondaryReplicationSetup` creates VolSync
ReplicationDestinations on the peer. All three contradict decision 24, and the
last is moot anyway since VolSync workloads are refused.

Nothing in any of the three states calls `setDRState`, `ensurePlacement`,
`updateUserPlacementRule`, `ensureActionTransition` or
`retainClusterDecisionAsFailover`, and nothing addresses the source cluster
(decisions 23, 24).

#### 5.2.1 Which cluster is the test target?

Derived as the peer that is **not** currently Primary, from the DRPolicy's
cluster pair and the current home cluster (decision 28).
`spec.failoverCluster` is not consulted: requiring it would reintroduce exactly
the coupling decision 22 removes, and it goes stale — after a real failover to
C2 it still reads `C2`, which is now the Primary, so a dry run would try to
"test" the cluster already running the workload.

The resolved target is recorded in `status.dryRun.targetCluster` at
`dryRunStart` and read from there for the rest of the run, so the run cannot
retarget itself if cluster roles shift underneath it.

#### 5.2.2 Conditions during a dry run

- **`PeerReady`** → `False` for the duration, `True` again at `Completed`. This
  is factually correct — the peer is Primary and unavailable as a failover
  target — and it makes the existing action-gating logic protect the dry run
  without any new checks. Only the `False` needs writing: `EnsureCleanup`
  restores it to `True` as part of the revert.
- **`Available`** and **`Protected`** are **not** touched. They describe the
  real action and the real workload, neither of which changes.
- Dry-run progress and any stall reason go to `status.dryRun` (§5.9), never to
  `status.progression`, which belongs to the real action.

### 5.3 The dry-run state annotation

```
drplacementcontrol.ramendr.openshift.io/dry-run:
    SettingUp | Reverting | Completed | TearingDown
```

| Value       | Meaning                                                     |
| ----------- | ----------------------------------------------------------- |
| *(absent)*  | no dry run in progress; a new one may begin                  |
| `SettingUp` | test cluster promoted, test copy being built — or stalled    |
| `Reverting` | test copy ready or abandoned; peer returning to Secondary    |
| `Completed`   | cycle finished; test copy running, waiting on the user     |
| `TearingDown` | the user cleared the flag; the copy is being deleted       |

```
  absent ──user: dryRun=true, pre-flight passes──► SettingUp
                                                      │ │
                      VRG status.dryRun Completed     │ │ stalled: stays
                      or pre-emption (§5.4)           │ │ here, logging
                                                      ▼ │ and requeuing
                                                  Reverting
                                                      │
                      target VRG confirmed Secondary  │
                                                      ▼
                                                  Completed
                                                      │
                      user: delete annotation         │
                            delete test namespaces    │
                                                      ▼
                                                   absent
```

Only Ramen writes this annotation, and it never clears it — leaving the
terminal state is a deliberate user action, and it is the only gate on a second
run (decision 8).

The annotation is propagated to the test VRG so the managed cluster can
distinguish a dry run from a real failover (`autoResync`, `vrg_volrep.go:1451`,
already depends on this). **This propagation is currently broken — see §7.**

`ProgressionTestingFailover` is no longer set; the constant remains in
`api/v1alpha1` marked deprecated.

#### 5.3.1 Tearing the copy down — [from implementation]

Earlier drafts left the test copy to the user: Ramen never cleared the
annotation, and deleting the namespaces was a manual step the release notes
had to warn about. That put two chores on the user after every run, and made
the repeat-run hazard of §12 something they had to remember rather than
something the system prevented.

Clearing `spec.dryRun` now means "I am finished with the copy". From
`Completed` that moves the run to `TearingDown`; the test cluster deletes the
namespaces, with the restored workload, its PVCs and its snapshots inside
them; and Ramen removes its own annotation once they are gone, which is what
frees the DRPC to run another test.

So the flag reads the same at both ends: setting it asks for a copy, clearing
it gives the copy back. Nothing is left for the user to tidy, and a second run
cannot start over the first one's leftovers, because the leftovers go before
the annotation does.

Three details that are not obvious:

- **The test cluster does the deleting.** The namespaces are there, not on the
  hub. It reports being finished by emptying its own `status.dryRun`, the same
  field that says a copy exists.
- **The phase has to be pushed to a Secondary VRG.** By tear-down time the
  test cluster has been demoted, and neither `ensureVRGManifestWork` (which
  refuses a VRG that is not Primary) nor `updateVRGState` (which returns early
  when the state already matches) will carry an annotation change there.
  `refreshVRGAnnotations` exists for that one job.
- **A promotion is not a tear-down.** Handing over to a real failover (§5.9)
  clears the annotation outright rather than finishing into `Completed`, which
  would read as a delete request the moment the cleared flag was seen. The copy
  keeps running alongside the now real workload.

### 5.4 Interaction with real DR actions and deletion

A dry run holds the peer Primary, so it has to get out of the way when
something real needs to happen. All four cases below are handled inside
`RunDryRunFailover`, which stays in control until the annotation is terminal.

**Detecting a real request.** `dryRunStart` records `spec.action` and
`spec.failoverCluster` into `status.dryRun`. `dryRunShouldYield` compares them
against the live spec on every reconcile; a change in either is a real request.
Both are watched because `spec.action` alone is not enough — a DRPC already
holding `action: Failover` from an earlier real failover would show no change
when the user retargets `failoverCluster` at a genuine disaster.

- **`dryRun` set while the DRPC is mid-action** — refused by pre-flight, §5.10
- **Real action requested mid-run** — **pre-empt** → `Reverting`
- **`dryRun` cleared mid-run, action wants a failover to the test cluster** —
  **hand over** to `RunFailover`
- **`dryRun` cleared mid-run, anything else** — **abort** → `Reverting`
- **Real action requested after `Completed`** — runs normally, via
  `dryRunFinished()`
- **DRPC deleted mid-run** — the existing finalizer drains both VRGs;
  `HandOff` runs first

**Pre-emption** (decision 30). A real action requested while the dry run is
building jumps straight to `Reverting`, abandoning a half-built test copy in
order to free the peer as quickly as possible. `updateVRGState(target,
Secondary)` still clears `DryRun`, so `HandOff` still runs and whatever test
namespaces and volumes exist survive as debris. Once `Completed` is reached,
`dryRunFinished()` lets the real action through on the next reconcile.

**Hand over to a real failover** (decision 31). If the user clears
`spec.dryRun` and `spec.action` is `Failover` resolving to the cluster the dry
run promoted, demoting and re-promoting would be pure waste in what is probably
an emergency. Instead the dry-run path finishes immediately: set the annotation
to `Completed`, clear `DryRun` on the test VRG so `HandOff` transfers the
snapshots, and let `executeAction` fall through. `RunFailover`'s first branch
is `vrgExistsAndPrimary(failoverCluster)` (`drplacementcontrol.go:455`), which
is already true, so it proceeds directly to `ensureActionTransition` — cleaning
up the source cluster and moving Placement as a real failover should. The
in-place kube object restore runs for the first time, because the dry run never
set `KubeObjectsReady` (§5.10). The test copy keeps running alongside.

**No second pre-emption path** (decision 33). Yielding only ever happens from
`SettingUp`. A real action arriving once the run is already `Reverting` simply
waits: the peer is being freed as fast as it can be, there is nothing to
accelerate, and `dryRunFinished()` lets the action through the moment
`Completed` is reached.

**Abort.** If `spec.dryRun` is cleared with any other action — empty, or
`Relocate` — the dry run goes to `Reverting` rather than being abandoned. This
matters: without it, `executeAction` would stop calling `RunDryRunFailover` the
instant the flag cleared, stranding the peer Primary with the annotation at
`SettingUp`. A `Relocate` requested here is additionally blocked by
`PeerReady=False` until the revert completes, which is the correct ordering —
the peer must be Secondary before anything can relocate to it.

**DRPC deleted mid-run** (decision 17, revised). No new finalizer:
`finalizeDRPC` already deletes Secondary VRGs then Primary VRGs and holds
`DRPCFinalizer` until none remain (§4.4). During a dry run both clusters are
Primary, so the Secondary pass is a no-op and the Primary pass drains both —
the peer cannot be stranded.

What *is* needed is that the test cluster's VRG hands the snapshots over before
it disappears. `processForDeletion`
(`volumereplicationgroup_controller.go:1352`) gains a `dryrun.HandOff` call
alongside the existing `kubeObjectsProtectionDelete` (`:1392`), guarded on the
dry-run annotation. Without it the source `VolumeSnapshotContent` objects are
deleted still carrying `deletionPolicy: Delete`, taking the storage snapshots
with them and leaving the test PVCs' backing volumes dependent on CSI clone
semantics rather than on a snapshot Ramen deliberately retained.

The test namespaces themselves survive, possibly half-built. That remains the
user's to clean up.

### 5.5 Run identity and target namespace naming

```
runID      = hex(sha256(drpcUID))[:8]
target(ns) = truncate(ns, 63-6-len(runID)) + "-test-" + runID
```

```
workload1 → workload1-test-78de2a4e
workload2 → workload2-test-78de2a4e
```

One shared suffix for all namespaces in a run (decision 4), derived from the
DRPC UID (decision 20). The hub has the UID directly and the managed cluster
already receives it as `DRPCUIDAnnotation` (`drplacementcontrol.go:1990`), so
nothing extra is carried and nothing is lost if status is lost.

Every run on a given DRPC therefore targets the same namespace names. Ramen
creates what is missing, adopts what is present, and carries on. §12 records
what that produces on a repeat run.

Namespace names are capped at 63 characters, so the source name is truncated
before the suffix is appended. Truncation can collide — two long source names
sharing a prefix map to one target. The hub pre-flight refuses rather than
silently merging two namespaces into one.

**Source namespace set.** Always `*vrg.Spec.ProtectedNamespaces`; the scope is
Discovered Apps, so it is always populated.

### 5.6 Materialising the test namespace

Created by the dry-run package before any PVC — or adopted if it already
exists — labelled:

```yaml
ramendr.openshift.io/dry-run-namespace-of: <sourceNamespace>
ramendr.openshift.io/dry-run-id:           <runID>
ramendr.openshift.io/dry-run-drpc-uid:     <drpcUID>
```

Identification only. No `ownerReference` to the VRG, and no VRG cleanup path
selects on these labels — the namespace must outlive both the VRG and the DRPC.

Copied from the source namespace so the test workload behaves like the real
one:

| Object                         | Copied | Notes                             |
| ------------------------------ | ------ | --------------------------------- |
| `pod-security.kubernetes.io/*` | yes    | namespace labels; without them a  |
|                                |        | restricted workload is rejected   |
| `NetworkPolicy`                | yes    |                                   |
| `ResourceQuota`                | yes    |                                   |
| `LimitRange`                   | yes    |                                   |
| `Role`, `RoleBinding`          | yes    | subjects naming the source        |
|                                |        | namespace are rewritten           |
| `openshift.io/sa.scc.*`        | no     | cluster-assigned; copying collides |
| `ServiceAccount`               | no     | arrives in the Velero backup      |

Everything else arrives from the Velero restore.

### 5.7 Volume hand-off: pre-provisioned VolumeSnapshotContent

A PVC in `workload1-test-ID` cannot reference a `VolumeSnapshot` in
`workload1` — `VolumeSnapshot` is namespaced and `PVC.spec.dataSource` is
namespace-local. Cross-namespace data sources (KEP-3294) are alpha and require
`ReferenceGrant`, so they are not an option.

`VolumeSnapshotContent` is **cluster-scoped** and its `volumeSnapshotRef` is a
full `ObjectReference`, so it can point at a `VolumeSnapshot` in any namespace.
Import the existing storage snapshot into the test namespace as a
pre-provisioned snapshot, then provision the test PVC from it normally.

Per protected PVC, once its dry-run snapshot is `ReadyToUse`:

```
 workload1                      cluster-scoped           workload1-test-ID
 ─────────                      ──────────────           ─────────────────
 VolumeSnapshot                 VolumeSnapshotContent
 db-snapshot ────── bound ────► snapcontent-abc
                                deletionPolicy: Retain ◄── ① patched
                                snapshotHandle: H
                                       │
                                       │ ② read H, driver,
                                       │   sourceVolumeMode
                                       ▼
                                VolumeSnapshotContent      ③ created
                                dryrun-<runID>-db
                                deletionPolicy: Delete
                                source.snapshotHandle: H
                                volumeSnapshotRef: ───────► VolumeSnapshot
                                  workload1-test-ID/         db-snapshot  ④
                                  db-snapshot                    │
                                                                 │ dataSource
                                                                 ▼
                                                            PVC  db       ⑤
```

1. Patch the **source** VSC to `deletionPolicy: Retain`, so deleting the source
   `VolumeSnapshot` later does not delete the underlying storage snapshot.
2. Read `snapshotHandle`, `driver` and `sourceVolumeMode` from the source VSC.
3. Create a cluster-scoped VSC with `deletionPolicy: Delete`,
   `source.snapshotHandle` set to the same handle, and `volumeSnapshotRef`
   pointing into the test namespace.
4. Create a `VolumeSnapshot` in the test namespace whose
   `source.volumeSnapshotContentName` is that VSC.
5. Create the test PVC with the **same name as the source PVC** — essential, so
   restored Pod and VirtualMachine specs resolve their claims — with
   `dataSource` referencing the snapshot, and `storageClassName`,
   `accessModes` and `resources.requests` copied from the source PVC.

Ownership of the storage snapshot transfers with the deletion policies: the
source side becomes `Retain` (gives it up), the test side is `Delete` (takes
it). When the user eventually deletes the test namespace, the storage snapshot
is reclaimed with it.

Step ① must happen **before** step ③. In the window where two VSCs reference
one handle, the source side must already be `Retain`, so that whichever is
removed first the storage snapshot survives for the other.

Steps ③–⑤ treat `AlreadyExists` as success. On a first run that is simple
idempotency; on a repeat run over uncleaned namespaces it is what makes
decision 20 land where §12 describes.

The gate before §2 step ⑦ is: every test PVC `Bound`. A PVC that never binds
stalls the run here, indefinitely, per decision 21.

**Why not hand off the PV instead?** Provisioning a PVC from the snapshot in
the source namespace, setting its PV to `Retain`, deleting the PVC and
rewriting the PV's `claimRef` into the test namespace reuses a pattern Ramen
already has (`cleanupPVForRestore`, `vrg_volrep.go:2944`). Rejected: the
intermediate PVC lives, however briefly, in a protected namespace, where the
VRG's own PVC selector would match it and create a `VolumeReplication` for it.
The VSC import never touches the protected namespace.

### 5.8 Kube object restore

The dry-run package builds its own `kubeobjects.RecoverSpec` values and calls
`RequestsManager.RecoverRequestCreate` directly. It does not go through
`executeRecoverSteps` / `executeRecoverGroup`, so the production restore path
is untouched and there is no name or label collision to design around.

For each `group` step of the recipe's restore workflow:

```go
kubeobjects.RecoverSpec{
    BackupName:       group.BackupName,
    Spec:             group.Spec,          // unchanged source namespaces
    NamespaceMapping: mapping,             // workload1 → workload1-test-ID
    ResourceModifier: ownerRefStripCM,     // §5.8.1
}
```

The `IncludedNamespaces` inside `Spec` stay as the **source** namespaces —
those are the keys Velero matches the mapping against. The capture side and the
S3 layout are untouched.

Requests are named `dryrun-<vrgName>-<runID>-<groupNumber>` and labelled
`ramendr.openshift.io/dry-run-id: <runID>`, so the dry-run package's
`RecoverRequestsGet`/`RecoverRequestsDelete` can never see or delete the
production flow's requests.

A Velero restore reporting `Failed` does not fail the run — it stalls it with
the restore named in the log and in `status.dryRun.message` (decision 13).

**Hooks are skipped.** Only `group` steps are executed; `hook` steps are
ignored. Hooks are run by Ramen rather than Velero, so an unremapped hook would
act on the production namespace. The built-in vm-recipe has no restore hooks.

**Static IP translation is not applied.** The production restore injects
`StaticIPTranslationSpec` so VMs come up on their configured addresses. Doing
the same for a test copy would make the test VMs claim the **same IPs** as the
production VMs still running on the source cluster. Test VMs get DHCP
addresses. `ensureResourceModifierCM` and `vrg_staticip.go` are untouched.

#### 5.8.1 Stripping ownerReferences

Restored objects keep the owner references recorded in the backup. In a new
namespace those owner UIDs do not resolve and the garbage collector deletes the
restored objects. Velero has no built-in strip option, but one wildcard
resource modifier does it:

```yaml
version: v1
resourceModifierRules:
  - conditions:
      groupResource: "*"
    mergePatches:
      - patchData: '{"metadata":{"ownerReferences":null}}'
```

A JSON **merge** patch rather than a JSON patch `remove`, because `remove`
fails when the path is absent and most objects carry no owner references.

The dry-run package owns this ConfigMap outright — name
`dryrun-<vrgName>-<runID>-resource-modifier`, in the Velero namespace. Because
static IP translation is not applied, this is the only rule in it, so Velero's
one-data-key constraint is satisfied with no merging.

### 5.9 Reporting, stalling, and what survives

The VRG publishes what it did, and the DRPC mirrors it:

```go
// VRGStatus and DRPCStatus
//+optional
DryRun *DryRunStatus `json:"dryRun,omitempty"`

type DryRunStatus struct {
    RunID            string            `json:"runID,omitempty"`
    TargetCluster    string            `json:"targetCluster,omitempty"`
    NamespaceMapping map[string]string `json:"namespaceMapping,omitempty"`
    State            DryRunState       `json:"state,omitempty"`
    StartTime        *metav1.Time      `json:"startTime,omitempty"`
    CompletionTime   *metav1.Time      `json:"completionTime,omitempty"`
    Message          string            `json:"message,omitempty"`

    // DRPC only — the spec the run started against, for pre-emption (§5.4)
    ActionAtStart          DRAction `json:"actionAtStart,omitempty"`
    FailoverClusterAtStart string   `json:"failoverClusterAtStart,omitempty"`
}
// State: InProgress | Completed
```

`Completed` on the VRG requires all three gates: every test namespace
materialised, every test PVC `Bound`, every Velero restore complete.

**There is no failure path.** Any condition the dry-run package cannot satisfy
— an ineligible workload, an unbound PVC, a failed Velero restore — leaves
`State: InProgress`, writes the reason to the log, sets it in `Message`, and
requeues. The run waits for a human indefinitely (decisions 13, 21).

Because the dr-cluster operator's log is not somewhere a DRPC user looks, the
DRPC mirrors `status.dryRun` — including `Message` — so the stall reason is
reachable from the hub. Logging is the primary channel; status is the mirror.

What survives the revert:

| Object                            | On revert                     |
| --------------------------------- | ----------------------------- |
| Real PVCs/PVs in protected ns     | **deleted by the dry run**, §5.7.1 |
| VolumeReplications                | demoted                       |
| Source-namespace `VolumeSnapshot` | deleted                       |
| Source-namespace VSC              | object deleted, snapshot kept |
| Dry-run Velero Restore/Backup CRs | deleted                       |
| Imported VSC                      | **kept**                      |
| Test-namespace `VolumeSnapshot`   | **kept**                      |
| Test PVC and its PV               | **kept**                      |
| Test namespace and restored app   | **kept**                      |
| Everything on the source cluster  | **untouched throughout**      |

`cleanupDryRunSnapshots` (`vrg_volrep.go:3874`) currently deletes the storage
snapshots outright, which would destroy the test volumes. It is replaced by
`dryrun.HandOff`, which:

1. patches each source VSC to `Retain`,
2. deletes the source `VolumeSnapshot`,
3. deletes the orphaned source VSC object,
4. deletes the dry-run Velero `Restore` and dummy `Backup` CRs by the
   `dry-run-id` label — these carry the dry-run labels, so the VRG's own
   `kubeObjectsProtectionDelete` would otherwise leave them as garbage.

Each step tolerates the object being absent, because a run pre-empted or
aborted mid-build may have completed only some of them. `HandOff` runs on
revert, on promotion (§5.4) and from `processForDeletion`.

#### 5.7.1 Discarding the PVCs step ① restored — [from implementation]

Earlier drafts said the revert removed the real PVCs from the protected
namespaces "exactly as today". That was wrong, and it hung the first run on a
real cluster: DRPC parked in `WaitOnUserToCleanUp`, VRG `Unknown`, refusing to
demote.

Demoting waits for each PVC to be marked for deletion
(`isPVCReadyForSecondary`, `vrg_volrep.go:316`). For a discovered application
"as today" means waiting for the **user** to remove the workload. On a test
cluster there is no workload to remove: Ramen restored those PVCs itself in
step ①, from S3, purely so their volumes could be promoted and snapshotted. So
the wait never ended.

The revert therefore deletes them, in `dryRunDiscardRestoredPVCs`, before the
Secondary path runs. Only the PVCs in the protected namespaces — the test
copy's own live in the generated namespaces, are not selected by the VRG, and
keep running.

**The guard is `status.dryRun`, never the annotation.** `setVRGAnnotations`
copies the dry-run annotation onto *every* VRG the DRPC manages, including the
one running the real workload; only the cluster that actually built a test
copy writes the status. Gating this on the annotation would delete a running
application's volumes on its next demote.

**Promotion keeps them.** Clearing the flag with a failover to this cluster
(§5.9) turns them from a test copy's scaffolding into the workload's own
volumes, so the discard is skipped when the VRG is staying Primary.

The VRG *deletion* path deliberately does not discard them. Deleting a DRPC
normally preserves PVCs, and whether dry-run leftovers should be an exception
is a separate decision — see §13.

#### 5.7.2 Auto-resync outlives the run — [from implementation]

A test failover promotes the test cluster's volumes in order to snapshot
them, and that divergence is what makes a resync necessary afterwards. The
need outlives the run that caused it.

`autoResync` therefore treats **any** dry-run phase as grounds to enable it,
the terminal one included. The cluster only reaches `Completed` once it is
already Secondary -- the hub sets that phase after `EnsureCleanup` confirms
the demote -- so treating `Completed` as "over" switched auto-resync off at
the exact moment it was wanted, leaving the volumes diverged.

This guard is deliberately looser than §5.7.1's. That one has to know which
cluster built the test copy, and keys on `status.dryRun` for it. This one
only asks whether these volumes were promoted by a test, and the annotation
answers that even for a run that never got far enough to write status. Being
wrong permissively costs a resync that was not needed; being wrong the other
way leaves volumes diverged, so the two guards should not be unified.

### 5.10 Eligibility and pre-flight guards

**On the hub, in `dryRunStart`, before any annotation is written** — each
failing with a condition and an event, leaving the DRPC untouched:

- **Idle.** `status.phase` must be a steady state (`Deployed`, `FailedOver`,
  `Relocated`), `status.progression` must be `Completed`, and `PeerReady` must
  be `True`. Starting a dry run on top of an in-flight action would hijack it.
- **Discovered app.** `isDiscoveredApp(drpc)` must be true.
- **Async DR.** `d.drType` must not be `DRTypeSync` (decision 26). A Metro test
  failover would exercise the fencing path against a live peer.
- **No VolSync.** Refuse if any `ProtectedPVC.ProtectedByVolSync` is true in
  the current primary's VRG status.
- **Annotation absent.** Refuse if the dry-run annotation is present in any
  state, including `Completed`. This is the only gate on repeat runs.
- **Target resolvable.** A peer that is not the current Primary must exist.
- **No truncation collision.** Refuse if two protected namespaces map to the
  same target name (§5.5).
- **Protected and in sync.** The existing `dryRunReadyToFailover` check —
  `isProtected() && !isGroupSyncLagging()` — moves into this list.

**On the test cluster, in `dryrun.Reconcile` before any object is created** —
stalling rather than failing:

- **No CephFS.** Stall if any protected PVC resolves to a CephFS provisioner
  (`isCephFSProvisioner`). This cannot be checked on the hub, which sees only
  the StorageClass *name*.

There is deliberately **no leftover-namespace check** (decision 20).

A partial test copy was considered and rejected for VolSync and CephFS alike: a
workload whose data is split across paths would come up with some volumes
missing, which is worse than a clear refusal.

**Invariants, not user errors:**

- Test namespaces are never protected — absent from `ProtectedNamespaces` and
  from `recipeElements.PvcSelector.NamespaceNames`, so `updatePVCList` never
  sees the test PVCs and no `VolumeReplication` is created for them. This also
  keeps `recipeNamespacesValidate` (`vrg_recipe.go:292`) untouched.
- `VRGConditionTypeKubeObjectsReady` is **not** set by a dry run, so a later
  real failover still performs its own restore (`shouldRestoreKubeObjects`,
  `volumereplicationgroup_controller.go:1626`).

## 6. API and RBAC Changes

**VRG status** — `DryRunStatus` (§5.9). No new VRG *spec* field.

**DRPC status** — `DryRunStatus`, mirrored from the VRG plus `TargetCluster`
and the two at-start fields used for pre-emption.

**Annotation** — `drplacementcontrol.ramendr.openshift.io/dry-run`, replacing
the current `.../test-failover-dryrun`.

**RBAC markers** on the VRG controller:

```go
//nolint:lll
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=resourcequotas;limitranges,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;escalate;bind
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshotcontents,verbs=get;list;watch;create;update;patch;delete
```

`escalate` and `bind` are required to copy `Role`/`RoleBinding` objects whose
rules exceed the operator's own permissions.

## 7. Prerequisite: Annotation Propagation — Resolved Upstream

Earlier iterations of this document recorded a bug here: `setVRGAnnotations`
built the VRG annotation map with the dry-run annotation's *value* as the map
key rather than its name, so the annotation never reached the VRG. This design
depends on that propagation working.

**It is fixed as of `650b3d0d`.** `drplacementcontrol.go:1995` now reads:

```go
//nolint:lll
DRPCTestFailoverDryRunAnnotation: d.instance.GetAnnotations()[DRPCTestFailoverDryRunAnnotation],
```

Nothing is owed here. Phase 0 is correspondingly smaller. The same mechanism
carries the new three-state annotation (§5.3), so the only change needed is
swapping the constant.

## 8. DRPC Code Removed by the Refactor

Decision 27. Everything below is deleted or moved into
`drplacementcontrol_dryrun.go`; none of it survives in its current form.

- `processTestFailoverFlowIfEnabled` (`:149`) — deleted, replaced by
  `dryRunStart`.
- dryRun branch in `RunFailover` (`:479-489`) — deleted; `RunFailover` loses
  all dry-run awareness.
- `dryRunReadyToFailover` (`:530`) — moves into the pre-flight list.
- revert detection in `RunInitialDeployment` (`:231`) — deleted; the revert is
  internal now.
- `cleanupTestFailoverAnnotation` — deleted.
- dryRun special-case in `ensurePlacement` (`:2054`) — deleted; never called
  on the dry-run path.
- dryRun logic in `setVRGAction` (`:2090-2106`) — deleted; the dry-run path
  sets the VRG shape explicitly (decision 25).
- commented-out block in `updateVRGState` (`:2612`) — deleted.
- `isFailoverPromotion` dryRun term (`:2630`) — simplified; a dry run no
  longer uses `spec.action`.
- the `// FIXME` block in `cleanupSecondaries` (`:2372`) that deletes the
  dry-run annotation once cleanup completes — deleted. Under this design the
  annotation is terminal and only the user clears it (decision 8), so
  `EnsureCleanup` must leave it alone. This matters because `dryRunRevert`
  now calls straight into that path.

The net effect is that `RunFailover`, `RunRelocate` and `RunInitialDeployment`
contain no dry-run conditionals at all.

### 8.1 Annotations

Decision 35. The new architecture needs exactly one annotation — the
three-state `dry-run` of §5.3. Everything the old flow leaned on either goes
or loses its dry-run role. The two categories are not interchangeable: one of
these annotations is load-bearing for real DR and must survive.

**Deleted outright — no remaining reader:**

- `DRPCTestFailoverDryRunAnnotation` (`:46`) and
  `DRPCTestFailoverDryRunAnnotationValueTrue` (`:49`) — superseded by the
  three-state annotation. All nine uses go. Note that `autoResync`
  (`vrg_volrep.go:1451`) is one of them: it must be repointed at the new
  annotation, and its `== "true"` comparison becomes "present and not
  `Completed`".
- `DRPCLastAction` (`:52`) — written in `ensurePlacement` (`:2056`) and read
  in exactly one place, the log line at `:177` inside
  `processTestFailoverFlowIfEnabled`, which is itself deleted. It is
  write-only once the dry-run code goes, so the constant, the write and the
  surrounding `if !d.instance.Spec.DryRun` guard (`:2054-2057`) all go
  together. Nothing in-tree reads it — but it is an annotation on a
  user-facing CR, so its removal belongs in the release notes in case
  external tooling does.
- The `spec.Action == ""` branch in `isSourceVRGForAction` (`:3197-3204`).
  Added for "after dryRun revert with last-action=''", a state the new design
  never produces because the dry run never clears `spec.action`. Its only
  caller is `getCleanupSecondaryVRG` (`:3160`), which uses the result to
  decide whether to inspect the *alternate* cluster's VRG when tracking
  discovered-app auto-cleanup. Removing the branch makes an empty action fall
  through to `return false`, so `getCleanupSecondaryVRG` reads the named
  cluster's own VRG — which is what it did before the dry-run work added
  this. `isSourceVRGForAction` is then a pure function of `spec.Action` and
  the two cluster fields, reading no annotations at all.

**Kept — load-bearing for real DR:**

- `LastAppDeploymentCluster` (`:40`) **must stay.** It is read functionally
  at `drplacementcontrol_controller.go:1832` to resolve the home cluster when
  no cluster decision is available, which has nothing to do with dry runs.
  What goes is only its dry-run usage: the read at `:161`, inside the deleted
  `processTestFailoverFlowIfEnabled`, and the
  `failoverCluster != lastAppCluster` validation it fed — obsolete now that
  the test target is derived rather than supplied (decision 28). The write at
  `:2050-2051` stays.

## 9. Implementation Plan

**Phase 0 — Prerequisite.** Export the two S3 naming helpers into
`internal/controller/kubeobjects` and repoint the production path at them
(§5.1). The annotation propagation bug that used to live here was fixed
upstream — see §7.

**Phase 1 — DRPC refactor.** Create `drplacementcontrol_dryrun.go`, move and
delete per §8 and §8.1, add the `executeAction` branch and the three-state
machine with `dryRunBuild`/`dryRunRevert` stubbed to no-ops. Existing failover
and relocate behaviour must be unchanged — verifiable by the existing DRPC
test suite passing untouched, *excluding* `drplacementcontrol_dryrun_test.go`,
which is rewritten rather than adjusted: it asserts on the old annotation and
on the `failoverCluster != lastAppCluster` validation that decision 28
removes.

**Phase 2 — Promote and demote.** Fill in `dryRunBuild` and `dryRunRevert`
against `createVRGManifestWorkAsPrimary` / `updateVRGState`, with the decision
25 overrides, the pre-flight guards, `PeerReady` handling, and the yield logic
of §5.4. At the end of this phase a dry run promotes the peer, does nothing
useful there, and demotes it again — and pre-emption, hand-over and abort all
work.

**Phase 3 — Managed-cluster package skeleton.** `internal/controller/dryrun`
with `Input`/`Output`, run-ID derivation, naming, the CephFS check, and the
adapter `if` in `processAsPrimary`.

**Phase 4 — Namespace materialisation.** Namespace creation, policy copy, RBAC
markers.

**Phase 5 — Volume hand-off.** VSC import and test PVC creation; `HandOff`
replacing `cleanupDryRunSnapshots` on the revert, promote and delete paths.
Highest storage-specific risk — prototype against real Ceph RBD first.

**Phase 6 — Kube object restore.** Velero restore into the test namespaces and
the ownerReferences-strip ConfigMap.

**Phase 7 — Reporting.** `DryRunStatus` on both CRs, mirroring, stall messages.

**Phase 8 — e2e.** Two-namespace discovered app: full cycle, pre-emption,
hand-over, stalled-run abort, DRPC deletion mid-run, and a repeat run over
uncleaned namespaces to confirm §12.

## 10. Alternatives Considered

**`dryRun` as a modifier on `action: Failover`.** The shape of iterations 1–8
and of the current code. Rejected: it conflates a test with a real DR action,
forces the user to set an action they do not want performed, leaves
`spec.action` holding a value that must be remembered and restored, and
scatters `if spec.DryRun` through `RunFailover`, `ensurePlacement`,
`setVRGAction` and `RunInitialDeployment`. Making it a mode removes all of
that and, as a side effect, makes the automatic revert possible without Ramen
ever writing `spec`.

**Cross-namespace volume data sources (KEP-3294).** Would make §5.7 a two-line
change. Rejected: alpha, feature-gated, requires `ReferenceGrant`.

**PV `claimRef` hand-off.** Rejected in §5.7 — the intermediate PVC would be
matched by the VRG's own PVC selector.

**Reusing `executeRecoverSteps` with an injected namespace remapper.** Rejected
in favour of the separate package: it would require restore-identity
discrimination, a conditional merge of two resource-modifier ConfigMaps, and
conditional hook remapping, all in the path real failovers depend on.

**A `Failed` state and a timeout.** Rejected (decisions 13, 21): a stalled run
is usually a fixable condition, and failing it automatically would destroy the
evidence. Stalling preserves state for diagnosis. The cost is in §12.

**A per-run `runID`.** Would make leftover detection a one-line label
comparison. Rejected (decision 20) in favour of carrying no extra annotation.
The cost is in §12.

**A dedicated finalizer for in-flight dry runs.** Considered for the
DRPC-deleted-mid-run case and rejected once `finalizeDRPC` was found already to
drain both Primary VRGs (§4.4). A `HandOff` call in `processForDeletion` covers
the remaining gap at a fraction of the complexity.

**Demoting volumes immediately after the hand-off.** Would shrink the RPO
window further, per Solution 1 in `test-failover-rpo-mitigation.md`. Deferred
(decision 19): it means a VRG that is `Primary` by spec while its volumes are
Secondary. The window is now bounded by Ramen's own progress rather than by a
human, which makes this much less pressing.

**A standalone `KubeObjectRestore` CR.** Cleaner separation still, and it would
enable ad-hoc restores with no DR action in flight. Deferred: new controller,
new hub plumbing. The `dryrun` package is a stepping stone.

## 11. Testing

- **Unit** — the three-state machine including every pre-flight refusal and
  all four yield cases of §5.4; target-cluster derivation; run-ID derivation;
  truncation collision; `RecoverSpec` carries the expected mapping; the
  resource-modifier ConfigMap parses under Velero's own
  `GetResourceModifiersFromConfig`.
- **Regression** — the existing DRPC suite must pass unchanged after Phase 1,
  proving `RunFailover`/`RunRelocate`/`RunInitialDeployment` lost nothing.
- **Envtest** — a dry-run Primary VRG materialises the expected namespaces,
  creates the imported VSCs and test PVCs, and produces `velero.Restore`
  objects with the expected mapping and labels; the in-place restore does not
  run; a CephFS workload stalls with the reason in `status.dryRun.message`;
  `processForDeletion` runs `HandOff` before the VRG goes away.
- **drenv / e2e** — two-cluster regional-DR environment, two-namespace
  discovered app: both test namespaces appear, each PVC lands in the matching
  test namespace, the application runs, the cluster returns to Secondary
  automatically, resync resumes, the test copy is still running, **the source
  cluster and the user Placement were never touched**, and `spec.action` is
  unchanged throughout. Separate cases for pre-emption, hand-over to a real
  failover, aborting a stalled run, deleting the DRPC mid-run, and a repeat run
  over uncleaned namespaces (§12).
- **Manual, before Phase 6 merges** — confirm on real Ceph RBD that an imported
  VSC provisions a usable PVC, and that deleting the source snapshot with
  `Retain` does not disturb it.

## 12. Known Limitations

- **A repeat run over uncleaned namespaces would silently reuse the old test
  copy** — but it can no longer arise in the normal flow. Every create in the
  chain treats `AlreadyExists` as success and Velero skips existing objects
  (§4.1), so a second run over a first one's namespaces would adopt them and
  report `Completed` over stale data. Decision 38 removes the opening: the
  namespaces are deleted before the annotation is, and the annotation gates
  the next run. It remains reachable only by deleting the annotation by hand
  while leaving the namespaces, which nothing in the documented flow asks
  anyone to do.
- **A stalled run holds the test cluster promoted indefinitely.** With no
  timeout and no failure state (decisions 13, 21), a dry run that cannot
  complete keeps the peer Primary and mirroring stopped until a human notices.
  This is why the stall reason must be visible in DRPC status.
- **Mirroring is stopped for the duration of the build** (decision 19),
  bounded by Ramen's own progress rather than by a human. The expected window
  is **about five minutes** (decision 34): promote the VRG, restore and
  promote the PVCs, restore the kube objects, then demote and return the VRG
  to Secondary. That budget is what makes the two-Primary window acceptable —
  but it is an expectation, not a limit, because there is no timeout
  (decision 21). A stalled run exceeds it indefinitely.
- **A pre-empted run leaves a half-built test copy** (decision 30) that counts
  as a leftover for the purposes of the limitation above.
- **Cluster-scoped resources are restored unmapped.** `includeClusterResources`
  stays `true` (decision 5), but Velero's `namespaceMapping` applies only to
  namespaced objects. A `ClusterRoleBinding` backed up with a subject in
  `workload1` restores pointing at `workload1`, and cluster-scoped objects
  restored by a test run still exist at a later real failover — Velero skips
  them as already present.
- **Test VMs do not get their configured static IPs** (decision 12); they fall
  back to DHCP.
- **Restore hooks do not run** (decision 11), so an application depending on
  one comes up unquiesced in the test namespace.
- **VolSync, CephFS and Sync/Metro workloads cannot be tested at all**
  (decisions 14, 16, 26).
- **Storage consumption roughly doubles** until the user deletes the test
  namespace.
- **Routes and Ingresses collide on hostname** with the originals.
- **Embedded namespace references are not rewritten.** See §1.
- **The real PVCs briefly exist on the test cluster** in the protected
  namespaces, promoted, so they can be snapshotted. The revert deletes them
  (§5.7.1). The application itself is never restored there.
- **A DRPC deleted while test leftovers exist keeps them.** The VRG deletion
  path does not discard the PVCs step ① restored, so they stay with the test
  namespaces for manual cleanup — see §13.

## 13. Implementation Notes

No open design questions. These are the details most likely to be got wrong.

1. **Reuse `EnsureCleanup`, not its wrapper.** §5.2 and decision 32. The
   wrapper writes to the source cluster, which decision 24 forbids.
2. **Delete the annotation cleanup in `cleanupSecondaries`.** §8.
   `EnsureCleanup` removes the dry-run annotation when cleanup finishes; under
   this
   design that annotation is terminal and only the user clears it. Because
   `dryRunRevert` calls straight into that path, leaving the block in place
   would silently let a second dry run start the moment the first reverted,
   defeating decision 8.
3. **Instrument the run duration.** `status.dryRun.StartTime` and
   `CompletionTime` already exist; surface the elapsed time so an operator can
   see when a run has blown through the five-minute budget of decision 34.
   With no timeout, this is the only signal that the two-Primary window has
   stopped being short.
4. **Audit the hub for *corrective* action on two Primaries.** The hub sees two
   Primary VRGs for the whole build window, which decision 34 accepts. The
   check to make in Phase 2 is narrower than "does anything notice": confirm
   that no hub path *acts* on the observation — fencing, forcing a demote, or
   flagging a conflict the user must resolve. Logging it is fine.
   `checkPVClusterData` (`vrg_volrep.go:2617`) documents this exact shape as a
   split-brain signal, so it is the natural place to start.
5. **`dryRunStart` must set `PeerReady=False` before anything else.**
   `EnsureCleanup` early-returns when `PeerReady` is already `True` for the
   current generation, so a revert that runs without the flag having been
   lowered first would declare success without demoting anything.

## 14. What The First Run Taught Us — [from implementation]

Three defects reached a cluster before anything caught them. Two share a root
cause worth naming, because the design did not anticipate it and the unit
tests could not have.

**Assuming an API call's return value reflects server state.** Twice.
`Update` on the DRPC decodes the stored object back over the one in memory,
which discarded the run plan the moment the phase annotation was written and
panicked the next reconcile. Then `RecoverRequestCreate` handed back the
`Restore` it had just constructed -- no status on it, whatever Velero had
done -- so the run reported `restore.status.phase absent` forever against a
restore that had completed minutes earlier.

Both were invisible to the unit tests, because the fakes returned what the
code expected rather than what the API does. The mocks now model the real
semantics: a submission reports no phase, and only a read-back knows. That
change alone makes the second bug reproducible in a test.

**Assuming "the existing path handles it".** §2 said the revert removed the
restored PVCs "exactly as today". It does not: today means waiting for a user
who, on a test cluster, has nothing to delete. See §5.7.1.

The lesson for the phases still unwritten is that the interesting failures
are at the seams with machinery this design reuses -- Velero's request
lifecycle, the client's write semantics, the discovered-app demote contract
-- and none of them are reachable from a fake.

### Still open

- **The VRG deletion path does not discard the restored PVCs** (§5.7.1).
  Deleting a DRPC normally preserves PVCs; whether dry-run leftovers are an
  exception needs deciding rather than defaulting.
- **The Ceph RBD validation is still owed.** No cluster has yet confirmed
  that an imported `VolumeSnapshotContent` provisions a usable PVC, or that
  deleting the source snapshot under `Retain` leaves it alone.
- **Four e2e scenarios are unwritten**: pre-emption mid-build, hand-over to a
  real failover, aborting a stalled run, and deleting the DRPC mid-run. Each
  races a build whose real timing is now observable, so they can be written
  against something real rather than guessed at.

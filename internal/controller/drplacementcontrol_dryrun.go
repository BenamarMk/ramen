// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rmn "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/dryrun"
	rmnutil "github.com/ramendr/ramen/internal/controller/util"
)

// Test failover ("dry run") lives entirely in this file on the hub side.
//
// A dry run is a mode, not an action. Setting spec.dryRun promotes the peer
// that is not currently Primary, lets it build a copy of the application in
// namespaces Ramen generates, and then returns it to Secondary. Throughout:
//
//   - spec.action is neither read to decide what to do nor written
//   - the user Placement never moves, and neither does status.phase
//   - the source cluster is never touched, so the real workload keeps running
//   - the test copy is left running afterwards for the user to inspect
//
// The whole lifecycle is driven by rmn.DryRunAnnotation on the DRPC. Only
// Ramen writes it and Ramen never clears it: leaving the terminal phase is a
// deliberate user action, and is the only gate on starting a second run.
//
//	absent ──user sets dryRun, pre-flight passes──► SettingUp
//	                                                   │
//	                       VRG reports the copy built  │
//	                       (or the run is pre-empted)  ▼
//	                                                Reverting
//	                                                   │
//	                       peer confirmed Secondary    ▼
//	                                                Completed  (terminal)
//	                                                   │
//	                       user deletes the annotation ▼
//	                                                absent

const dryRunConditionReason = "DryRunTestFailover"

// dryRunPhase reads the lifecycle phase from the DRPC. An empty return means
// no dry run is in progress.
func (d *DRPCInstance) dryRunPhase() rmn.DryRunPhase {
	return rmn.DryRunPhase(d.instance.GetAnnotations()[rmn.DryRunAnnotation])
}

// setDryRunPhase advances the lifecycle and persists it. The annotation is the
// state of record, so this must succeed before the next phase is acted on.
func (d *DRPCInstance) setDryRunPhase(phase rmn.DryRunPhase) error {
	rmnutil.AddAnnotation(d.instance, rmn.DryRunAnnotation, string(phase))

	if err := d.reconciler.Update(d.ctx, d.instance); err != nil {
		return fmt.Errorf("failed to set dry run phase %s: %w", phase, err)
	}

	d.log.Info("Dry run phase set", "phase", phase)

	return nil
}

// dryRunFinished reports whether a dry run has run its course. Once it has,
// executeAction stops routing here and the DRPC behaves normally again even
// though spec.dryRun is still set -- otherwise a stale flag would block a real
// failover indefinitely.
func (d *DRPCInstance) dryRunFinished() bool {
	return d.dryRunPhase() == rmn.DryRunCompleted
}

// RunDryRunFailover drives the test failover. It is a state machine over the
// annotation rather than over spec, so a reconcile can resume it at any point.
func (d *DRPCInstance) RunDryRunFailover() (bool, error) {
	const done = true

	switch d.dryRunPhase() {
	case rmn.DryRunSettingUp:
		return d.dryRunBuild()
	case rmn.DryRunReverting:
		return d.dryRunRevert()
	case rmn.DryRunCompleted:
		// Terminal. dryRunFinished keeps us from being called at all, so this
		// is only reachable in the reconcile that sets it.
		return done, nil
	default:
		return d.dryRunStart()
	}
}

// dryRunStart validates that a test failover can begin, records what it is
// about to do, and moves to SettingUp.
func (d *DRPCInstance) dryRunStart() (bool, error) {
	const done = true

	target, mapping, err := d.dryRunPlan()
	if err != nil {
		// The workload or the request is ineligible. Say so and stop rather
		// than spinning: something has to change before this can succeed.
		d.reconciler.recordFailure(d.ctx, d.instance, d.userPlacement,
			dryRunConditionReason, err.Error(), d.log)

		return done, nil
	}

	if !d.dryRunReadyToFailover() {
		// Transient: the workload is not protected yet, or replication is
		// behind. Wait rather than refuse.
		return !done, nil
	}

	now := metav1.Now()
	d.instance.Status.DryRun = &rmn.DryRunStatus{
		RunID:                  dryrun.RunID(string(d.instance.UID)),
		TargetCluster:          target,
		NamespaceMapping:       mapping,
		State:                  rmn.DryRunBuildInProgress,
		StartTime:              &now,
		ActionAtStart:          d.instance.Spec.Action,
		FailoverClusterAtStart: d.instance.Spec.FailoverCluster,
	}

	// Lower PeerReady before anything else. It is true -- the peer is about to
	// be Primary and is no longer available as a failover target -- and it is
	// also what stops EnsureCleanup short-circuiting during the revert, since
	// that returns early when PeerReady is already true for this generation.
	addOrUpdateCondition(&d.instance.Status.Conditions, rmn.ConditionPeerReady, d.instance.Generation,
		metav1.ConditionFalse, dryRunConditionReason,
		fmt.Sprintf("Test failover building a copy on cluster %q", target))

	d.log.Info("Starting dry run test failover", "target", target, "namespaces", mapping)

	if err := d.setDryRunPhase(rmn.DryRunSettingUp); err != nil {
		return !done, err
	}

	// Requeue so the next reconcile sees the persisted annotation.
	return !done, nil
}

// dryRunBuild promotes the test cluster and waits for it to report the test
// copy built, then moves on to the revert.
func (d *DRPCInstance) dryRunBuild() (bool, error) {
	const done = true

	if next, yield := d.dryRunShouldYield(); yield {
		d.log.Info("Dry run yielding to a real request", "phase", next)

		return !done, d.setDryRunPhase(next)
	}

	target := d.instance.Status.DryRun.TargetCluster

	// Promote. The dry-run shape is forced onto this VRG by
	// setVRGDryRunFields, which both the create and the flip path run through.
	if _, err := d.createVRGManifestWorkAsPrimary(target); err != nil {
		return !done, fmt.Errorf("dry run could not promote cluster %s: %w", target, err)
	}

	vrg := d.vrgs[target]
	if vrg == nil {
		d.log.Info("Dry run waiting for the test cluster's VRG to appear", "cluster", target)

		return !done, nil
	}

	d.mirrorDryRunStatus(vrg)

	if vrg.Status.DryRun == nil || vrg.Status.DryRun.State != rmn.DryRunBuildCompleted {
		return !done, nil
	}

	d.log.Info("Dry run test copy built, returning the test cluster to Secondary", "cluster", target)

	return !done, d.setDryRunPhase(rmn.DryRunReverting)
}

// dryRunRevert returns the test cluster to Secondary and completes the cycle,
// leaving the test copy running.
func (d *DRPCInstance) dryRunRevert() (bool, error) {
	const done = true

	home, err := d.dryRunHomeCluster()
	if err != nil {
		return !done, err
	}

	// EnsureCleanup demotes every peer other than home -- here, only the test
	// cluster -- waits for it to report Secondary, and restores PeerReady. Its
	// enclosing wrapper is deliberately not used: that one also writes to the
	// source cluster's VRG, which a dry run must never touch.
	if err := d.EnsureCleanup(home); err != nil {
		d.log.Info("Dry run waiting for the test cluster to return to Secondary", "reason", err.Error())

		return !done, nil
	}

	if err := d.setDryRunPhase(rmn.DryRunCompleted); err != nil {
		return !done, err
	}

	now := metav1.Now()
	d.instance.Status.DryRun.CompletionTime = &now

	d.log.Info("Dry run test failover complete; test copy left running",
		"namespaces", d.instance.Status.DryRun.NamespaceMapping)

	return done, nil
}

// dryRunShouldYield reports whether something real needs the peer back, and
// which phase to move to. A dry run holds a cluster Primary, so it has to get
// out of the way promptly.
//
// Yielding only ever happens while building. Once reverting there is nothing
// to accelerate -- the peer is already being freed as fast as it can be -- and
// dryRunFinished lets the real action through the moment it completes.
func (d *DRPCInstance) dryRunShouldYield() (rmn.DryRunPhase, bool) {
	status := d.instance.Status.DryRun
	if status == nil {
		return "", false
	}

	if !d.instance.Spec.DryRun {
		// Clearing the flag while asking to fail over to the cluster this run
		// already promoted is a promotion, not an abort. Finish immediately
		// and let RunFailover take it from there: its first branch finds the
		// VRG already Primary, so it proceeds straight to cleaning up the
		// source. Demoting and re-promoting would be waste in what is most
		// likely an emergency.
		if d.instance.Spec.Action == rmn.ActionFailover &&
			d.instance.Spec.FailoverCluster == status.TargetCluster {
			return rmn.DryRunCompleted, true
		}

		return rmn.DryRunReverting, true
	}

	// A real action was requested mid-build. Abandon the half-built copy and
	// free the peer. Both fields are watched because action alone is not
	// enough: a DRPC already holding Failover from an earlier real failover
	// shows no change when the user retargets failoverCluster at a genuine
	// disaster.
	if d.instance.Spec.Action != status.ActionAtStart ||
		d.instance.Spec.FailoverCluster != status.FailoverClusterAtStart {
		return rmn.DryRunReverting, true
	}

	return "", false
}

// dryRunPlan resolves what a run would do, and refuses if it cannot be done.
// Guards that the user must act on return an error; transient ones are checked
// separately by the caller.
func (d *DRPCInstance) dryRunPlan() (string, map[string]string, error) {
	if !isDiscoveredApp(d.instance) {
		return "", nil, fmt.Errorf("test failover supports discovered applications only; " +
			"spec.protectedNamespaces is empty")
	}

	if d.drType == DRTypeSync {
		return "", nil, fmt.Errorf("test failover is not supported for Sync (Metro) DR, " +
			"which would exercise fencing against a live peer")
	}

	if !d.dryRunIdle() {
		return "", nil, fmt.Errorf("test failover cannot start while another action is in progress "+
			"(phase %q, progression %q)", d.instance.Status.Phase, d.instance.Status.Progression)
	}

	target, err := d.dryRunTargetCluster()
	if err != nil {
		return "", nil, err
	}

	if err := d.dryRunVolSyncCheck(); err != nil {
		return "", nil, err
	}

	mapping, err := dryrun.NamespaceMapping(*d.instance.Spec.ProtectedNamespaces,
		dryrun.RunID(string(d.instance.UID)))
	if err != nil {
		return "", nil, err
	}

	return target, mapping, nil
}

// dryRunIdle reports whether the DRPC has settled. Starting a test on top of
// an in-flight action would hijack it.
func (d *DRPCInstance) dryRunIdle() bool {
	// Only the settled phases qualify; everything else is an action in flight.
	//nolint:exhaustive
	switch d.instance.Status.Phase {
	case rmn.Deployed, rmn.FailedOver, rmn.Relocated:
	default:
		return false
	}

	if d.instance.Status.Progression != rmn.ProgressionCompleted {
		return false
	}

	peerReady := rmnutil.FindCondition(d.instance.Status.Conditions, rmn.ConditionPeerReady)

	return peerReady != nil && peerReady.Status == metav1.ConditionTrue
}

// dryRunVolSyncCheck refuses workloads with VolSync-protected volumes. A
// partial test copy -- some volumes present, some missing -- is worse than a
// clear refusal. CephFS is refused the same way, but on the managed cluster,
// which is the only side that can resolve a StorageClass to a provisioner.
func (d *DRPCInstance) dryRunVolSyncCheck() error {
	for _, vrg := range d.vrgs {
		for i := range vrg.Status.ProtectedPVCs {
			if vrg.Status.ProtectedPVCs[i].ProtectedByVolSync {
				return fmt.Errorf("test failover does not yet support VolSync; PVC %q is VolSync protected",
					vrg.Status.ProtectedPVCs[i].Name)
			}
		}
	}

	return nil
}

// dryRunTargetCluster derives the peer to promote: the one not currently
// running the workload.
//
// spec.failoverCluster is deliberately not consulted. Requiring it would
// reintroduce the coupling to a real action that this design removes, and it
// goes stale -- after a real failover to a cluster it still names that
// cluster, which is now the Primary, so a test would target the cluster
// already running the workload.
func (d *DRPCInstance) dryRunTargetCluster() (string, error) {
	clusters := rmnutil.DRPolicyClusterNames(d.drPolicy)
	if len(clusters) != 2 { //nolint:mnd
		return "", fmt.Errorf("test failover needs a two cluster DRPolicy; %q has %d",
			d.drPolicy.GetName(), len(clusters))
	}

	home := d.dryRunCurrentHome()
	if home == "" {
		return "", fmt.Errorf("cannot determine which cluster is running the workload")
	}

	for _, cluster := range clusters {
		if cluster != home {
			return cluster, nil
		}
	}

	return "", fmt.Errorf("cluster %q running the workload is not in DRPolicy %q", home, d.drPolicy.GetName())
}

// dryRunHomeCluster returns the cluster still running the real workload. It is
// read back from the recorded target so that it cannot drift mid-run.
func (d *DRPCInstance) dryRunHomeCluster() (string, error) {
	status := d.instance.Status.DryRun
	if status == nil || status.TargetCluster == "" {
		return "", fmt.Errorf("dry run has no recorded target cluster")
	}

	for _, cluster := range rmnutil.DRPolicyClusterNames(d.drPolicy) {
		if cluster != status.TargetCluster {
			return cluster, nil
		}
	}

	return "", fmt.Errorf("dry run target %q is the only cluster in DRPolicy %q",
		status.TargetCluster, d.drPolicy.GetName())
}

// dryRunCurrentHome is the cluster the workload runs on now.
//
// getCurrentHomeClusterName is not reused here: its last resort is to return
// whichever cluster is not the one passed in, and guessing is unacceptable
// when the answer decides which cluster gets promoted.
func (d *DRPCInstance) dryRunCurrentHome() string {
	if decision := d.reconciler.getClusterDecision(d.userPlacement); decision.ClusterName != "" {
		return decision.ClusterName
	}

	return d.instance.Status.PreferredDecision.ClusterName
}

// dryRunReadyToFailover is the transient half of the pre-flight: there is no
// point building a test copy of a workload that is not protected, or whose
// replication has fallen behind, because the copy would not represent
// recoverable state.
func (d *DRPCInstance) dryRunReadyToFailover() bool {
	return d.isProtected() && !d.isGroupSyncLagging()
}

// mirrorDryRunStatus copies what the test cluster reports onto the DRPC. The
// managed cluster's log is not somewhere a DRPC user looks, and with no
// timeout and no failure state the message is the only signal a stalled run
// gives.
func (d *DRPCInstance) mirrorDryRunStatus(vrg *rmn.VolumeReplicationGroup) {
	if vrg.Status.DryRun == nil || d.instance.Status.DryRun == nil {
		return
	}

	d.instance.Status.DryRun.State = vrg.Status.DryRun.State
	d.instance.Status.DryRun.Message = vrg.Status.DryRun.Message

	if len(vrg.Status.DryRun.NamespaceMapping) > 0 {
		d.instance.Status.DryRun.NamespaceMapping = vrg.Status.DryRun.NamespaceMapping
	}
}

// setVRGDryRunFields forces the dry-run shape onto the VRG of the cluster a
// test failover has promoted, and clears it everywhere else.
//
// The managed cluster only takes dry-run snapshots when its VRG says action
// Failover and dryRun true. spec.action no longer supplies that -- a dry run
// leaves it alone, and it is commonly empty -- so the hub sets both here.
// Without this no snapshots would ever be taken and every run would stall.
//
// Clearing dryRun once the run leaves SettingUp is what triggers the managed
// cluster to hand the storage snapshots over to the test namespaces, so the
// test copy survives the revert.
func (d *DRPCInstance) setVRGDryRunFields(vrg *rmn.VolumeReplicationGroup, cluster string) {
	status := d.instance.Status.DryRun
	if status == nil || status.TargetCluster != cluster {
		return
	}

	building := d.dryRunPhase() == rmn.DryRunSettingUp

	vrg.Spec.DryRun = building
	if building {
		vrg.Spec.Action = rmn.VRGActionFailover
	}
}

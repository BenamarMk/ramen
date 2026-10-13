// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"fmt"
	"time"

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
// Ramen writes it, and the annotation is present for exactly as long as a test
// copy exists -- which is the only gate on starting a second run. Ready is
// terminal until the user clears spec.dryRun; Ramen removes the annotation
// itself once the copy it describes is gone.
//
//	absent ──user sets dryRun, pre-flight passes──► SettingUp
//	                                                   │
//	                       VRG reports the copy built  │
//	                       (or the run is pre-empted)  ▼
//	                                                Reverting
//	                                                   │
//	                       peer confirmed Secondary    ▼
//	                                                 Ready  (the copy is the
//	                                                   │      user's until they
//	                           user clears dryRun      ▼      clear the flag)
//	                                               TearingDown
//	                                                   │
//	                         test copy deleted         ▼
//	                                                absent

const dryRunConditionReason = "DryRunTestFailover"

// dryRunBudget is how long building a test copy is expected to take: promote
// the test cluster, restore and promote its PVCs, restore the kube objects,
// then hand the snapshots over and demote.
//
// It is an expectation, not a limit. There is no timeout by design, so
// nothing acts on this -- but a run that exceeds it has almost certainly
// stopped making progress, and with no failure state to raise, saying so is
// the only signal an operator gets.
const dryRunBudget = 5 * time.Minute

// dryRunPhase reads the lifecycle phase from the DRPC. An empty return means
// no dry run is in progress.
func (d *DRPCInstance) dryRunPhase() rmn.DryRunPhase {
	return rmn.DryRunPhase(d.instance.GetAnnotations()[rmn.DryRunAnnotation])
}

// setDryRunPhase advances the lifecycle and persists it. The annotation is the
// state of record, so this must succeed before the next phase is acted on.
func (d *DRPCInstance) setDryRunPhase(phase rmn.DryRunPhase) error {
	rmnutil.AddAnnotation(d.instance, rmn.DryRunAnnotation, string(phase))

	// Updating the DRPC decodes the server's copy back over this object, and
	// the server's copy carries whatever status was last persisted -- not the
	// plan just computed in memory, which has not been written yet. Without
	// holding on to it here, writing the annotation silently discards the
	// plan, and the next reconcile finds a run in progress with nothing
	// recorded about it.
	plan := d.instance.Status.DryRun

	if err := d.reconciler.Update(d.ctx, d.instance); err != nil {
		return fmt.Errorf("failed to set dry run phase %s: %w", phase, err)
	}

	d.instance.Status.DryRun = plan

	d.log.Info("Dry run phase set", "phase", phase)

	return nil
}

// dryRunWants reports whether the dry-run path has anything to do.
//
// The interesting case is Ready, where a test copy is up and waiting to be
// looked at. With the flag still set there is nothing to do and real actions
// must pass through -- a stale flag cannot be allowed to block a failover.
// Clearing the flag from there is what asks for the copy to be taken down.
func (d *DRPCInstance) dryRunWants() bool {
	// Every other phase is a run already under way.
	//nolint:exhaustive
	switch d.dryRunPhase() {
	case "":
		return d.instance.Spec.DryRun
	case rmn.DryRunReady:
		return !d.instance.Spec.DryRun
	default:
		return true
	}
}

// RunDryRunFailover drives the test failover. It is a state machine over the
// annotation rather than over spec, so a reconcile can resume it at any point.
// RunDryRunFailover advances the test failover by one step.
//
// It paces its own retries. The DRPC reconciler turns "not done" into
// Result{Requeue: true}, which is rate limited: controller-runtime never
// forgets the item on that path, so the delay doubles on every consecutive
// pass and tops out at the limiter's ~16 minute ceiling. Real DR actions
// finish in a handful of reconciles and never feel it. A dry run holds "not
// done" continuously from SettingUp through Reverting into TearingDown --
// minutes of it -- and by the end the hub is waking only every few minutes,
// so a tear-down that finished long ago sits unnoticed.
//
// RequeueAfter forgets the item first, which both resets the backoff and
// bounds the wait.
func (d *DRPCInstance) RunDryRunFailover() (bool, error) {
	done, err := d.runDryRunFailover()
	if !done && err == nil {
		d.requeueAfter = dryRunRequeueInterval
	}

	return done, err
}

func (d *DRPCInstance) runDryRunFailover() (bool, error) {
	const done = true

	phase := d.dryRunPhase()
	if phase == "" {
		d.log.Info("Dry run requested")
	} else {
		d.log.Info("Dry run reconciling", "phase", phase)
	}

	switch phase {
	case rmn.DryRunSettingUp:
		return d.dryRunBuild()
	case rmn.DryRunReverting:
		return d.dryRunRevert()
	case rmn.DryRunTearingDown:
		return d.dryRunTearDown()
	case rmn.DryRunReady:
		// The copy is up. dryRunWants only routes here from Ready once the
		// flag has been cleared, which asks for the copy to go.
		d.log.Info("Dry run tearing the test copy down")

		return !done, d.setDryRunPhase(rmn.DryRunTearingDown)
	default:
		return d.dryRunStart()
	}
}

// clearDryRunPhase removes the annotation, freeing the DRPC to run another
// test. It is the last thing a run does.
func (d *DRPCInstance) clearDryRunPhase() error {
	if !rmnutil.HasAnnotation(d.instance, rmn.DryRunAnnotation) {
		return nil
	}

	delete(d.instance.Annotations, rmn.DryRunAnnotation)

	plan := d.instance.Status.DryRun

	if err := d.reconciler.Update(d.ctx, d.instance); err != nil {
		return fmt.Errorf("failed to clear the dry run phase: %w", err)
	}

	d.instance.Status.DryRun = plan

	d.log.Info("Dry run finished; another test failover may be started")

	return nil
}

// dryRunTearDown waits for the test cluster to delete the copy, then clears
// the annotation.
//
// The test cluster does the deleting -- the namespaces are there, not here --
// and reports being done by emptying its own status.dryRun.
func (d *DRPCInstance) dryRunTearDown() (bool, error) {
	const done = true

	status := d.instance.Status.DryRun
	if status == nil {
		return !done, d.clearDryRunPhase()
	}

	target := status.TargetCluster

	// Carry the phase to the test cluster. Its VRG is Secondary by now, so
	// none of the usual update paths will refresh its annotations.
	if err := d.refreshVRGAnnotations(target); err != nil {
		d.log.Info("Dry run waiting to tell the test cluster to tear down", "reason", err.Error())

		return !done, nil
	}

	vrg := d.vrgs[target]
	if vrg != nil && vrg.Status.DryRun != nil {
		d.log.Info("Dry run waiting for the test copy to be deleted",
			"cluster", target, "namespaces", vrg.Status.DryRun.NamespaceMapping)

		return !done, nil
	}

	d.instance.Status.DryRun = dryRunHistory(status)

	d.log.Info("Test copy gone; keeping the record of the run",
		"runID", status.RunID, "cluster", target, "completed", status.CompletionTime)

	return done, d.clearDryRunPhase()
}

// dryRunHistory reduces a finished run to what is still true once the test
// copy has been deleted.
//
// The alternative was to drop status.dryRun entirely, which lost the only
// evidence a DRPC had ever been tested. The annotation cannot carry that: it
// is the gate on starting a second run, so it has to go back to absent, and a
// bare terminal value would say nothing about *when*. A date is what a DR
// drill actually needs, and kubectl renders CompletionTime as lastTested.
//
// What is dropped describes things that no longer exist. NamespaceMapping
// names deleted namespaces and would send someone looking for them; Message
// explained a run in flight; State is build progress for a build that is
// gone, and a lingering InProgress would read as a live run; the at-start
// spec snapshot only ever mattered for pre-emption.
func dryRunHistory(status *rmn.DryRunStatus) *rmn.DryRunStatus {
	if status == nil {
		return nil
	}

	return &rmn.DryRunStatus{
		RunID:          status.RunID,
		TargetCluster:  status.TargetCluster,
		StartTime:      status.StartTime,
		CompletionTime: status.CompletionTime,
	}
}

// refreshVRGAnnotations rewrites a VRG ManifestWork with current annotations
// and nothing else.
//
// ensureVRGManifestWork refuses a VRG that is not Primary, and updateVRGState
// returns early when the state already matches, so neither will carry a phase
// change to a cluster that has already been demoted.
func (d *DRPCInstance) refreshVRGAnnotations(cluster string) error {
	vrg, err := d.getVRGFromManifestWork(cluster)
	if err != nil {
		return err
	}

	d.setVRGAnnotations(vrg, cluster)

	return d.updateManifestWork(cluster, vrg)
}

// dryRunStart validates that a test failover can begin, records what it is
// about to do, and moves to SettingUp.
func (d *DRPCInstance) dryRunStart() (bool, error) {
	const done = true

	d.log.Info("Dry run pre-flight")

	target, mapping, err := d.dryRunPlan()
	if err != nil {
		d.log.Info("Dry run refused", "reason", err.Error())

		// The workload or the request is ineligible. Say so and stop rather
		// than spinning: something has to change before this can succeed.
		d.reconciler.recordFailure(d.ctx, d.instance, d.userPlacement,
			dryRunConditionReason, err.Error(), d.log)

		return done, nil
	}

	if !d.dryRunReadyToFailover() {
		// Transient: the workload is not protected yet, or replication is
		// behind. Wait rather than refuse.
		d.log.Info("Dry run waiting: the workload is not protected, or replication is behind")

		return !done, nil
	}

	d.instance.Status.DryRun = d.newDryRunStatus(target, mapping)

	// Lower PeerReady before anything else. It is true -- the peer is about to
	// be Primary and is no longer available as a failover target -- and it is
	// also what stops EnsureCleanup short-circuiting during the revert, since
	// that returns early when PeerReady is already true for this generation.
	addOrUpdateCondition(&d.instance.Status.Conditions, rmn.ConditionPeerReady, d.instance.Generation,
		metav1.ConditionFalse, dryRunConditionReason,
		fmt.Sprintf("Test failover building a copy on cluster %q", target))

	d.log.Info("Starting dry run test failover",
		"target", target, "namespaces", mapping,
		"runID", dryrun.RunID(string(d.instance.UID)),
		"action", d.instance.Spec.Action, "phase", d.instance.Status.Phase)

	if err := d.setDryRunPhase(rmn.DryRunSettingUp); err != nil {
		return !done, err
	}

	// Requeue so the next reconcile sees the persisted annotation.
	return !done, nil
}

// newDryRunStatus records what a run is about to do, so that later phases
// read back the plan rather than recomputing it and risking a different
// answer if cluster roles shift underneath them.
func (d *DRPCInstance) newDryRunStatus(target string, mapping map[string]string) *rmn.DryRunStatus {
	now := metav1.Now()

	return &rmn.DryRunStatus{
		RunID:                  dryrun.RunID(string(d.instance.UID)),
		TargetCluster:          target,
		NamespaceMapping:       mapping,
		State:                  rmn.DryRunBuildInProgress,
		StartTime:              &now,
		ActionAtStart:          d.instance.Spec.Action,
		FailoverClusterAtStart: d.instance.Spec.FailoverCluster,
	}
}

// dryRunStatus returns the recorded plan, rebuilding it if it has gone
// missing.
//
// A run in progress with no plan recorded used to be fatal. It is recoverable
// instead: everything in the plan is derived -- the run ID from the DRPC UID,
// the namespaces from that, the target from which cluster is not running the
// workload -- and none of those move while a run is in flight, so rebuilding
// gives the same answer the run started with.
//
// The exception is the spec snapshot. A real action requested during the
// window where the plan was missing cannot be detected as a change, because
// there is nothing left to compare against. That is a narrow hole, and it
// closes as soon as the plan is recorded again.
func (d *DRPCInstance) dryRunStatus() (*rmn.DryRunStatus, error) {
	if status := d.instance.Status.DryRun; status != nil && status.TargetCluster != "" {
		return status, nil
	}

	target, err := d.dryRunTargetCluster()
	if err != nil {
		return nil, err
	}

	namespaces := d.dryRunSourceNamespaces()
	if len(namespaces) == 0 {
		return nil, fmt.Errorf("dry run has no protected namespaces to rebuild its plan from")
	}

	mapping, err := dryrun.NamespaceMapping(namespaces, dryrun.RunID(string(d.instance.UID)))
	if err != nil {
		return nil, err
	}

	d.log.Info("Rebuilding the dry run plan, which was not recorded", "target", target)

	d.instance.Status.DryRun = d.newDryRunStatus(target, mapping)

	return d.instance.Status.DryRun, nil
}

// dryRunSourceNamespaces are the namespaces a test copy mirrors. The scope is
// discovered applications, so this is always populated for a run that passed
// pre-flight.
func (d *DRPCInstance) dryRunSourceNamespaces() []string {
	if d.instance.Spec.ProtectedNamespaces == nil {
		return nil
	}

	return *d.instance.Spec.ProtectedNamespaces
}

// dryRunBuild promotes the test cluster and waits for it to report the test
// copy built, then moves on to the revert.
func (d *DRPCInstance) dryRunBuild() (bool, error) {
	const done = true

	if next, yield := d.dryRunShouldYield(); yield {
		d.log.Info("Dry run yielding to a real request", "phase", next)

		if next == "" {
			return !done, d.clearDryRunPhase()
		}

		return !done, d.setDryRunPhase(next)
	}

	status, err := d.dryRunStatus()
	if err != nil {
		return !done, err
	}

	target := status.TargetCluster

	d.log.Info("Dry run promoting the test cluster", "cluster", target, "runID", status.RunID)

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

	if vrg.Status.DryRun == nil {
		d.log.Info("Dry run waiting for the test cluster to report", "cluster", target)

		return !done, nil
	}

	if vrg.Status.DryRun.State != rmn.DryRunBuildCompleted {
		d.log.Info("Dry run building", "cluster", target,
			"state", vrg.Status.DryRun.State, "reason", vrg.Status.DryRun.Message)

		return !done, nil
	}

	d.log.Info("Dry run test copy built, returning the test cluster to Secondary", "cluster", target)

	return !done, d.setDryRunPhase(rmn.DryRunReverting)
}

// dryRunRevert returns the test cluster to Secondary and completes the cycle,
// leaving the test copy running.
func (d *DRPCInstance) dryRunRevert() (bool, error) {
	const done = true

	// Recover the plan before anything reads it: the peer still has to be
	// handed back even if the run's status was lost along the way.
	status, err := d.dryRunStatus()
	if err != nil {
		return !done, err
	}

	home, err := d.dryRunHomeCluster()
	if err != nil {
		return !done, err
	}

	// EnsureCleanup demotes every peer other than home -- here, only the test
	// cluster -- waits for it to report Secondary, and restores PeerReady. Its
	// enclosing wrapper is deliberately not used: that one also writes to the
	// source cluster's VRG, which a dry run must never touch.
	d.log.Info("Dry run returning the test cluster to Secondary",
		"cluster", status.TargetCluster, "home", home)

	if err := d.EnsureCleanup(home); err != nil {
		d.log.Info("Dry run waiting for the test cluster to return to Secondary", "reason", err.Error())

		return !done, nil
	}

	if err := d.setDryRunPhase(rmn.DryRunReady); err != nil {
		return !done, err
	}

	now := metav1.Now()
	status.CompletionTime = &now

	d.log.Info("Dry run test failover complete; test copy left running",
		"namespaces", status.NamespaceMapping)

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
			d.log.Info("Dry run handing over to a real failover on the cluster it already promoted",
				"cluster", status.TargetCluster)

			// Step aside entirely rather than finishing into Ready, which
			// would read as "tear the copy down" the moment the flag is seen
			// clear. A promotion is not a request to delete anything, and the
			// copy keeps running alongside the now real workload.
			return "", true
		}

		d.log.Info("Dry run aborting: the flag was cleared without asking for a failover here",
			"action", d.instance.Spec.Action)

		return rmn.DryRunReverting, true
	}

	// A real action was requested mid-build. Abandon the half-built copy and
	// free the peer. Both fields are watched because action alone is not
	// enough: a DRPC already holding Failover from an earlier real failover
	// shows no change when the user retargets failoverCluster at a genuine
	// disaster.
	if d.instance.Spec.Action != status.ActionAtStart ||
		d.instance.Spec.FailoverCluster != status.FailoverClusterAtStart {
		d.log.Info("Dry run pre-empted by a real request; freeing the peer",
			"action", d.instance.Spec.Action, "wasAction", status.ActionAtStart,
			"failoverCluster", d.instance.Spec.FailoverCluster,
			"wasFailoverCluster", status.FailoverClusterAtStart)

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
	status := d.instance.Status.DryRun
	if vrg.Status.DryRun == nil || status == nil {
		return
	}

	status.State = vrg.Status.DryRun.State
	status.Message = d.dryRunMessage(vrg.Status.DryRun.Message)

	if len(vrg.Status.DryRun.NamespaceMapping) > 0 {
		status.NamespaceMapping = vrg.Status.DryRun.NamespaceMapping
	}
}

// dryRunMessage is what the test cluster reported, with how long the run has
// been going if that has become the more interesting fact.
//
// A run holds the peer Primary and replication stopped for as long as it
// lasts. Nothing bounds that, so an operator has to be told when a run has
// outlived the time it should have taken -- the reason alone reads the same
// at thirty seconds as at three hours.
func (d *DRPCInstance) dryRunMessage(reason string) string {
	status := d.instance.Status.DryRun
	if status == nil || status.StartTime == nil || status.State == rmn.DryRunBuildCompleted {
		return reason
	}

	elapsed := time.Since(status.StartTime.Time)
	if elapsed <= dryRunBudget {
		return reason
	}

	return fmt.Sprintf("building for %s, longer than the %s this should take: %s",
		elapsed.Round(time.Second), dryRunBudget, reason)
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
	// The phase, not status, is what says a run is live. status.dryRun
	// outlives the run as the record of it (dryRunHistory), and this is
	// called from updateVRGOptionalFields on every VRG write -- including
	// real failovers long after a test finished.
	status := d.instance.Status.DryRun
	if status == nil || status.TargetCluster != cluster || d.dryRunPhase() == "" {
		return
	}

	building := d.dryRunPhase() == rmn.DryRunSettingUp

	vrg.Spec.DryRun = building
	if building {
		vrg.Spec.Action = rmn.VRGActionFailover
	}

	d.log.Info("Shaped the test cluster's VRG for the dry run",
		"cluster", cluster, "dryRun", vrg.Spec.DryRun, "action", vrg.Spec.Action)
}

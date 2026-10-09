// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	rmn "github.com/ramendr/ramen/api/v1alpha1"
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

// dryRunPhase reads the lifecycle phase from the DRPC. An empty return means
// no dry run is in progress.
func (d *DRPCInstance) dryRunPhase() rmn.DryRunPhase {
	return rmn.DryRunPhase(d.instance.GetAnnotations()[rmn.DryRunAnnotation])
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

// dryRunReadyToFailover is the first pre-flight guard: there is no point
// building a test copy of a workload that is not protected, or whose
// replication has fallen behind, because the copy would not represent
// recoverable state.
func (d *DRPCInstance) dryRunReadyToFailover() bool {
	return d.isProtected() && !d.isGroupSyncLagging()
}

// dryRunStart validates that a test failover can begin, then records what it is
// about to do and moves to SettingUp.
//
// TODO(phase 2): the remaining pre-flight guards, target cluster derivation,
// status.dryRun, PeerReady=False, and the move to SettingUp. Until those land
// this deliberately writes no annotation -- stranding a DRPC in SettingUp with
// the build and revert handlers still stubbed would leave it no way out.
func (d *DRPCInstance) dryRunStart() (bool, error) {
	const done = true

	if !d.dryRunReadyToFailover() {
		return !done, nil
	}

	d.log.Info("Dry run test failover requested; start is not yet implemented, ignoring")

	return done, nil
}

// dryRunBuild promotes the test cluster and waits for it to report the test
// copy built, then moves to Reverting.
//
// TODO(phase 2): promote via createVRGManifestWorkAsPrimary, mirror VRG
// status.dryRun, yield to a real action if one arrives.
func (d *DRPCInstance) dryRunBuild() (bool, error) {
	const done = true

	return done, nil
}

// dryRunRevert returns the test cluster to Secondary and completes the cycle,
// leaving the test copy running.
//
// TODO(phase 2): EnsureCleanup against the unchanged home cluster, then
// Completed.
func (d *DRPCInstance) dryRunRevert() (bool, error) {
	const done = true

	return done, nil
}

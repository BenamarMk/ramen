// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rmn "github.com/ramendr/ramen/api/v1alpha1"
)

const (
	testHomeCluster   = "east"
	testTargetCluster = "west"
)

// dryRunInstance builds a DRPCInstance mid-build: the run has started against
// the given spec and promoted testTargetCluster.
func dryRunInstance(action rmn.DRAction, failoverCluster string) *DRPCInstance {
	return &DRPCInstance{
		instance: &rmn.DRPlacementControl{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{rmn.DryRunAnnotation: string(rmn.DryRunSettingUp)},
			},
			Spec: rmn.DRPlacementControlSpec{
				DryRun:          true,
				Action:          action,
				FailoverCluster: failoverCluster,
			},
			Status: rmn.DRPlacementControlStatus{
				DryRun: &rmn.DryRunStatus{
					TargetCluster:          testTargetCluster,
					ActionAtStart:          action,
					FailoverClusterAtStart: failoverCluster,
				},
			},
		},
		drPolicy: &rmn.DRPolicy{
			Spec: rmn.DRPolicySpec{DRClusters: []string{testHomeCluster, testTargetCluster}},
		},
	}
}

func TestDryRunHoldsWhileNothingRealIsAsked(t *testing.T) {
	d := dryRunInstance("", "")

	if phase, yield := d.dryRunShouldYield(); yield {
		t.Errorf("yielded to nothing, moving to %q", phase)
	}
}

func TestDryRunPreemptedByANewAction(t *testing.T) {
	// A disaster lands mid-build. The peer is held Primary by the test, so the
	// run must abandon the half-built copy and free it.
	d := dryRunInstance("", "")
	d.instance.Spec.Action = rmn.ActionFailover

	phase, yield := d.dryRunShouldYield()
	if !yield || phase != rmn.DryRunReverting {
		t.Errorf("got (%q, %v), want (Reverting, true)", phase, yield)
	}
}

func TestDryRunPreemptedByARetargetedFailover(t *testing.T) {
	// A DRPC already holding Failover from an earlier real failover shows no
	// change in action when the user retargets failoverCluster at a genuine
	// disaster, so failoverCluster has to be watched too.
	d := dryRunInstance(rmn.ActionFailover, testHomeCluster)
	d.instance.Spec.FailoverCluster = "a-third-cluster"

	phase, yield := d.dryRunShouldYield()
	if !yield || phase != rmn.DryRunReverting {
		t.Errorf("got (%q, %v), want (Reverting, true)", phase, yield)
	}
}

func TestDryRunHandsOverToARealFailoverOnTheSameCluster(t *testing.T) {
	// Clearing the flag while asking to fail over to the cluster already
	// promoted is a promotion, not an abort. Demoting and re-promoting would
	// be waste in what is most likely an emergency, so the run completes and
	// lets RunFailover carry on from the already-Primary VRG.
	d := dryRunInstance(rmn.ActionFailover, testTargetCluster)
	d.instance.Spec.DryRun = false

	phase, yield := d.dryRunShouldYield()
	if !yield || phase != rmn.DryRunCompleted {
		t.Errorf("got (%q, %v), want (Completed, true)", phase, yield)
	}
}

func TestDryRunAbortsWhenTheFlagIsClearedWithoutAFailover(t *testing.T) {
	// Without this the dry-run path would simply stop being called, stranding
	// the peer Primary with the annotation still at SettingUp.
	for name, action := range map[string]rmn.DRAction{
		"no action": "",
		"relocate":  rmn.ActionRelocate,
	} {
		t.Run(name, func(t *testing.T) {
			d := dryRunInstance(action, "")
			d.instance.Spec.DryRun = false

			phase, yield := d.dryRunShouldYield()
			if !yield || phase != rmn.DryRunReverting {
				t.Errorf("got (%q, %v), want (Reverting, true)", phase, yield)
			}
		})
	}
}

func TestDryRunTargetsThePeerNotRunningTheWorkload(t *testing.T) {
	d := dryRunInstance("", "")
	d.instance.Status.PreferredDecision.ClusterName = testHomeCluster

	target, err := d.dryRunTargetCluster()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if target != testTargetCluster {
		t.Errorf("target %q, want %q", target, testTargetCluster)
	}
}

func TestDryRunRefusesWhenTheHomeClusterIsUnknown(t *testing.T) {
	// Guessing is unacceptable here: the answer decides which cluster gets
	// promoted, and the wrong guess promotes the one running the workload.
	d := dryRunInstance("", "")

	if _, err := d.dryRunTargetCluster(); err == nil {
		t.Error("expected an error when no cluster decision is available")
	}
}

func TestDryRunHomeClusterIsTheRecordedTargetsPeer(t *testing.T) {
	d := dryRunInstance("", "")

	home, err := d.dryRunHomeCluster()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if home != testHomeCluster {
		t.Errorf("home %q, want %q", home, testHomeCluster)
	}
}

func TestDryRunIdleOnlyInASettledPhase(t *testing.T) {
	ready := []metav1.Condition{{
		Type:   rmn.ConditionPeerReady,
		Status: metav1.ConditionTrue,
	}}

	for name, tc := range map[string]struct {
		phase       rmn.DRState
		progression rmn.ProgressionStatus
		conditions  []metav1.Condition
		want        bool
	}{
		"settled after deploy":   {rmn.Deployed, rmn.ProgressionCompleted, ready, true},
		"settled after failover": {rmn.FailedOver, rmn.ProgressionCompleted, ready, true},
		"settled after relocate": {rmn.Relocated, rmn.ProgressionCompleted, ready, true},
		"mid failover":           {rmn.FailingOver, rmn.ProgressionCompleted, ready, false},
		"still cleaning up":      {rmn.FailedOver, rmn.ProgressionCleaningUp, ready, false},
		"peer not ready":         {rmn.Deployed, rmn.ProgressionCompleted, nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			d := dryRunInstance("", "")
			d.instance.Status.Phase = tc.phase
			d.instance.Status.Progression = tc.progression
			d.instance.Status.Conditions = tc.conditions

			if got := d.dryRunIdle(); got != tc.want {
				t.Errorf("dryRunIdle() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDryRunRefusesVolSyncWorkloads(t *testing.T) {
	// A partial test copy -- some volumes present, some missing -- is worse
	// than a clear refusal.
	d := dryRunInstance("", "")
	d.vrgs = map[string]*rmn.VolumeReplicationGroup{
		testHomeCluster: {Status: rmn.VolumeReplicationGroupStatus{
			ProtectedPVCs: []rmn.ProtectedPVC{
				{Name: "rbd-pvc"},
				{Name: "volsync-pvc", ProtectedByVolSync: true},
			},
		}},
	}

	err := d.dryRunVolSyncCheck()
	if err == nil {
		t.Fatal("expected VolSync to be refused")
	}

	if got := err.Error(); !strings.Contains(got, "volsync-pvc") {
		t.Errorf("error does not name the offending PVC: %v", got)
	}
}

func TestSetVRGDryRunFieldsOnlyTouchesTheTestCluster(t *testing.T) {
	d := dryRunInstance("", "")

	// The source cluster must come away untouched: a dry run never writes to
	// the cluster running the real workload.
	source := &rmn.VolumeReplicationGroup{}
	d.setVRGDryRunFields(source, testHomeCluster)

	if source.Spec.DryRun || source.Spec.Action != "" {
		t.Errorf("source VRG was modified: dryRun=%v action=%q", source.Spec.DryRun, source.Spec.Action)
	}

	// The test cluster is forced into the shape the managed cluster gates
	// snapshotting on, regardless of spec.action being empty.
	target := &rmn.VolumeReplicationGroup{}
	d.setVRGDryRunFields(target, testTargetCluster)

	if !target.Spec.DryRun || target.Spec.Action != rmn.VRGActionFailover {
		t.Errorf("target VRG not shaped for a dry run: dryRun=%v action=%q",
			target.Spec.DryRun, target.Spec.Action)
	}
}

func TestSetVRGDryRunFieldsClearsDryRunOnRevert(t *testing.T) {
	// Clearing dryRun is what tells the managed cluster to hand the storage
	// snapshots over to the test namespaces, so the copy survives the revert.
	d := dryRunInstance("", "")
	d.instance.Annotations[rmn.DryRunAnnotation] = string(rmn.DryRunReverting)

	vrg := &rmn.VolumeReplicationGroup{Spec: rmn.VolumeReplicationGroupSpec{DryRun: true}}
	d.setVRGDryRunFields(vrg, testTargetCluster)

	if vrg.Spec.DryRun {
		t.Error("dryRun still set on the test VRG while reverting")
	}
}

func TestSetVRGDryRunFieldsIsInertWithoutARun(t *testing.T) {
	d := dryRunInstance("", "")
	d.instance.Status.DryRun = nil

	vrg := &rmn.VolumeReplicationGroup{Spec: rmn.VolumeReplicationGroupSpec{Action: rmn.VRGActionRelocate}}
	d.setVRGDryRunFields(vrg, testTargetCluster)

	if vrg.Spec.DryRun || vrg.Spec.Action != rmn.VRGActionRelocate {
		t.Errorf("VRG modified with no run recorded: dryRun=%v action=%q", vrg.Spec.DryRun, vrg.Spec.Action)
	}
}

// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	volrep "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
				DryRun:              true,
				Action:              action,
				FailoverCluster:     failoverCluster,
				ProtectedNamespaces: &[]string{"workload1", "workload2"},
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

func TestMirrorBringsTheTestClusterReportToTheHub(t *testing.T) {
	// The managed cluster's log is not somewhere a DRPC user looks, so what
	// the test cluster says has to reach the hub.
	d := dryRunInstance("", "")
	started := metav1.NewTime(time.Now())
	d.instance.Status.DryRun.StartTime = &started

	d.mirrorDryRunStatus(&rmn.VolumeReplicationGroup{
		Status: rmn.VolumeReplicationGroupStatus{
			DryRun: &rmn.DryRunStatus{
				State:            rmn.DryRunBuildInProgress,
				Message:          "waiting for test PVC db",
				NamespaceMapping: map[string]string{"workload1": "workload1-test-abc"},
			},
		},
	})

	got := d.instance.Status.DryRun
	if got.State != rmn.DryRunBuildInProgress {
		t.Errorf("state %q not mirrored", got.State)
	}

	if got.Message != "waiting for test PVC db" {
		t.Errorf("message %q not mirrored verbatim", got.Message)
	}

	if got.NamespaceMapping["workload1"] != "workload1-test-abc" {
		t.Errorf("namespace mapping not mirrored: %v", got.NamespaceMapping)
	}
}

func TestMirrorFlagsARunThatHasOutlivedItsBudget(t *testing.T) {
	// A run holds the peer Primary and replication stopped for as long as it
	// lasts, and nothing bounds that. The reason alone reads the same at
	// thirty seconds as at three hours, so the duration has to be said.
	d := dryRunInstance("", "")
	started := metav1.NewTime(time.Now().Add(-90 * time.Minute))
	d.instance.Status.DryRun.StartTime = &started

	d.mirrorDryRunStatus(&rmn.VolumeReplicationGroup{
		Status: rmn.VolumeReplicationGroupStatus{
			DryRun: &rmn.DryRunStatus{
				State:   rmn.DryRunBuildInProgress,
				Message: "waiting for test PVC db",
			},
		},
	})

	message := d.instance.Status.DryRun.Message
	if !strings.Contains(message, "longer than") {
		t.Errorf("an hour-and-a-half old run is not flagged: %s", message)
	}

	// The underlying reason must survive: it is what someone acts on.
	if !strings.Contains(message, "waiting for test PVC db") {
		t.Errorf("the reason was lost: %s", message)
	}
}

func TestMirrorDoesNotFlagACompletedRun(t *testing.T) {
	// How long it took stops being interesting once the copy is up.
	d := dryRunInstance("", "")
	started := metav1.NewTime(time.Now().Add(-90 * time.Minute))
	d.instance.Status.DryRun.StartTime = &started

	d.mirrorDryRunStatus(&rmn.VolumeReplicationGroup{
		Status: rmn.VolumeReplicationGroupStatus{
			DryRun: &rmn.DryRunStatus{State: rmn.DryRunBuildCompleted},
		},
	})

	if strings.Contains(d.instance.Status.DryRun.Message, "longer than") {
		t.Errorf("a completed run was flagged as overdue: %s", d.instance.Status.DryRun.Message)
	}
}

func TestDryRunLeavesTheRealWorkloadsConditionsAlone(t *testing.T) {
	// Available and Protected describe the real action and the real
	// workload, neither of which a test failover changes. Reporting a stall
	// through Available would tell a user their DR is broken when it is not.
	d := dryRunInstance("", "")
	d.instance.Status.Conditions = []metav1.Condition{
		{Type: rmn.ConditionAvailable, Status: metav1.ConditionTrue, Reason: "Completed"},
		{Type: rmn.ConditionProtected, Status: metav1.ConditionTrue, Reason: "Protected"},
	}

	started := metav1.NewTime(time.Now().Add(-90 * time.Minute))
	d.instance.Status.DryRun.StartTime = &started

	d.mirrorDryRunStatus(&rmn.VolumeReplicationGroup{
		Status: rmn.VolumeReplicationGroupStatus{
			DryRun: &rmn.DryRunStatus{
				State:   rmn.DryRunBuildInProgress,
				Message: "stalled",
			},
		},
	})

	for _, condition := range d.instance.Status.Conditions {
		if condition.Status != metav1.ConditionTrue {
			t.Errorf("condition %s was changed to %s by a stalled dry run",
				condition.Type, condition.Status)
		}
	}
}

func TestDryRunRebuildsAPlanThatWentMissing(t *testing.T) {
	// Writing the annotation decodes the server's copy of the DRPC back over
	// the in-memory one, and the server's copy carries the status that was
	// last persisted -- not the plan just computed. That used to leave a run
	// in progress with nothing recorded about it, and the next reconcile
	// dereferenced it.
	//
	// Everything in the plan is derived and none of it moves mid-run, so it
	// is rebuilt rather than treated as fatal.
	d := dryRunInstance("", "")
	d.instance.Status.DryRun = nil
	d.instance.Status.PreferredDecision.ClusterName = testHomeCluster

	status, err := d.dryRunStatus()
	if err != nil {
		t.Fatalf("a missing plan was not recovered: %v", err)
	}

	if status.TargetCluster != testTargetCluster {
		t.Errorf("rebuilt plan targets %q, want %q", status.TargetCluster, testTargetCluster)
	}

	if len(status.NamespaceMapping) == 0 {
		t.Error("rebuilt plan has no namespace mapping")
	}
}

func TestDryRunKeepsTheRecordedPlanWhenItIsThere(t *testing.T) {
	// Rebuilding is a recovery path, not the normal one: a run must keep
	// reading back what it started with, so it cannot retarget itself if
	// cluster roles shift underneath it.
	d := dryRunInstance("", "")
	d.instance.Status.DryRun.RunID = "original"
	d.instance.Status.PreferredDecision.ClusterName = testTargetCluster

	status, err := d.dryRunStatus()
	if err != nil {
		t.Fatal(err)
	}

	if status.RunID != "original" {
		t.Errorf("recorded plan was replaced: run ID is %q", status.RunID)
	}

	if status.TargetCluster != testTargetCluster {
		t.Errorf("recorded target was replaced with %q", status.TargetCluster)
	}
}

func TestDryRunRebuildNeedsProtectedNamespaces(t *testing.T) {
	// Without them there is nothing to derive a mapping from, and guessing
	// would build a test copy somewhere nobody asked for.
	d := dryRunInstance("", "")
	d.instance.Status.DryRun = nil
	d.instance.Spec.ProtectedNamespaces = nil
	d.instance.Status.PreferredDecision.ClusterName = testHomeCluster

	if _, err := d.dryRunStatus(); err == nil {
		t.Error("rebuilt a plan with no protected namespaces to build it from")
	}
}

// discardInstance is a VRG on the cluster that built a test copy, now being
// demoted: the point at which the restored PVCs have to go.
func discardInstance(t *testing.T, state rmn.ReplicationState, pvcNamespaces ...string) (
	*VRGInstance, client.Client,
) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	pvcs := make([]corev1.PersistentVolumeClaim, 0, len(pvcNamespaces))
	objects := make([]client.Object, 0, len(pvcNamespaces))

	for i, namespace := range pvcNamespaces {
		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pvc-%d", i), Namespace: namespace},
		}
		pvcs = append(pvcs, pvc)
		objects = append(objects, pvc.DeepCopy())
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

	return &VRGInstance{
		ctx:        context.Background(),
		log:        logr.Discard(),
		reconciler: &VolumeReplicationGroupReconciler{Client: c},
		instance: &rmn.VolumeReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vrg", Namespace: "ramen-ops"},
			Spec:       rmn.VolumeReplicationGroupSpec{ReplicationState: state},
			Status: rmn.VolumeReplicationGroupStatus{
				DryRun: &rmn.DryRunStatus{
					RunID:            "abc12345",
					NamespaceMapping: map[string]string{"workload1": "workload1-test-abc12345"},
				},
			},
		},
		volRepPVCs: pvcs,
	}, c
}

func pvcExists(t *testing.T, c client.Client, namespace, name string) bool {
	t.Helper()

	err := c.Get(context.Background(),
		types.NamespacedName{Name: name, Namespace: namespace}, &corev1.PersistentVolumeClaim{})

	return err == nil
}

func TestDryRunDiscardsThePVCsItRestored(t *testing.T) {
	// Demoting waits for a PVC to be marked for deletion, which for a
	// discovered app means waiting for the user to remove the workload. On a
	// test cluster the user never deployed one, so without this the DRPC
	// sits in WaitOnUserToCleanUp for good.
	v, c := discardInstance(t, rmn.Secondary, "workload1")

	if err := v.dryRunDiscardRestoredPVCs(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pvcExists(t, c, "workload1", "pvc-0") {
		t.Error("a PVC the dry run restored was left behind, blocking the demote")
	}
}

func TestDryRunNeverDiscardsTheRealWorkloadsPVCs(t *testing.T) {
	// The dry-run annotation is copied onto every VRG the DRPC manages,
	// including the one running the real workload. Only status.dryRun marks
	// the cluster that actually built a test copy, and getting that wrong
	// here deletes the running application's volumes.
	v, c := discardInstance(t, rmn.Secondary, "workload1")
	v.instance.Status.DryRun = nil

	if err := v.dryRunDiscardRestoredPVCs(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !pvcExists(t, c, "workload1", "pvc-0") {
		t.Error("deleted a PVC on a cluster that never built a test copy")
	}
}

func TestDryRunKeepsRestoredPVCsWhenPromoting(t *testing.T) {
	// Clearing the flag with a failover to this cluster promotes it. These
	// stop being scaffolding and become the workload's own volumes.
	v, c := discardInstance(t, rmn.Primary, "workload1")

	if err := v.dryRunDiscardRestoredPVCs(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !pvcExists(t, c, "workload1", "pvc-0") {
		t.Error("deleted the volumes of a workload being promoted to a real failover")
	}
}

func TestDryRunOnlyDiscardsProtectedNamespaces(t *testing.T) {
	// A PVC outside the namespaces this run restored into is not its
	// business, whatever else is going on in the cluster.
	v, c := discardInstance(t, rmn.Secondary, "workload1", "somebody-elses-namespace")

	if err := v.dryRunDiscardRestoredPVCs(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pvcExists(t, c, "workload1", "pvc-0") {
		t.Error("restored PVC was not discarded")
	}

	if !pvcExists(t, c, "somebody-elses-namespace", "pvc-1") {
		t.Error("deleted a PVC outside the namespaces this run restored into")
	}
}

func autoResyncInstance(phase rmn.DryRunPhase, action rmn.VRGAction) *VRGInstance {
	annotations := map[string]string{}
	if phase != "" {
		annotations[rmn.DryRunAnnotation] = string(phase)
	}

	return &VRGInstance{
		log: logr.Discard(),
		instance: &rmn.VolumeReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
			Spec:       rmn.VolumeReplicationGroupSpec{Action: action},
		},
	}
}

func TestAutoResyncAfterATestFailoverPromotedTheVolumes(t *testing.T) {
	// A test failover promotes this cluster's volumes to snapshot them, and
	// that divergence is what makes a resync necessary. The need outlives
	// the run: the cluster only reaches Completed once it is already
	// Secondary, so treating that phase as "over" would switch auto-resync
	// off at the exact moment it is wanted.
	for _, phase := range []rmn.DryRunPhase{
		rmn.DryRunSettingUp, rmn.DryRunReverting, rmn.DryRunCompleted,
	} {
		t.Run(string(phase), func(t *testing.T) {
			v := autoResyncInstance(phase, "")

			if !v.autoResync(volrep.Secondary) {
				t.Errorf("auto-resync disabled at phase %q, leaving the volumes diverged", phase)
			}
		})
	}
}

func TestAutoResyncNeedsSecondary(t *testing.T) {
	// Nothing to resync while this cluster is still the one being written to.
	v := autoResyncInstance(rmn.DryRunReverting, "")

	if v.autoResync(volrep.Primary) {
		t.Error("auto-resync enabled on a Primary")
	}
}

func TestAutoResyncUnchangedWithoutADryRun(t *testing.T) {
	// The pre-existing rule still holds where no test failover was involved.
	if v := autoResyncInstance("", rmn.VRGActionFailover); !v.autoResync(volrep.Secondary) {
		t.Error("auto-resync disabled for a real failover")
	}

	if v := autoResyncInstance("", rmn.VRGActionRelocate); v.autoResync(volrep.Secondary) {
		t.Error("auto-resync enabled for a relocate with no test failover")
	}
}

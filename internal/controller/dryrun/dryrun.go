// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rmn "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/kubeobjects"
	"github.com/ramendr/ramen/internal/controller/util"
)

// This package runs on the cluster a test failover has promoted. The hub holds
// it Primary, with spec.dryRun set, until Reconcile reports the test copy
// built; it then returns the cluster to Secondary, and the copy is left
// running for the user to inspect and eventually delete.
//
// Everything here is deliberately separate from the production restore path.
// Nothing in internal/controller changes behavior because of a dry run except
// the single branch in processAsPrimary that routes here, which is also what
// keeps the real application from being restored on the test cluster.
//
// Two rules shape the whole package:
//
//   - Nothing it creates in a test namespace may be reachable from VRG
//     teardown. No ownerReference to the VRG, and no label a cleanup path
//     selects on: the copy has to outlive both the VRG and the DRPC.
//   - There is no failure state. Anything that cannot be done yet leaves the
//     state InProgress, records why, and asks to be called again. Nothing
//     times out and nothing is cleaned up automatically, so a stalled run
//     keeps its evidence for whoever comes to look.

// Input is everything the test copy is built from. It is assembled by the
// adapter in package controllers; this package reaches for nothing else, which
// is what keeps it out of the production reconcile's way.
type Input struct {
	Client    client.Client
	APIReader client.Reader
	Log       logr.Logger

	// VRG is the instance being reconciled, Primary with spec.dryRun set.
	VRG *rmn.VolumeReplicationGroup

	// DRPCUID identifies the DRPC driving this run. Both clusters derive the
	// run identity from it, so they agree on the test namespace names without
	// anything being carried between them. The hub stamps it on the VRG; the
	// adapter reads it from there, so the annotation key stays owned by one
	// package.
	DRPCUID string

	// RecipeElements supplies the restore workflow whose group steps are
	// replayed into the test namespaces.
	RecipeElements util.RecipeElements

	// VolRepPVCs are the protected PVCs. Each is snapshotted and handed to the
	// matching test namespace.
	VolRepPVCs []corev1.PersistentVolumeClaim

	// CephFSPVCs names any protected PVC backed by CephFS. Resolving a
	// StorageClass to a provisioner is the VRG's job, so the verdict is passed
	// in rather than recomputed here; see Eligible.
	CephFSPVCs []string

	// Capture locates the kube objects capture to restore from. Nil until the
	// adapter has read the source VRG out of S3, which is a reason to wait
	// rather than a reason to fail.
	Capture *Capture

	// KubeObjects creates the Velero requests for the restore.
	KubeObjects kubeobjects.RequestsManager

	// VeleroNamespace is where those requests are created.
	VeleroNamespace string
}

// Capture is the kube objects capture a test copy is restored from: the same
// one a real failover would use, read from the same store.
type Capture struct {
	S3Profile S3Profile

	// Path is the S3 key prefix the capture was written under.
	Path string

	// NamePrefix is the prefix shared by the capture's request names. Both
	// come from the shared helper in package kubeobjects, so the names this
	// side looks for cannot drift from the names the capture side wrote.
	NamePrefix string
}

// S3Profile is the slice of a store accessor this package needs. It is a local
// copy rather than an import so that the package does not depend on
// internal/controller, which depends on it.
type S3Profile struct {
	Name           string
	Endpoint       string
	Bucket         string
	Region         string
	SecretKeyRef   *corev1.SecretKeySelector
	CACertificates []byte
}

// Output is what the caller writes back to VRG status, plus whether to be
// called again.
type Output struct {
	State            rmn.DryRunState
	RunID            string
	NamespaceMapping map[string]string

	// Message says why a run that is still InProgress has not advanced. With
	// no timeout and no failure state this is the only signal a stalled run
	// gives, so it names the resource being waited on.
	Message string

	Requeue bool
}

// inProgress reports a run that cannot advance yet. The reason is logged as
// well as returned, because the two audiences differ: the message reaches a
// DRPC user through status, the log reaches whoever is debugging the managed
// cluster.
func inProgress(in Input, runID string, mapping map[string]string, reason string) Output {
	in.Log.Info("Dry run waiting", "reason", reason)

	return Output{
		State:            rmn.DryRunBuildInProgress,
		RunID:            runID,
		NamespaceMapping: mapping,
		Message:          reason,
		Requeue:          true,
	}
}

// Reconcile builds the test copy: test namespaces, snapshot-backed PVCs in
// them, and the application's kube objects restored into them. It is
// idempotent and safe to call on every reconcile.
//
// It never reports failure. Anything it cannot do yet comes back InProgress
// with a reason.
func Reconcile(ctx context.Context, in Input) (Output, error) {
	runID := RunID(in.DRPCUID)

	// Bind the run to the logger once. Every line below, and every line in
	// everything this calls, carries it -- so a single run can be followed
	// across both clusters with one grep.
	in.Log = in.Log.WithValues("runID", runID)

	in.Log.Info("Dry run building the test copy",
		"protectedNamespaces", sourceNamespaces(in.VRG), "pvcs", len(in.VolRepPVCs))

	if reason := Eligible(in); reason != "" {
		return inProgress(in, runID, nil, reason), nil
	}

	mapping, err := NamespaceMapping(sourceNamespaces(in.VRG), runID)
	if err != nil {
		return inProgress(in, runID, nil, err.Error()), nil
	}

	in.Log.Info("Dry run stage 1/4 creating the test namespaces", "mapping", mapping)

	if err := ensureTestNamespaces(ctx, in, runID, mapping); err != nil {
		return inProgress(in, runID, mapping, err.Error()), nil
	}

	in.Log.Info("Dry run stage 2/4 handing the volumes to the test namespaces")

	pending, err := ensureTestVolumes(ctx, in, runID, mapping)
	if err != nil {
		return inProgress(in, runID, mapping, err.Error()), nil
	}

	// Every test PVC must be Bound before the kube objects are restored: a
	// workload restored against a claim that never binds sits Pending with
	// nothing to say why.
	if len(pending) > 0 {
		return inProgress(in, runID, mapping, strings.Join(pending, "; ")), nil
	}

	in.Log.Info("Dry run stage 3/4 test volumes are bound, restoring the kube objects")

	pending, err = restoreKubeObjects(ctx, in, runID, mapping)
	if err != nil {
		return inProgress(in, runID, mapping, err.Error()), nil
	}

	if len(pending) > 0 {
		return inProgress(in, runID, mapping, strings.Join(pending, "; ")), nil
	}

	in.Log.Info("Dry run stage 4/4 kube objects restored")
	in.Log.Info("Dry run test copy is ready", "namespaces", mapping)

	return Output{
		State:            rmn.DryRunBuildCompleted,
		RunID:            runID,
		NamespaceMapping: mapping,
		Requeue:          false,
	}, nil
}

// sourceNamespaces are the namespaces the test copy mirrors. The scope is
// discovered applications, so protectedNamespaces is always populated; the
// fallback keeps this honest if that ever stops being true.
func sourceNamespaces(vrg *rmn.VolumeReplicationGroup) []string {
	if vrg.Spec.ProtectedNamespaces != nil && len(*vrg.Spec.ProtectedNamespaces) > 0 {
		return *vrg.Spec.ProtectedNamespaces
	}

	return []string{vrg.Namespace}
}

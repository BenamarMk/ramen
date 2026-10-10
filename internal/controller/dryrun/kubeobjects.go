// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"

	"github.com/ramendr/ramen/internal/controller/kubeobjects"
)

// The kube objects are restored by Velero, from the same captures a real
// failover would use, with a namespace mapping that redirects every one of
// them into its test namespace.
//
// This path deliberately does not go through the production restore in
// package controllers. Sharing it would mean threading a "which run is this"
// discrimination through the naming, the labeling and the resource modifier
// selection -- all in the code a real failover depends on. Here the cost of
// keeping them apart is a loop; the benefit is that a test failover cannot
// regress a real one.

// restoreKubeObjects replays the recipe's restore workflow into the test
// namespaces. It returns the reasons it is not finished, empty when every
// group has been restored.
func restoreKubeObjects(
	ctx context.Context, in Input, runID string, mapping map[string]string,
) ([]string, error) {
	if in.Capture == nil {
		return []string{"waiting to locate the kube objects capture in S3"}, nil
	}

	modifier, err := ensureResourceModifier(ctx, in, runID)
	if err != nil {
		return nil, err
	}

	captures, err := captureRequests(ctx, in, runID)
	if err != nil {
		return nil, err
	}

	restores, err := restoreRequests(ctx, in, runID)
	if err != nil {
		return nil, err
	}

	var pending []string

	for group, spec := range in.RecipeElements.RecoverWorkflow {
		// Hooks are run by Ramen rather than Velero, so a namespace mapping
		// does not reach them: an unremapped hook would act on the production
		// namespace. Running them against a test copy is deferred, and the
		// built-in VM recipe has no restore hooks, so nothing is lost today.
		if spec.IsHook {
			continue
		}

		reason, err := restoreGroup(ctx, in, runID, group, spec, mapping, modifier, captures, restores)
		if err != nil {
			return nil, err
		}

		if reason != "" {
			pending = append(pending, reason)
		}
	}

	return pending, nil
}

// captureRequests are the Velero Backup objects this run has already asked
// for. Velero restores from a Backup object, so a capture taken on the other
// cluster has to be re-presented here before it can be restored from; the
// first pass creates that stand-in and a later pass finds it ready.
func captureRequests(ctx context.Context, in Input, runID string) (map[string]kubeobjects.Request, error) {
	requests, err := in.KubeObjects.ProtectRequestsGet(
		ctx, in.APIReader, in.VeleroNamespace, RequestLabels(runID))
	if err != nil {
		return nil, fmt.Errorf("reading dry run capture requests: %w", err)
	}

	return kubeobjects.RequestsMapKeyedByName(requests), nil
}

// restoreRequests are the Velero Restore objects this run has already
// submitted, read back from the cluster.
//
// Reading them back is the only way to learn how a restore is going.
// Submitting one hands back the object just constructed, which has no status
// on it yet, so asking that whether the restore finished answers "not yet"
// forever however long Velero has been done.
func restoreRequests(ctx context.Context, in Input, runID string) (map[string]kubeobjects.Request, error) {
	requests, err := in.KubeObjects.RecoverRequestsGet(
		ctx, in.APIReader, in.VeleroNamespace, RequestLabels(runID))
	if err != nil {
		return nil, fmt.Errorf("reading dry run restore requests: %w", err)
	}

	return kubeobjects.RequestsMapKeyedByName(requests), nil
}

func restoreGroup(
	ctx context.Context, in Input, runID string, group int, spec kubeobjects.RecoverSpec,
	mapping map[string]string, modifier string,
	captures, restores map[string]kubeobjects.Request,
) (string, error) {
	profile := in.Capture.S3Profile
	captureName := kubeobjects.CaptureName(in.Capture.NamePrefix, spec.BackupName, profile.Name)
	restoreName := restoreRequestName(in.VRG.GetName(), runID, group)

	// Already submitted: judge it on what the cluster says, not on what the
	// submission handed back.
	if request, submitted := restores[restoreName]; submitted {
		return restoreProgress(in, restoreName, request), nil
	}

	// The mapping is what redirects the restore. The spec's includedNamespaces
	// stay as the source namespaces: those are the keys Velero matches the
	// mapping against, and the capture itself is untouched by any of this.
	spec.NamespaceMapping = mapping
	spec.ResourceModifier = &corev1.TypedLocalObjectReference{
		Kind: "ConfigMap",
		Name: modifier,
	}

	request, err := in.KubeObjects.RecoverRequestCreate(
		ctx, in.Client, in.Log,
		profile.Endpoint, profile.Bucket, profile.Region, in.Capture.Path,
		profile.SecretKeyRef, profile.CACertificates,
		spec, in.VeleroNamespace,
		captureName, captures[captureName],
		restoreName,
		RequestLabels(runID), map[string]string{},
	)
	if err != nil {
		return "", fmt.Errorf("restoring group %s: %w", spec.BackupName, err)
	}

	if request == nil {
		// Velero restores from a Backup object, so a capture taken on the
		// other cluster has to be re-presented here first. That is what this
		// pass asked for.
		return fmt.Sprintf("waiting for capture %s to be readable", captureName), nil
	}

	return fmt.Sprintf("restore %s submitted", restoreName), nil
}

// restoreProgress turns what Velero says about a restore into a reason to keep
// waiting, or "" when it is done.
func restoreProgress(in Input, restoreName string, request kubeobjects.Request) string {
	err := request.Status(in.Log)
	if err == nil {
		return ""
	}

	// A restore still running is not a problem. One that failed is not
	// either, in the sense that it must not fail the run: the reason is
	// surfaced and the run waits for a human, keeping the evidence rather
	// than tearing it down.
	if errors.Is(err, kubeobjects.RequestProcessingError{}) {
		return fmt.Sprintf("restore %s: %s", restoreName, err.Error())
	}

	return fmt.Sprintf("restore %s needs attention: %s", restoreName, err.Error())
}

// restoreRequestName keeps this run's Velero requests out of the production
// restore flow's way. They are labeled distinctly too, so neither flow's
// cleanup can see the other's.
func restoreRequestName(vrgName, runID string, group int) string {
	return "dryrun-" + vrgName + "-" + runID + "-" + strconv.Itoa(group)
}

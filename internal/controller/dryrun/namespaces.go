// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// NamespaceOfLabel names the protected namespace a test namespace mirrors.
	NamespaceOfLabel = "ramendr.openshift.io/dry-run-namespace-of"

	// RunIDLabel ties everything a run created together.
	RunIDLabel = "ramendr.openshift.io/dry-run-id"

	// DRPCUIDLabel identifies the DRPC the run belongs to, so a test copy can
	// still be traced back after the DRPC itself is gone.
	DRPCUIDLabel = "ramendr.openshift.io/dry-run-drpc-uid"

	// podSecurityLabelPrefix marks the Pod Security Admission labels. Without
	// them a restricted workload is rejected by the API server on admission,
	// and the test copy fails for a reason that has nothing to do with DR.
	podSecurityLabelPrefix = "pod-security.kubernetes.io/"
)

// These labels identify a test namespace and its contents. They are for
// humans and for the user's eventual cleanup -- no Ramen code selects on them
// to delete anything, and nothing here carries an ownerReference to the VRG.
// The test copy has to outlive both the VRG and the DRPC.
func runLabels(in Input, runID, sourceNamespace string) map[string]string {
	return map[string]string{
		NamespaceOfLabel: sourceNamespace,
		RunIDLabel:       runID,
		DRPCUIDLabel:     in.DRPCUID,
	}
}

// policyObjectLists are the namespaced objects copied into a test namespace so
// the test workload behaves like the real one.
//
// ServiceAccounts are absent deliberately: they arrive in the Velero backup.
// Annotations are not copied at all, which is also how the cluster-assigned
// openshift.io/sa.scc.* range is left alone -- copying those collides with
// what the cluster assigns the new namespace.
func policyObjectLists() []client.ObjectList {
	return []client.ObjectList{
		&networkingv1.NetworkPolicyList{},
		&corev1.ResourceQuotaList{},
		&corev1.LimitRangeList{},
		&rbacv1.RoleList{},
		&rbacv1.RoleBindingList{},
	}
}

// ensureTestNamespaces creates the test namespaces that are missing, adopts
// any already there, and copies each source namespace's policy objects in.
//
// Adoption rather than reconciliation is deliberate: on a repeat run over
// namespaces the user never deleted, this leaves the earlier copy as it is.
func ensureTestNamespaces(ctx context.Context, in Input, runID string, mapping map[string]string) error {
	for source, target := range mapping {
		in.Log.Info("Preparing test namespace", "source", source, "target", target)

		if err := ensureTestNamespace(ctx, in, runID, source, target); err != nil {
			return err
		}

		if err := copyPolicyObjects(ctx, in, runID, source, target); err != nil {
			return err
		}

		in.Log.Info("Test namespace ready", "namespace", target)
	}

	return nil
}

func ensureTestNamespace(ctx context.Context, in Input, runID, source, target string) error {
	labels := runLabels(in, runID, source)

	sourceNamespace := &corev1.Namespace{}
	if err := in.APIReader.Get(ctx, client.ObjectKey{Name: source}, sourceNamespace); err != nil {
		return fmt.Errorf("reading protected namespace %s: %w", source, err)
	}

	for key, value := range sourceNamespace.GetLabels() {
		if strings.HasPrefix(key, podSecurityLabelPrefix) {
			labels[key] = value
		}
	}

	err := in.Client.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: target, Labels: labels},
	})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating test namespace %s: %w", target, err)
	}

	if k8serrors.IsAlreadyExists(err) {
		in.Log.Info("Adopting an existing test namespace; its contents are left as they are",
			"namespace", target)
	} else {
		in.Log.Info("Created test namespace", "namespace", target, "for", source,
			"podSecurity", podSecurityLabels(labels))
	}

	return nil
}

// copyPolicyObjects reproduces the source namespace's policy posture in the
// test namespace. What is copied is listed in policyObjectLists.
func copyPolicyObjects(ctx context.Context, in Input, runID, source, target string) error {
	copied := 0

	for _, list := range policyObjectLists() {
		if err := in.APIReader.List(ctx, list, client.InNamespace(source)); err != nil {
			return fmt.Errorf("listing %T in %s: %w", list, source, err)
		}

		items, err := meta.ExtractList(list)
		if err != nil {
			return fmt.Errorf("extracting %T: %w", list, err)
		}

		for _, item := range items {
			obj, ok := item.(client.Object)
			if !ok {
				continue
			}

			if err := copyPolicyObject(ctx, in, runID, source, target, obj); err != nil {
				return err
			}

			copied++
		}
	}

	in.Log.Info("Copied the source namespace's policy objects",
		"from", source, "to", target, "count", copied)

	return nil
}

func copyPolicyObject(ctx context.Context, in Input, runID, source, target string, obj client.Object) error {
	retargetObject(obj, target)
	rewriteRoleBindingSubjects(obj, source, target)

	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	for key, value := range runLabels(in, runID, source) {
		labels[key] = value
	}

	obj.SetLabels(labels)

	err := in.Client.Create(ctx, obj)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("copying %T %s into %s: %w", obj, obj.GetName(), target, err)
	}

	in.Log.V(1).Info("Copied policy object", "kind", fmt.Sprintf("%T", obj),
		"name", obj.GetName(), "namespace", target, "alreadyThere", err != nil)

	return nil
}

// podSecurityLabels are the admission labels carried over, logged because a
// test copy that is rejected on admission fails for a reason that looks
// nothing like DR.
func podSecurityLabels(labels map[string]string) map[string]string {
	carried := map[string]string{}

	for key, value := range labels {
		if strings.HasPrefix(key, podSecurityLabelPrefix) {
			carried[key] = value
		}
	}

	return carried
}

// retargetObject strips the identity a fetched object carries so it can be
// created fresh elsewhere. Clearing ownerReferences matters most: they would
// point at owners in the source namespace that do not own this copy, and the
// garbage collector would delete it.
func retargetObject(obj client.Object, target string) {
	obj.SetNamespace(target)
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetGeneration(0)
	obj.SetCreationTimestamp(metav1.Time{})
	obj.SetOwnerReferences(nil)
	obj.SetManagedFields(nil)
	obj.SetFinalizers(nil)
}

// rewriteRoleBindingSubjects repoints subjects that named the source
// namespace. A binding left pointing at the source would grant the real
// workload's ServiceAccounts rights in the test namespace, and leave the test
// copy's own accounts with none.
func rewriteRoleBindingSubjects(obj client.Object, source, target string) {
	binding, ok := obj.(*rbacv1.RoleBinding)
	if !ok {
		return
	}

	for i := range binding.Subjects {
		if binding.Subjects[i].Namespace == source {
			binding.Subjects[i].Namespace = target
		}
	}
}

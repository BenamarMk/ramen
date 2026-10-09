// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun_test

import (
	"context"
	"testing"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rmn "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/dryrun"
)

const sourceNS = "workload1"

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	if err := rmn.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	if err := snapv1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	return scheme
}

// sourceWorld is a protected namespace carrying the policy posture a test copy
// is expected to reproduce.
func sourceWorld() []client.Object {
	return []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: sourceNS,
			Labels: map[string]string{
				"pod-security.kubernetes.io/enforce": "restricted",
				"pod-security.kubernetes.io/audit":   "restricted",
				"kubernetes.io/metadata.name":        sourceNS,
				"unrelated":                          "keep-out",
			},
			Annotations: map[string]string{
				"openshift.io/sa.scc.uid-range": "1000680000/10000",
			},
		}},
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
			Name: "deny-all", Namespace: sourceNS,
		}},
		&corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{
			Name: "compute", Namespace: sourceNS,
		}},
		&corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{
			Name: "limits", Namespace: sourceNS,
		}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{
			Name: "app", Namespace: sourceNS,
		}},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: sourceNS},
			Subjects: []rbacv1.Subject{
				{Kind: "ServiceAccount", Name: "app", Namespace: sourceNS},
				{Kind: "ServiceAccount", Name: "monitor", Namespace: "observability"},
			},
			RoleRef: rbacv1.RoleRef{Kind: "Role", Name: "app"},
		},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
			Name: "app", Namespace: sourceNS,
		}},
	}
}

func reconcileInto(t *testing.T, extra ...client.Object) (client.Client, string) {
	t.Helper()

	objects := append(sourceWorld(), extra...)
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(objects...).Build()

	in := testInput(t, []string{sourceNS}, "db")
	in.Client = c
	in.APIReader = c

	out, err := dryrun.Reconcile(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	target := out.NamespaceMapping[sourceNS]
	if target == "" {
		t.Fatalf("no target namespace reported: %+v", out)
	}

	return c, target
}

func TestTestNamespaceCarriesTheRunLabels(t *testing.T) {
	c, target := reconcileInto(t)

	ns := &corev1.Namespace{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: target}, ns); err != nil {
		t.Fatalf("test namespace not created: %v", err)
	}

	for key, want := range map[string]string{
		dryrun.NamespaceOfLabel: sourceNS,
		dryrun.RunIDLabel:       dryrun.RunID(testDRPCUID),
		dryrun.DRPCUIDLabel:     testDRPCUID,
	} {
		if got := ns.Labels[key]; got != want {
			t.Errorf("label %s = %q, want %q", key, got, want)
		}
	}

	// Nothing may tie the test copy to the VRG: it has to outlive both the
	// VRG and the DRPC.
	if len(ns.OwnerReferences) != 0 {
		t.Errorf("test namespace has owner references: %v", ns.OwnerReferences)
	}
}

func TestTestNamespaceInheritsPodSecurityButNotTheRest(t *testing.T) {
	c, target := reconcileInto(t)

	ns := &corev1.Namespace{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: target}, ns); err != nil {
		t.Fatal(err)
	}

	// Without these a restricted workload is rejected on admission, and the
	// test fails for a reason that has nothing to do with DR.
	if ns.Labels["pod-security.kubernetes.io/enforce"] != "restricted" ||
		ns.Labels["pod-security.kubernetes.io/audit"] != "restricted" {
		t.Errorf("pod security labels not carried over: %v", ns.Labels)
	}

	if _, copied := ns.Labels["unrelated"]; copied {
		t.Error("copied a label that is not pod security related")
	}

	// The SCC range is assigned by the cluster per namespace; copying the
	// source's would collide with what this namespace was given.
	if _, copied := ns.Annotations["openshift.io/sa.scc.uid-range"]; copied {
		t.Error("copied the cluster-assigned SCC annotation")
	}
}

func TestPolicyObjectsAreCopied(t *testing.T) {
	c, target := reconcileInto(t)
	ctx := context.Background()
	key := func(name string) client.ObjectKey {
		return client.ObjectKey{Name: name, Namespace: target}
	}

	for name, obj := range map[string]client.Object{
		"networkpolicy": &networkingv1.NetworkPolicy{},
		"resourcequota": &corev1.ResourceQuota{},
		"limitrange":    &corev1.LimitRange{},
		"role":          &rbacv1.Role{},
		"rolebinding":   &rbacv1.RoleBinding{},
	} {
		objName := map[string]string{
			"networkpolicy": "deny-all", "resourcequota": "compute",
			"limitrange": "limits", "role": "app", "rolebinding": "app",
		}[name]

		if err := c.Get(ctx, key(objName), obj); err != nil {
			t.Errorf("%s not copied: %v", name, err)
		}
	}

	// ServiceAccounts arrive in the Velero backup; copying them here would
	// collide with the restore.
	sa := &corev1.ServiceAccount{}
	if err := c.Get(ctx, key("app"), sa); err == nil {
		t.Error("ServiceAccount was copied but should come from the backup")
	}
}

func TestRoleBindingSubjectsAreRepointed(t *testing.T) {
	c, target := reconcileInto(t)

	binding := &rbacv1.RoleBinding{}
	if err := c.Get(context.Background(),
		client.ObjectKey{Name: "app", Namespace: target}, binding); err != nil {
		t.Fatal(err)
	}

	// A subject left naming the source would grant the real workload's
	// accounts rights here, and leave the copy's own accounts with none.
	if binding.Subjects[0].Namespace != target {
		t.Errorf("subject in the source namespace was not repointed: %v", binding.Subjects[0])
	}

	// A subject naming some third namespace is not ours to rewrite.
	if binding.Subjects[1].Namespace != "observability" {
		t.Errorf("unrelated subject was rewritten: %v", binding.Subjects[1])
	}
}

func TestCopiedObjectsLoseTheirOriginalIdentity(t *testing.T) {
	c, target := reconcileInto(t)

	policy := &networkingv1.NetworkPolicy{}
	if err := c.Get(context.Background(),
		client.ObjectKey{Name: "deny-all", Namespace: target}, policy); err != nil {
		t.Fatal(err)
	}

	// Owner references would point at owners in the source namespace that do
	// not own this copy, and the garbage collector would delete it.
	if len(policy.OwnerReferences) != 0 {
		t.Errorf("copy carries owner references: %v", policy.OwnerReferences)
	}

	if policy.Labels[dryrun.RunIDLabel] != dryrun.RunID(testDRPCUID) {
		t.Errorf("copy is not labeled with the run: %v", policy.Labels)
	}
}

func TestExistingTestNamespaceIsAdoptedNotOverwritten(t *testing.T) {
	// A repeat run over namespaces the user never deleted leaves the earlier
	// copy as it is. This is the documented consequence of deriving the run
	// ID from the DRPC UID, and it belongs in the release notes.
	runID := dryrun.RunID(testDRPCUID)
	existing := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   dryrun.TargetNamespace(sourceNS, runID),
		Labels: map[string]string{"left": "from-the-previous-run"},
	}}

	c, target := reconcileInto(t, existing)

	ns := &corev1.Namespace{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: target}, ns); err != nil {
		t.Fatal(err)
	}

	if ns.Labels["left"] != "from-the-previous-run" {
		t.Errorf("an existing test namespace was overwritten: %v", ns.Labels)
	}
}

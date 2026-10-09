// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun_test

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	rmn "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/dryrun"
	"github.com/ramendr/ramen/internal/controller/kubeobjects"
	"github.com/ramendr/ramen/internal/controller/util"
)

// recordingRequests is a RequestsManager that records what the restore asked
// for, so the test can assert on the request rather than on Velero.
type recordingRequests struct {
	created  []kubeobjects.RecoverSpec
	names    []string
	complete bool
}

type stubRequest struct{}

func (r stubRequest) Object() client.Object    { return &corev1.ConfigMap{} }
func (r stubRequest) Name() string             { return "stub" }
func (r stubRequest) StartTime() metav1.Time   { return metav1.Now() }
func (r stubRequest) EndTime() metav1.Time     { return metav1.Now() }
func (r stubRequest) Status(logr.Logger) error { return nil }
func (r stubRequest) Deallocate(context.Context, client.Client, logr.Logger) error {
	return nil
}

func (m *recordingRequests) ProtectsPath() string                          { return "velero/backups/" }
func (m *recordingRequests) RecoversPath() string                          { return "velero/restores/" }
func (m *recordingRequests) ProtectRequestNew() kubeobjects.ProtectRequest { return stubRequest{} }
func (m *recordingRequests) RecoverRequestNew() kubeobjects.RecoverRequest { return stubRequest{} }

func (m *recordingRequests) ProtectRequestCreate(
	context.Context, client.Client, logr.Logger, string, string, string, string,
	*corev1.SecretKeySelector, []byte, kubeobjects.Spec, string, string,
	map[string]string, map[string]string,
) (kubeobjects.ProtectRequest, error) {
	return stubRequest{}, nil
}

func (m *recordingRequests) RecoverRequestCreate(
	_ context.Context, _ client.Client, _ logr.Logger,
	_, _, _, _ string, _ *corev1.SecretKeySelector, _ []byte,
	spec kubeobjects.RecoverSpec, _, _ string, _ kubeobjects.ProtectRequest,
	recoverName string, _, _ map[string]string,
) (kubeobjects.RecoverRequest, error) {
	m.created = append(m.created, spec)
	m.names = append(m.names, recoverName)

	if !m.complete {
		// Mirrors Velero's two-step dance: the first pass only asks for the
		// capture to be made readable.
		return nil, nil
	}

	return stubRequest{}, nil
}

func (m *recordingRequests) ProtectRequestsGet(
	context.Context, client.Reader, string, map[string]string,
) (kubeobjects.Requests, error) {
	return emptyRequests{}, nil
}

func (m *recordingRequests) RecoverRequestsGet(
	context.Context, client.Reader, string, map[string]string,
) (kubeobjects.Requests, error) {
	return emptyRequests{}, nil
}

func (m *recordingRequests) ProtectRequestsDelete(
	context.Context, client.Writer, string, map[string]string,
) error {
	return nil
}

func (m *recordingRequests) RecoverRequestsDelete(
	context.Context, client.Writer, string, map[string]string,
) error {
	return nil
}

func (m *recordingRequests) ProtectBSLsDelete(
	context.Context, client.Writer, string, map[string]string,
) error {
	return nil
}

type emptyRequests struct{}

func (emptyRequests) Count() int                  { return 0 }
func (emptyRequests) Get(int) kubeobjects.Request { return nil }

// restoreInput is a workload whose volumes are already in place, so the
// restore is the only thing left to do.
func restoreInput(t *testing.T, complete bool) (dryrun.Input, *recordingRequests, client.Client) {
	t.Helper()

	in, c := volumeInput(t)
	manager := &recordingRequests{complete: complete}

	in.KubeObjects = manager
	in.VeleroNamespace = "velero"
	in.Capture = &dryrun.Capture{
		S3Profile:  dryrun.S3Profile{Name: "s3profile", Bucket: "bucket"},
		Path:       "ramen-ops/test-vrg/kube-objects/0/",
		NamePrefix: "ramen-ops--test-vrg--0",
	}
	in.RecipeElements = util.RecipeElements{
		RecoverWorkflow: []kubeobjects.RecoverSpec{
			{BackupName: "vm-backup"},
			{Spec: kubeobjects.Spec{KubeResourcesSpec: kubeobjects.KubeResourcesSpec{IsHook: true}}},
		},
	}

	// The test PVC has to be Bound before the restore runs, or the workload
	// would come up against a claim that never binds.
	bindTestPVC(t, c, in)

	return in, manager, c
}

func bindTestPVC(t *testing.T, c client.Client, in dryrun.Input) {
	t.Helper()

	ctx := context.Background()
	target := dryrun.TargetNamespace(sourceNS, dryrun.RunID(testDRPCUID))

	// Provoke creation of the test PVC, then mark it Bound as a provisioner
	// would.
	if _, err := dryrun.Reconcile(ctx, in); err != nil {
		t.Fatal(err)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, client.ObjectKey{Name: testPVCName, Namespace: target}, pvc); err != nil {
		t.Fatalf("test PVC was not created: %v", err)
	}

	pvc.Status.Phase = corev1.ClaimBound
	if err := c.Status().Update(ctx, pvc); err != nil {
		t.Fatalf("binding the test PVC: %v", err)
	}
}

func TestRestoreRedirectsEveryNamespace(t *testing.T) {
	in, manager, _ := restoreInput(t, true)

	if _, err := dryrun.Reconcile(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	if len(manager.created) != 1 {
		t.Fatalf("expected one restore request, got %d", len(manager.created))
	}

	spec := manager.created[0]
	want := dryrun.TargetNamespace(sourceNS, dryrun.RunID(testDRPCUID))

	if spec.NamespaceMapping[sourceNS] != want {
		t.Errorf("mapping is %v, want %s -> %s", spec.NamespaceMapping, sourceNS, want)
	}
}

func TestRestoreSkipsHookSteps(t *testing.T) {
	// Hooks are run by Ramen, not Velero, so a namespace mapping does not
	// reach them: an unremapped hook would act on the production namespace.
	in, manager, _ := restoreInput(t, true)

	if _, err := dryrun.Reconcile(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	if len(manager.created) != 1 {
		t.Errorf("hook step was restored: %d requests for a workflow with 1 group and 1 hook",
			len(manager.created))
	}
}

func TestRestoreNamesRequestsForThisRunOnly(t *testing.T) {
	// Names and labels keep this run clear of the production restore flow, so
	// neither one's cleanup can see the other's requests.
	in, manager, _ := restoreInput(t, true)

	if _, err := dryrun.Reconcile(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(manager.names[0], "dryrun-") ||
		!strings.Contains(manager.names[0], dryrun.RunID(testDRPCUID)) {
		t.Errorf("restore request name %q is not scoped to this run", manager.names[0])
	}
}

func TestRestoreStripsOwnerReferences(t *testing.T) {
	// Restored objects carry owner references naming owners in the namespace
	// they came from. Those UIDs do not resolve here, and the garbage
	// collector deletes anything whose owner it cannot find -- so without
	// this the test copy quietly disappears after being restored.
	in, manager, c := restoreInput(t, true)

	if _, err := dryrun.Reconcile(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	ref := manager.created[0].ResourceModifier
	if ref == nil {
		t.Fatal("no resource modifier attached to the restore")
	}

	configMap := &corev1.ConfigMap{}
	if err := c.Get(context.Background(),
		client.ObjectKey{Name: ref.Name, Namespace: "velero"}, configMap); err != nil {
		t.Fatalf("resource modifier ConfigMap not created: %v", err)
	}

	// Velero insists on exactly one key.
	if len(configMap.Data) != 1 {
		t.Errorf("ConfigMap has %d keys, Velero accepts exactly 1", len(configMap.Data))
	}

	var rules struct {
		Version               string `json:"version"`
		ResourceModifierRules []struct {
			Conditions struct {
				GroupResource string `json:"groupResource"`
			} `json:"conditions"`
			MergePatches []struct {
				PatchData string `json:"patchData"`
			} `json:"mergePatches"`
		} `json:"resourceModifierRules"`
	}

	for _, data := range configMap.Data {
		if err := yaml.Unmarshal([]byte(data), &rules); err != nil {
			t.Fatalf("rules are not valid YAML: %v", err)
		}
	}

	if len(rules.ResourceModifierRules) != 1 {
		t.Fatalf("expected one rule, got %d", len(rules.ResourceModifierRules))
	}

	rule := rules.ResourceModifierRules[0]
	if rule.Conditions.GroupResource != "*" {
		t.Errorf("rule matches %q, want every kind", rule.Conditions.GroupResource)
	}

	// A merge patch, not a JSON patch remove: remove fails when the path is
	// absent, and most objects carry no owner references.
	if !strings.Contains(rule.MergePatches[0].PatchData, `"ownerReferences":null`) {
		t.Errorf("rule does not strip owner references: %s", rule.MergePatches[0].PatchData)
	}
}

func TestRestoreDoesNotApplyStaticIPTranslation(t *testing.T) {
	// The test VMs would otherwise claim the same addresses as the production
	// VMs still running on the other cluster.
	in, manager, _ := restoreInput(t, true)

	if _, err := dryrun.Reconcile(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	ref := manager.created[0].ResourceModifier
	if ref != nil && !strings.HasPrefix(ref.Name, "dryrun-") {
		t.Errorf("a non dry-run resource modifier was attached: %s", ref.Name)
	}
}

func TestRunCompletesOnceEverythingIsRestored(t *testing.T) {
	in, _, _ := restoreInput(t, true)

	out, err := dryrun.Reconcile(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}

	if out.State != rmn.DryRunBuildCompleted {
		t.Errorf("state %q, want Completed; message: %s", out.State, out.Message)
	}
}

func TestRunWaitsWhileTheCaptureIsNotReadableYet(t *testing.T) {
	// Velero restores from a Backup object, so a capture taken on the other
	// cluster is re-presented here first. Until that is ready there is
	// nothing to restore from.
	in, _, _ := restoreInput(t, false)

	out, err := dryrun.Reconcile(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}

	if out.State != rmn.DryRunBuildInProgress {
		t.Errorf("state %q, want InProgress", out.State)
	}
}

func TestRunWaitsWhenTheCaptureCannotBeLocated(t *testing.T) {
	in, _, _ := restoreInput(t, true)
	in.Capture = nil

	out, err := dryrun.Reconcile(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}

	if out.State != rmn.DryRunBuildInProgress {
		t.Errorf("state %q, want InProgress", out.State)
	}

	if !strings.Contains(out.Message, "capture") {
		t.Errorf("message does not explain the wait: %s", out.Message)
	}
}

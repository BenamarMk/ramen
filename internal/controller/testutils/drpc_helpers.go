// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rmn "github.com/ramendr/ramen/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// TestState manages global test state (to be replaced with TestContext)
type TestState struct {
	RestorePVs            bool
	ClusterDown           string
	ToggleUIDChecks       bool
	FakeSecondaryFor      string
	ProtectedPVCCount     int
	RunningVolSyncTests   bool
	UseApplicationSet     bool
}

// NewTestState creates a new TestState with default values
func NewTestState() *TestState {
	return &TestState{
		RestorePVs:          true,
		ClusterDown:         "",
		ToggleUIDChecks:     false,
		FakeSecondaryFor:    "",
		ProtectedPVCCount:   2,
		RunningVolSyncTests: false,
		UseApplicationSet:   false,
	}
}

// SetRestorePVsComplete marks PV restore as complete
func (s *TestState) SetRestorePVsComplete() {
	s.RestorePVs = true
}

// SetRestorePVsIncomplete marks PV restore as incomplete
func (s *TestState) SetRestorePVsIncomplete() {
	s.RestorePVs = false
}

// IsRestorePVsComplete returns whether PV restore is complete
func (s *TestState) IsRestorePVsComplete() bool {
	return s.RestorePVs
}

// SetClusterDown marks a cluster as down
func (s *TestState) SetClusterDown(clusterName string) {
	s.ClusterDown = clusterName
}

// ResetClusterDown resets the cluster down state
func (s *TestState) ResetClusterDown() {
	s.ClusterDown = ""
}

// SetToggleUIDChecks enables UID checks
func (s *TestState) SetToggleUIDChecks() {
	s.ToggleUIDChecks = true
}

// ResetToggleUIDChecks disables UID checks
func (s *TestState) ResetToggleUIDChecks() {
	s.ToggleUIDChecks = false
}

// GetFunctionNameAtIndex returns the function name at the given call stack index
func GetFunctionNameAtIndex(idx int) string {
	pc, _, _, _ := runtime.Caller(idx)
	data := runtime.FuncForPC(pc).Name()
	result := strings.Split(data, ".")

	return result[len(result)-1]
}

// WaitForDRPCPhase waits for DRPC to reach the expected phase
func WaitForDRPCPhase(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	expectedPhase rmn.DRState,
	timeout time.Duration,
) {
	gomega.Eventually(func() rmn.DRState {
		drpc := &rmn.DRPlacementControl{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, drpc)
		if err != nil {
			return ""
		}
		return drpc.Status.Phase
	}, timeout, time.Second).Should(gomega.Equal(expectedPhase))
}

// GetDRPCCondition returns the condition of the specified type from DRPC status
func GetDRPCCondition(status *rmn.DRPlacementControlStatus, conditionType string) (int, *metav1.Condition) {
	for i, condition := range status.Conditions {
		if condition.Type == conditionType {
			return i, &condition
		}
	}
	return -1, nil
}

// CreateNamespace creates a namespace using the provided client
func CreateNamespace(ctx context.Context, k8sClient client.Client, ns *corev1.Namespace) error {
	return k8sClient.Create(ctx, ns)
}

// DeleteNamespace deletes a namespace using the provided client
func DeleteNamespace(ctx context.Context, k8sClient client.Client, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	return client.IgnoreNotFound(k8sClient.Delete(ctx, ns))
}

// EnsureNamespaceExists ensures a namespace exists, creating it if necessary
func EnsureNamespaceExists(ctx context.Context, k8sClient client.Client, name string) error {
	ns := &corev1.Namespace{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, ns)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Namespace doesn't exist, create it
			return CreateNamespace(ctx, k8sClient, NewNamespaceBuilder(name).Build())
		}
		return err
	}
	return nil
}

// GetLatestDRPC retrieves the latest version of a DRPC
func GetLatestDRPC(ctx context.Context, k8sClient client.Client, name, namespace string) (*rmn.DRPlacementControl, error) {
	drpc := &rmn.DRPlacementControl{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, drpc)
	return drpc, err
}

// UpdateDRPCSpec updates the DRPC spec with the provided values
func UpdateDRPCSpec(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	preferredCluster, failoverCluster string,
	action rmn.DRAction,
) error {
	drpc, err := GetLatestDRPC(ctx, k8sClient, name, namespace)
	if err != nil {
		return err
	}

	drpc.Spec.PreferredCluster = preferredCluster
	drpc.Spec.FailoverCluster = failoverCluster
	drpc.Spec.Action = action

	return k8sClient.Update(ctx, drpc)
}

// ClearDRPCStatus clears the DRPC status for testing
func ClearDRPCStatus(ctx context.Context, k8sClient client.Client, name, namespace string) error {
	drpc, err := GetLatestDRPC(ctx, k8sClient, name, namespace)
	if err != nil {
		return err
	}

	drpc.Status = rmn.DRPlacementControlStatus{}
	return k8sClient.Status().Update(ctx, drpc)
}

// VerifyDRPCPhase verifies that DRPC is in the expected phase
func VerifyDRPCPhase(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	expectedPhase rmn.DRState,
) error {
	drpc, err := GetLatestDRPC(ctx, k8sClient, name, namespace)
	if err != nil {
		return err
	}

	if drpc.Status.Phase != expectedPhase {
		return fmt.Errorf("expected phase %s, got %s", expectedPhase, drpc.Status.Phase)
	}

	return nil
}

// VerifyDRPCCondition verifies that DRPC has the expected condition
func VerifyDRPCCondition(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	conditionType string,
	expectedStatus metav1.ConditionStatus,
) error {
	drpc, err := GetLatestDRPC(ctx, k8sClient, name, namespace)
	if err != nil {
		return err
	}

	_, condition := GetDRPCCondition(&drpc.Status, conditionType)
	if condition == nil {
		return fmt.Errorf("condition %s not found", conditionType)
	}

	if condition.Status != expectedStatus {
		return fmt.Errorf("expected condition status %s, got %s", expectedStatus, condition.Status)
	}

	return nil
}

// GetManifestWorkCount returns the count of ManifestWorks in a namespace
func GetManifestWorkCount(ctx context.Context, k8sClient client.Client, namespace string) (int, error) {
	// This would need to import the ManifestWork type and list them
	// Placeholder for now
	return 0, nil
}

// GetManagedClusterViewCount returns the count of ManagedClusterViews in a namespace
func GetManagedClusterViewCount(ctx context.Context, k8sClient client.Client, namespace string) (int, error) {
	// This would need to import the ManagedClusterView type and list them
	// Placeholder for now
	return 0, nil
}

// BuildVRG creates a VRG with the specified parameters
func BuildVRG(objectName, namespaceName, dstCluster string, action rmn.VRGAction) *rmn.VolumeReplicationGroup {
	vrg := NewVRGBuilder(objectName, namespaceName).
		WithReplicationState(rmn.Primary).
		WithAsync("1h").
		WithPVCSelector(metav1.LabelSelector{
			MatchLabels: map[string]string{"appclass": "gold"},
		}).
		Build()

	// Set action-specific fields
	if action == rmn.VRGActionFailover {
		vrg.Spec.ReplicationState = rmn.Secondary
	}

	return vrg
}

// Made with Bob

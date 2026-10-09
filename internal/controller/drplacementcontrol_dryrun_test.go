// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clrapiv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rmn "github.com/ramendr/ramen/api/v1alpha1"
)

var _ = Describe("DRPCDryRunTestFailover", func() {
	var (
		drpc               *rmn.DRPlacementControl
		drpcNamespacedName types.NamespacedName
		namespace          string
		drPolicy           *rmn.DRPolicy
		userPlacement      client.Object
	)

	BeforeEach(func() {
		// Initialize DRClusters with proper S3 profiles
		populateDRClusters()

		namespace = "test-drpc-dryrun-" + newRandomNamespaceSuffix()

		// Create namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(context.TODO(), ns)).To(Succeed())

		// Create namespaces for managed clusters (required by OCM for ManagedClusterAddOns)
		for _, cluster := range asyncClusters {
			clusterNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: cluster.Name,
				},
			}

			err := k8sClient.Create(context.TODO(), clusterNs)
			if err != nil && !k8serrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		}

		// Create managed clusters and DRClusters using existing helpers
		createManagedClusters(asyncClusters)
		createDRClustersAsync()

		// Create DRPolicy
		drPolicy = &rmn.DRPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-drpolicy-" + newRandomNamespaceSuffix(),
			},
			Spec: rmn.DRPolicySpec{
				DRClusters:         []string{East1ManagedCluster, West1ManagedCluster},
				SchedulingInterval: "1h",
			},
		}
		Expect(k8sClient.Create(context.TODO(), drPolicy)).To(Succeed())

		// Create user placement
		userPlacement = createUserPlacement(namespace)

		// Create DRPC
		drpc = &rmn.DRPlacementControl{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-drpc-dryrun",
				Namespace: namespace,
			},
			Spec: rmn.DRPlacementControlSpec{
				PlacementRef: corev1.ObjectReference{
					Name: userPlacement.GetName(),
					Kind: userPlacement.GetObjectKind().GroupVersionKind().Kind,
				},
				DRPolicyRef: corev1.ObjectReference{
					Name: drPolicy.Name,
				},
				PVCSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"appname": "testapp",
					},
				},
			},
		}
		drpcNamespacedName = types.NamespacedName{Name: drpc.Name, Namespace: drpc.Namespace}
	})

	AfterEach(func() {
		// Cleanup - errors are ignored as resources may not exist
		if drpc != nil {
			Expect(k8sClient.Delete(context.TODO(), drpc)).To(Or(Succeed(), MatchError(ContainSubstring("not found"))))
		}

		if userPlacement != nil {
			Expect(k8sClient.Delete(context.TODO(), userPlacement)).To(Or(Succeed(), MatchError(ContainSubstring("not found"))))
		}

		if drPolicy != nil {
			Expect(k8sClient.Delete(context.TODO(), drPolicy)).To(Or(Succeed(), MatchError(ContainSubstring("not found"))))
		}

		// Cleanup DRClusters
		deleteDRClustersAsync()

		if namespace != "" {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Delete(context.TODO(), ns)).To(Or(Succeed(), MatchError(ContainSubstring("not found"))))
		}
	})

	Describe("DryRun field behavior", func() {
		Context("When DryRun is not set (defaults to false)", func() {
			It("should have DryRun defaulting to false", func() {
				drpc.Spec.Action = rmn.ActionFailover
				drpc.Spec.PreferredCluster = West1ManagedCluster
				drpc.Spec.FailoverCluster = East1ManagedCluster
				Expect(k8sClient.Create(context.TODO(), drpc)).To(Succeed())

				// Verify DRPC was created
				Eventually(func() error {
					return apiReader.Get(context.TODO(), drpcNamespacedName, drpc)
				}, timeout, interval).Should(Succeed())

				// Verify DryRun defaults to false (boolean zero value)
				Expect(drpc.Spec.DryRun).To(BeFalse())
			})
		})

		Context("When DryRun is explicitly set to true", func() {
			It("should preserve DryRun=true", func() {
				drpc.Spec.Action = rmn.ActionFailover
				drpc.Spec.PreferredCluster = West1ManagedCluster
				drpc.Spec.FailoverCluster = East1ManagedCluster
				drpc.Spec.DryRun = true
				Expect(k8sClient.Create(context.TODO(), drpc)).To(Succeed())

				// Verify DRPC was created with DryRun=true
				Eventually(func() bool {
					if err := apiReader.Get(context.TODO(), drpcNamespacedName, drpc); err != nil {
						return false
					}

					return drpc.Spec.DryRun
				}, timeout, interval).Should(BeTrue())
			})
		})

		Context("When DryRun transitions from true to false", func() {
			It("should update DryRun field successfully", func() {
				// Create with DryRun=true
				drpc.Spec.Action = rmn.ActionFailover
				drpc.Spec.PreferredCluster = West1ManagedCluster
				drpc.Spec.FailoverCluster = East1ManagedCluster
				drpc.Spec.DryRun = true
				Expect(k8sClient.Create(context.TODO(), drpc)).To(Succeed())

				// Verify initial state
				Eventually(func() bool {
					if err := apiReader.Get(context.TODO(), drpcNamespacedName, drpc); err != nil {
						return false
					}

					return drpc.Spec.DryRun
				}, timeout, interval).Should(BeTrue())

				// Update to DryRun=false
				Eventually(func() error {
					if err := apiReader.Get(context.TODO(), drpcNamespacedName, drpc); err != nil {
						return err
					}

					drpc.Spec.DryRun = false

					return k8sClient.Update(context.TODO(), drpc)
				}, timeout, interval).Should(Succeed())

				// Verify DryRun is now false
				Eventually(func() bool {
					if err := apiReader.Get(context.TODO(), drpcNamespacedName, drpc); err != nil {
						return true // return opposite to fail the check
					}

					return drpc.Spec.DryRun
				}, timeout, interval).Should(BeFalse())
			})
		})
	})

	// spec.dryRun is a mode of its own: it no longer requires, consumes or
	// modifies spec.action. These cases pin that decoupling, which is the
	// user-visible change of the DRPC refactor.
	Describe("dryRun is independent of spec.action", func() {
		// Previously this combination was rejected outright with
		// "dryRun is enabled but action is not failover".
		It("accepts dryRun with a Relocate action", func() {
			drpc.Spec.Action = rmn.ActionRelocate
			drpc.Spec.PreferredCluster = West1ManagedCluster
			drpc.Spec.DryRun = true
			Expect(k8sClient.Create(context.TODO(), drpc)).To(Succeed())

			Consistently(func() string {
				return dryRunValidationMessage(drpcNamespacedName)
			}, "2s", interval).Should(BeEmpty())
		})

		It("accepts dryRun with no action at all", func() {
			drpc.Spec.PreferredCluster = West1ManagedCluster
			drpc.Spec.DryRun = true
			Expect(k8sClient.Create(context.TODO(), drpc)).To(Succeed())

			Consistently(func() string {
				return dryRunValidationMessage(drpcNamespacedName)
			}, "2s", interval).Should(BeEmpty())
		})

		It("leaves spec.action untouched while dryRun is set", func() {
			drpc.Spec.Action = rmn.ActionRelocate
			drpc.Spec.PreferredCluster = West1ManagedCluster
			drpc.Spec.DryRun = true
			Expect(k8sClient.Create(context.TODO(), drpc)).To(Succeed())

			Consistently(func() rmn.DRAction {
				latest := &rmn.DRPlacementControl{}
				if err := apiReader.Get(context.TODO(), drpcNamespacedName, latest); err != nil {
					return ""
				}

				return latest.Spec.Action
			}, "2s", interval).Should(Equal(rmn.ActionRelocate))
		})
	})

	Describe("a real action is unaffected by the dry-run path", func() {
		It("accepts a normal failover with dryRun unset", func() {
			drpc.Spec.Action = rmn.ActionFailover
			drpc.Spec.PreferredCluster = West1ManagedCluster
			drpc.Spec.FailoverCluster = East1ManagedCluster
			drpc.Spec.DryRun = false
			Expect(k8sClient.Create(context.TODO(), drpc)).To(Succeed())

			Eventually(func() error {
				return apiReader.Get(context.TODO(), drpcNamespacedName, drpc)
			}, timeout, interval).Should(Succeed())

			Expect(drpc.Spec.DryRun).To(BeFalse())
			Expect(drpc.Spec.Action).To(Equal(rmn.ActionFailover))
		})
	})
})

// dryRunValidationMessage returns any condition message complaining that
// dryRun requires a failover action. The coupling it describes was removed,
// so this must stay empty.
func dryRunValidationMessage(key types.NamespacedName) string {
	latest := &rmn.DRPlacementControl{}
	if err := apiReader.Get(context.TODO(), key, latest); err != nil {
		return ""
	}

	for i := range latest.Status.Conditions {
		if strings.Contains(latest.Status.Conditions[i].Message, "dryRun is enabled but action is not failover") {
			return latest.Status.Conditions[i].Message
		}
	}

	return ""
}

func createUserPlacement(namespace string) client.Object {
	var numberOfClusters int32 = 1

	placement := &clrapiv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-placement-" + newRandomNamespaceSuffix(),
			Namespace: namespace,
			Annotations: map[string]string{
				clrapiv1beta1.PlacementDisableAnnotation: "true",
			},
		},
		Spec: clrapiv1beta1.PlacementSpec{
			NumberOfClusters: &numberOfClusters,
			ClusterSets:      []string{East1ManagedCluster, West1ManagedCluster},
		},
	}
	Expect(k8sClient.Create(context.TODO(), placement)).To(Succeed())

	// Create PlacementDecision for the Placement
	createPlacementDecisionForPlacement(placement)

	return placement
}

func createPlacementDecisionForPlacement(placement *clrapiv1beta1.Placement) {
	placementDecision := &clrapiv1beta1.PlacementDecision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      placement.Name + "-decision-1",
			Namespace: placement.Namespace,
			Labels: map[string]string{
				"cluster.open-cluster-management.io/decision-group-index": "0",
				"cluster.open-cluster-management.io/decision-group-name":  "",
				"cluster.open-cluster-management.io/placement":            placement.Name,
			},
		},
		Status: clrapiv1beta1.PlacementDecisionStatus{
			Decisions: []clrapiv1beta1.ClusterDecision{
				{
					ClusterName: East1ManagedCluster,
					Reason:      "selected",
				},
			},
		},
	}
	Expect(k8sClient.Create(context.TODO(), placementDecision)).To(Succeed())
}

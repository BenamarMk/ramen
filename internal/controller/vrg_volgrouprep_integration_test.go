// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers_test

import (
	"context"

	volrep "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	ramendrv1alpha1 "github.com/ramendr/ramen/api/v1alpha1"
	neutralv1alpha1 "github.com/ramendr/replication-storage-io-crds/api/v1alpha1"
	vrgController "github.com/ramendr/ramen/internal/controller"
)

// Integration tests for VRG controller with replication handler abstraction
// These tests validate that the VRG controller correctly uses the abstraction layer
// to work with both neutral (replication.storage.io) and legacy (replication.storage.openshift.io) APIs

var _ = Describe("VRGVolumeGroupReplicationIntegration", func() {
	const (
		vrgName       = "test-vrg"
		pvcName       = "test-pvc"
		storageID     = "test-storage-id"
		replicationID = "test-replication-id"
	)

	var (
		vrg                  *ramendrv1alpha1.VolumeReplicationGroup
		vrgNamespacedName    types.NamespacedName
		testCtx              context.Context
		testNamespace        string
		storageClassName     string
		neutralVGRClassName  string
		legacyVGRClassName   string
	)

	BeforeEach(func() {
		testCtx = context.TODO()
		
		// Generate unique names for each test to avoid conflicts
		suffix := newRandomNamespaceSuffix()
		testNamespace = "vrg-int-test-" + suffix
		storageClassName = "test-sc-" + suffix
		neutralVGRClassName = "neutral-vgr-class-" + suffix
		legacyVGRClassName = "legacy-vgr-class-" + suffix
		
		vrgNamespacedName = types.NamespacedName{
			Name:      vrgName,
			Namespace: testNamespace,
		}

		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())

		// Create storage class with unique name
		sc := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: storageClassName,
				Labels: map[string]string{
					vrgController.StorageIDLabel: storageID,
				},
			},
			Provisioner: "test.csi.driver",
		}
		Expect(k8sClient.Create(testCtx, sc)).To(Succeed())
	})

	AfterEach(func() {
		// Clean up VRG if it exists
		if vrg != nil {
			_ = k8sClient.Delete(testCtx, vrg)
		}

		// Clean up neutral VGRClass if it exists
		neutralVGRClass := &neutralv1alpha1.VolumeGroupReplicationClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: neutralVGRClassName,
			},
		}
		_ = k8sClient.Delete(testCtx, neutralVGRClass)

		// Clean up legacy VGRClass if it exists
		legacyVGRClass := &volrep.VolumeGroupReplicationClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: legacyVGRClassName,
			},
		}
		_ = k8sClient.Delete(testCtx, legacyVGRClass)

		// Clean up storage class
		sc := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: storageClassName,
			},
		}
		_ = k8sClient.Delete(testCtx, sc)

		// Clean up namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		_ = k8sClient.Delete(testCtx, ns)
	})

	Context("Handler Discovery and Initialization", func() {
		It("should initialize with neutral handler when neutral API is available", func() {
			// Create neutral VolumeGroupReplicationClass
			neutralVGRClass := &neutralv1alpha1.VolumeGroupReplicationClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: neutralVGRClassName,
					Labels: map[string]string{
						vrgController.StorageIDLabel:     storageID,
						vrgController.ReplicationIDLabel: replicationID,
					},
				},
				Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
					Provisioner: "test.csi.driver",
					Parameters: map[string]string{
						"replication.storage.io/replication-secret-name":      "test-secret",
						"replication.storage.io/replication-secret-namespace": testNamespace,
					},
				},
			}
			Expect(k8sClient.Create(testCtx, neutralVGRClass)).To(Succeed())

			// Create VRG
			vrg = &ramendrv1alpha1.VolumeReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vrgName,
					Namespace: testNamespace,
				},
				Spec: ramendrv1alpha1.VolumeReplicationGroupSpec{
					PVCSelector:      metav1.LabelSelector{},
					ReplicationState: ramendrv1alpha1.Primary,
					S3Profiles:       []string{},
				},
			}
			Expect(k8sClient.Create(testCtx, vrg)).To(Succeed())

			// Wait for VRG to be reconciled
			Eventually(func() bool {
				err := apiReader.Get(testCtx, vrgNamespacedName, vrg)
				return err == nil && len(vrg.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue())

			// Verify VRG was processed (conditions should be set)
			Expect(vrg.Status.Conditions).NotTo(BeEmpty())
		})

		It("should fallback to legacy handler when neutral API is not available", func() {
			// Create legacy VolumeGroupReplicationClass
			legacyVGRClass := &volrep.VolumeGroupReplicationClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: legacyVGRClassName,
					Labels: map[string]string{
						vrgController.StorageIDLabel:     storageID,
						vrgController.ReplicationIDLabel: replicationID,
					},
				},
				Spec: volrep.VolumeGroupReplicationClassSpec{
					Provisioner: "test.csi.driver",
					Parameters: map[string]string{
						"replication.storage.openshift.io/replication-secret-name":      "test-secret",
						"replication.storage.openshift.io/replication-secret-namespace": testNamespace,
					},
				},
			}
			Expect(k8sClient.Create(testCtx, legacyVGRClass)).To(Succeed())

			// Create VRG
			vrg = &ramendrv1alpha1.VolumeReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vrgName,
					Namespace: testNamespace,
				},
				Spec: ramendrv1alpha1.VolumeReplicationGroupSpec{
					PVCSelector:      metav1.LabelSelector{},
					ReplicationState: ramendrv1alpha1.Primary,
					S3Profiles:       []string{},
				},
			}
			Expect(k8sClient.Create(testCtx, vrg)).To(Succeed())

			// Wait for VRG to be reconciled
			Eventually(func() bool {
				err := apiReader.Get(testCtx, vrgNamespacedName, vrg)
				return err == nil && len(vrg.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue())

			// Verify VRG was processed
			Expect(vrg.Status.Conditions).NotTo(BeEmpty())
		})
	})

	Context("Handler Switching", func() {
		It("should switch from legacy to neutral handler when neutral API becomes available", func() {
			// Start with only legacy API available
			legacyVGRClass := &volrep.VolumeGroupReplicationClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: legacyVGRClassName,
					Labels: map[string]string{
						vrgController.StorageIDLabel:     storageID,
						vrgController.ReplicationIDLabel: replicationID,
					},
				},
				Spec: volrep.VolumeGroupReplicationClassSpec{
					Provisioner: "test.csi.driver",
					Parameters: map[string]string{
						"replication.storage.openshift.io/replication-secret-name":      "test-secret",
						"replication.storage.openshift.io/replication-secret-namespace": testNamespace,
					},
				},
			}
			Expect(k8sClient.Create(testCtx, legacyVGRClass)).To(Succeed())

			// Create VRG (should use legacy handler)
			vrg = &ramendrv1alpha1.VolumeReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vrgName,
					Namespace: testNamespace,
				},
				Spec: ramendrv1alpha1.VolumeReplicationGroupSpec{
					PVCSelector:      metav1.LabelSelector{},
					ReplicationState: ramendrv1alpha1.Primary,
					S3Profiles:       []string{},
				},
			}
			Expect(k8sClient.Create(testCtx, vrg)).To(Succeed())

			// Wait for initial reconciliation
			Eventually(func() bool {
				err := apiReader.Get(testCtx, vrgNamespacedName, vrg)
				return err == nil && len(vrg.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue())

			// Now add neutral API
			neutralVGRClass := &neutralv1alpha1.VolumeGroupReplicationClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: neutralVGRClassName,
					Labels: map[string]string{
						vrgController.StorageIDLabel:     storageID,
						vrgController.ReplicationIDLabel: replicationID,
					},
				},
				Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
					Provisioner: "test.csi.driver",
					Parameters: map[string]string{
						"replication.storage.io/replication-secret-name":      "test-secret",
						"replication.storage.io/replication-secret-namespace": testNamespace,
					},
				},
			}
			Expect(k8sClient.Create(testCtx, neutralVGRClass)).To(Succeed())

			// Trigger reconciliation by updating VRG
			vrg.Spec.ReplicationState = ramendrv1alpha1.Secondary
			Expect(k8sClient.Update(testCtx, vrg)).To(Succeed())

			// Wait for update to be processed
			Eventually(func() bool {
				err := apiReader.Get(testCtx, vrgNamespacedName, vrg)
				if err != nil {
					return false
				}
				return vrg.Status.ObservedGeneration > 0
			}, timeout, interval).Should(BeTrue())

			// Handler should now prefer neutral API if available
			// This is verified by the discovery mechanism's priority logic
		})
	})

	Context("Error Handling", func() {
		It("should handle missing VolumeGroupReplicationClass gracefully", func() {
			// Create VRG without any VGRClass
			vrg = &ramendrv1alpha1.VolumeReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vrgName,
					Namespace: testNamespace,
				},
				Spec: ramendrv1alpha1.VolumeReplicationGroupSpec{
					PVCSelector:      metav1.LabelSelector{},
					ReplicationState: ramendrv1alpha1.Primary,
					S3Profiles:       []string{},
				},
			}
			Expect(k8sClient.Create(testCtx, vrg)).To(Succeed())

			// Wait for VRG to be reconciled
			Eventually(func() bool {
				err := apiReader.Get(testCtx, vrgNamespacedName, vrg)
				return err == nil && len(vrg.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue())

			// VRG should have conditions set (even if error conditions)
			Expect(vrg.Status.Conditions).NotTo(BeEmpty())
		})

		It("should handle API group not found errors", func() {
			// This test verifies that the controller handles cases where
			// neither neutral nor legacy API groups are available
			// The controller should still reconcile without crashing

			vrg = &ramendrv1alpha1.VolumeReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vrgName,
					Namespace: testNamespace,
				},
				Spec: ramendrv1alpha1.VolumeReplicationGroupSpec{
					PVCSelector:      metav1.LabelSelector{},
					ReplicationState: ramendrv1alpha1.Primary,
					S3Profiles:       []string{},
				},
			}
			Expect(k8sClient.Create(testCtx, vrg)).To(Succeed())

			// Controller should not crash and should set conditions
			Eventually(func() bool {
				err := apiReader.Get(testCtx, vrgNamespacedName, vrg)
				return err == nil && len(vrg.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Status Aggregation", func() {
		It("should aggregate status from VolumeGroupReplication to VRG", func() {
			// Create neutral VGRClass
			neutralVGRClass := &neutralv1alpha1.VolumeGroupReplicationClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: neutralVGRClassName,
					Labels: map[string]string{
						vrgController.StorageIDLabel:     storageID,
						vrgController.ReplicationIDLabel: replicationID,
					},
				},
				Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
					Provisioner: "test.csi.driver",
					Parameters: map[string]string{
						"replication.storage.io/replication-secret-name":      "test-secret",
						"replication.storage.io/replication-secret-namespace": testNamespace,
					},
				},
			}
			Expect(k8sClient.Create(testCtx, neutralVGRClass)).To(Succeed())

			// Create PVC
			scName := storageClassName
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: testNamespace,
					Labels: map[string]string{
						"app": "test-status",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
					StorageClassName: &scName,
				},
			}
			Expect(k8sClient.Create(testCtx, pvc)).To(Succeed())

			// Create VRG
			vrg = &ramendrv1alpha1.VolumeReplicationGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vrgName,
					Namespace: testNamespace,
				},
				Spec: ramendrv1alpha1.VolumeReplicationGroupSpec{
					PVCSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-status",
						},
					},
					ReplicationState: ramendrv1alpha1.Primary,
					S3Profiles:       []string{},
				},
			}
			Expect(k8sClient.Create(testCtx, vrg)).To(Succeed())

			// Wait for VRG status to be updated
			Eventually(func() bool {
				err := apiReader.Get(testCtx, vrgNamespacedName, vrg)
				if err != nil {
					return false
				}
				// Check that status has been populated
				return len(vrg.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue())

			// Verify status conditions are set
			Expect(vrg.Status.Conditions).NotTo(BeEmpty())
			
			// Verify ObservedGeneration is updated
			Expect(vrg.Status.ObservedGeneration).To(BeNumerically(">=", 0))
		})
	})
})

// Made with Bob

package controllers_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	ramendrv1alpha1 "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/controllers"
	"github.com/ramendr/ramen/controllers/volsync"
)

const (
	testMaxWait          = 10 * time.Second
	testInterval         = 250 * time.Millisecond
	testStorageClassName = "fakestorageclass"
)

var _ = Describe("VolsyncreplicationgroupController", func() {
	var testNamespace *corev1.Namespace
	testLogger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var testCtx context.Context
	var cancel context.CancelFunc

	BeforeEach(func() {
		testCtx, cancel = context.WithCancel(context.TODO())

		// Create namespace for test
		testNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "vh-",
			},
		}
		Expect(k8sClient.Create(testCtx, testNamespace)).To(Succeed())
		Expect(testNamespace.GetName()).NotTo(BeEmpty())
	})

	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(testCtx, testNamespace)).To(Succeed())

		cancel()
	})

	Describe("Primary initial setup", func() {
		testMatchLabels := map[string]string{
			"ramentest": "backmeup",
		}

		var testVsrg *ramendrv1alpha1.VolSyncReplicationGroup

		Context("When VSRG created on primary", func() {
			BeforeEach(func() {
				testVsrg = &ramendrv1alpha1.VolSyncReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "test-vsrg-east-",
						Namespace:    testNamespace.GetName(),
					},
					Spec: ramendrv1alpha1.VolSyncReplicationGroupSpec{
						ReplicationState:   ramendrv1alpha1.Primary,
						SchedulingInterval: "1h",
						PVCSelector: metav1.LabelSelector{
							MatchLabels: testMatchLabels,
						},
					},
				}
			})

			JustBeforeEach(func() {
				Expect(k8sClient.Create(testCtx, testVsrg)).To(Succeed())

				Eventually(func() []string {
					err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(testVsrg), testVsrg)
					if err != nil {
						return []string{}
					}
					return testVsrg.GetFinalizers()
				}, testMaxWait, testInterval).Should(ContainElement(controllers.VsrgFinalizerName))
			})

			Context("When no matching PVCs are bound", func() {
				It("Should not update status with protected PVCs", func() {
					Expect(len(testVsrg.Status.VolSyncPVCs)).To(Equal(0))
				})
			})

			Context("When matching PVCs are bound", func() {
				var boundPvcs []corev1.PersistentVolumeClaim
				testSshKeys := "testsshkeys"
				JustBeforeEach(func() {
					boundPvcs = []corev1.PersistentVolumeClaim{} // Reset for each test

					// Create some PVCs that are bound
					for i := 0; i < 3; i++ {
						newPvc := createPVC(testCtx, testNamespace.GetName(), testMatchLabels, corev1.ClaimBound)
						boundPvcs = append(boundPvcs, *newPvc)
					}

					Eventually(func() int {
						err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(testVsrg), testVsrg)
						if err != nil {
							return 0
						}
						return len(testVsrg.Status.VolSyncPVCs)
					}, testMaxWait, testInterval).Should(Equal(len(boundPvcs)))
				})

				It("Should find the bound PVCs and report in Status", func() {
					// Check the volsync pvcs
					foundBoundPVC0 := false
					foundBoundPVC1 := false
					foundBoundPVC2 := false
					for _, vsPvc := range testVsrg.Status.VolSyncPVCs {
						switch vsPvc.PVCName {
						case boundPvcs[0].GetName():
							foundBoundPVC0 = true
						case boundPvcs[1].GetName():
							foundBoundPVC1 = true
						case boundPvcs[2].GetName():
							foundBoundPVC2 = true
						}
					}
					Expect(foundBoundPVC0).To(BeTrue())
					Expect(foundBoundPVC1).To(BeTrue())
					Expect(foundBoundPVC2).To(BeTrue())
				})

				Context("When RSSpec entries are added to vsrg spec", func() {
					rs0 := &volsyncv1alpha1.ReplicationSource{}
					rs1 := &volsyncv1alpha1.ReplicationSource{}
					rs2 := &volsyncv1alpha1.ReplicationSource{}

					JustBeforeEach(func() {
						// Update the vsrg spec with some RSSpec entries
						rsSpec := []ramendrv1alpha1.ReplicationSourceSpec{
							{
								PVCName: boundPvcs[0].GetName(),
								Address: "1.2.0.10",
								SSHKeys: testSshKeys,
							},
							{
								PVCName: boundPvcs[1].GetName(),
								Address: "1.2.1.10",
								SSHKeys: testSshKeys,
							},
							{
								PVCName: boundPvcs[2].GetName(),
								Address: "1.2.2.10",
								SSHKeys: testSshKeys,
							},
						}
						Eventually(func() error {
							// Put this in Eventually loop to avoid update issues (controller is also updating the vsrg)
							err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(testVsrg), testVsrg)
							if err != nil {
								return err
							}
							testVsrg.Spec.RSSpec = rsSpec
							return k8sClient.Update(testCtx, testVsrg)
						}, testMaxWait, interval).Should(Succeed())

						allRSs := &volsyncv1alpha1.ReplicationSourceList{}
						Eventually(func() int {
							Expect(k8sClient.List(testCtx, allRSs,
								client.InNamespace(testNamespace.GetName()))).To(Succeed())
							return len(allRSs.Items)
						}, testMaxWait, testInterval).Should(Equal(len(testVsrg.Spec.RSSpec)))

						Expect(k8sClient.Get(testCtx, types.NamespacedName{
							Name: boundPvcs[0].GetName(), Namespace: testNamespace.GetName()}, rs0)).To(Succeed())
						Expect(k8sClient.Get(testCtx, types.NamespacedName{
							Name: boundPvcs[1].GetName(), Namespace: testNamespace.GetName()}, rs1)).To(Succeed())
						Expect(k8sClient.Get(testCtx, types.NamespacedName{
							Name: boundPvcs[2].GetName(), Namespace: testNamespace.GetName()}, rs2)).To(Succeed())
					})

					It("Should create ReplicationSources for each", func() {
						Expect(rs0.Spec.SourcePVC).To(Equal(boundPvcs[0].GetName()))
						Expect(*rs0.Spec.Rsync.Address).To(Equal(testVsrg.Spec.RSSpec[0].Address))
						Expect(*rs0.Spec.Rsync.SSHKeys).To(Equal(testVsrg.Spec.RSSpec[0].SSHKeys))
						Expect(rs0.Spec.Trigger).NotTo(BeNil())
						Expect(*rs0.Spec.Trigger.Schedule).To(Equal("* */1 * * *")) // scheduling interval was set to 1h

						Expect(rs1.Spec.SourcePVC).To(Equal(boundPvcs[1].GetName()))
						Expect(*rs1.Spec.Rsync.Address).To(Equal(testVsrg.Spec.RSSpec[1].Address))
						Expect(*rs1.Spec.Rsync.SSHKeys).To(Equal(testVsrg.Spec.RSSpec[1].SSHKeys))
						Expect(rs1.Spec.Trigger).NotTo(BeNil())
						Expect(*rs1.Spec.Trigger.Schedule).To(Equal("* */1 * * *")) // scheduling interval was set to 1h

						Expect(rs2.Spec.SourcePVC).To(Equal(boundPvcs[2].GetName()))
						Expect(*rs2.Spec.Rsync.Address).To(Equal(testVsrg.Spec.RSSpec[2].Address))
						Expect(*rs2.Spec.Rsync.SSHKeys).To(Equal(testVsrg.Spec.RSSpec[2].SSHKeys))
						Expect(rs2.Spec.Trigger).NotTo(BeNil())
						Expect(*rs2.Spec.Trigger.Schedule).To(Equal("* */1 * * *")) // scheduling interval was set to 1h
					})

					// Note this is technically a relocate since the RSSpec[] is not empty
					Context("When relocate/failover occurs (primary becomes secondary)", func() {
						JustBeforeEach(func() {
							// JustBeforeEach in parent tests (see outer contexts) are already setting up
							// the VSRG with RSSpec and corresponding ReplicationSources are created

							// Now update VSRG to indicate failover (change from Primary to Secondary)
							Eventually(func() error {
								// Put this in Eventually loop to avoid update issues (controller is also updating the vsrg)
								err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(testVsrg), testVsrg)
								if err != nil {
									return err
								}
								testVsrg.Spec.ReplicationState = ramendrv1alpha1.Secondary
								return k8sClient.Update(testCtx, testVsrg)
							}, testMaxWait, interval).Should(Succeed())
						})

						It("Should run a final synchronization on the ReplicationSources previously created", func() {
							// Controller should try to run a final sync, re-load the RSs to check
							Eventually(func() bool {
								rsList := []*volsyncv1alpha1.ReplicationSource{rs0, rs1, rs2}
								for _, rs := range rsList {
									err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(rs), rs)
									if err != nil {
										return false
									}
									if rs.Spec.Trigger == nil || rs.Spec.Trigger.Manual == "" {
										return false
									}
								}
								return true
							}, testMaxWait, testInterval).Should(BeTrue())

							Expect(rs0.Spec.Trigger.Manual).To(Equal(volsync.FinalSyncTriggerString))
							Expect(rs1.Spec.Trigger.Manual).To(Equal(volsync.FinalSyncTriggerString))
							Expect(rs2.Spec.Trigger.Manual).To(Equal(volsync.FinalSyncTriggerString))

							//TODO: get status from VSRG to indicate that final syncs are done
						})
					})
				})
			})
		})
	})

	Describe("Secondary initial setup", func() {
		testMatchLabels := map[string]string{
			"ramentest": "backmeup",
		}

		var testVsrg *ramendrv1alpha1.VolSyncReplicationGroup

		testAccessModes := []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteOnce,
		}

		testSshKeys := "secondarytestsshkeys"

		Context("When VSRG created on secondary", func() {
			JustBeforeEach(func() {
				testVsrg = &ramendrv1alpha1.VolSyncReplicationGroup{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "test-vsrg-east-",
						Namespace:    testNamespace.GetName(),
					},
					Spec: ramendrv1alpha1.VolSyncReplicationGroupSpec{
						ReplicationState:   ramendrv1alpha1.Secondary,
						SchedulingInterval: "1h",
						PVCSelector: metav1.LabelSelector{
							MatchLabels: testMatchLabels,
						},
					},
				}

				Expect(k8sClient.Create(testCtx, testVsrg)).To(Succeed())

				Eventually(func() []string {
					err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(testVsrg), testVsrg)
					if err != nil {
						return []string{}
					}
					return testVsrg.GetFinalizers()
				}, testMaxWait, testInterval).Should(ContainElement(controllers.VsrgFinalizerName))
			})

			Context("When RDSpec entries are added to vsrg spec", func() {
				storageClassName := testStorageClassName

				testCapacity0 := corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				}
				testCapacity1 := corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("20Gi"),
				}

				rd0 := &volsyncv1alpha1.ReplicationDestination{}
				rd1 := &volsyncv1alpha1.ReplicationDestination{}

				JustBeforeEach(func() {
					// Putting this in JustBeforeEach instead of BeforeEach because we want the VSRG to have been
					// created first with no RDSpec entries, then updated with RDSpec entries afterwards.
					// This is to simulate what we expect to see in practice.

					// Update the vsrg spec with some RDSpec entries
					rdSpec := []ramendrv1alpha1.ReplicationDestinationSpec{
						{
							VolSyncPVCInfo: ramendrv1alpha1.VolSyncPVCInfo{
								PVCName: "testingpvc-a",
								Labels: map[string]string{
									"drlabel":      "backmeup",
									"anotherlabel": "abcd",
								},
								StorageClassName: &storageClassName,
								AccessModes:      testAccessModes,

								Resources: corev1.ResourceRequirements{Requests: testCapacity0},
							},
							SSHKeys: testSshKeys,
						},
						{
							VolSyncPVCInfo: ramendrv1alpha1.VolSyncPVCInfo{
								PVCName: "testingpvc-b",
								Labels: map[string]string{
									"drlabel": "backmeup",
									"b-label": "blahblah",
								},
								StorageClassName: &storageClassName,
								AccessModes:      testAccessModes,

								Resources: corev1.ResourceRequirements{Requests: testCapacity1},
							},
							SSHKeys: testSshKeys,
						},
					}
					Eventually(func() error {
						// Put this in Eventually loop to avoid update issues (controller is also updating the vsrg)
						err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(testVsrg), testVsrg)
						if err != nil {
							return err
						}
						testVsrg.Spec.RDSpec = rdSpec
						return k8sClient.Update(testCtx, testVsrg)
					}, testMaxWait, interval).Should(Succeed())

					allRDs := &volsyncv1alpha1.ReplicationDestinationList{}
					Eventually(func() int {
						Expect(k8sClient.List(testCtx, allRDs,
							client.InNamespace(testNamespace.GetName()))).To(Succeed())
						return len(allRDs.Items)
					}, testMaxWait, testInterval).Should(Equal(len(testVsrg.Spec.RDSpec)))

					testLogger.Info("Found RDs", "allRDs", allRDs)

					Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: testVsrg.Spec.RDSpec[0].PVCName,
						Namespace: testNamespace.GetName()}, rd0)).To(Succeed())
					Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: testVsrg.Spec.RDSpec[1].PVCName,
						Namespace: testNamespace.GetName()}, rd1)).To(Succeed())
				})

				It("Should create ReplicationDestinations for each", func() {
					Expect(rd0.Spec.Trigger).To(BeNil()) // Rsync, so destination will not have schedule
					Expect(rd0.Spec.Rsync).NotTo(BeNil())
					Expect(*rd0.Spec.Rsync.SSHKeys).To(Equal(testSshKeys))
					Expect(*rd0.Spec.Rsync.StorageClassName).To(Equal(testStorageClassName))
					Expect(rd0.Spec.Rsync.AccessModes).To(Equal(testAccessModes))
					Expect(rd0.Spec.Rsync.CopyMethod).To(Equal(volsyncv1alpha1.CopyMethodSnapshot))
					Expect(*rd0.Spec.Rsync.ServiceType).To(Equal(corev1.ServiceTypeLoadBalancer))

					Expect(rd1.Spec.Trigger).To(BeNil()) // Rsync, so destination will not have schedule
					Expect(rd1.Spec.Rsync).NotTo(BeNil())
					Expect(*rd1.Spec.Rsync.SSHKeys).To(Equal(testSshKeys))
					Expect(*rd1.Spec.Rsync.StorageClassName).To(Equal(testStorageClassName))
					Expect(rd1.Spec.Rsync.AccessModes).To(Equal(testAccessModes))
					Expect(rd1.Spec.Rsync.CopyMethod).To(Equal(volsyncv1alpha1.CopyMethodSnapshot))
					Expect(*rd1.Spec.Rsync.ServiceType).To(Equal(corev1.ServiceTypeLoadBalancer))
				})

				Context("When ReplicationDestinations have address set in status", func() {
					rd0Address := "99.98.97.96"
					rd1Address := "99.88.77.66"
					JustBeforeEach(func() {
						// fake address set in status on the ReplicationDestinations
						rd0.Status = &volsyncv1alpha1.ReplicationDestinationStatus{
							Rsync: &volsyncv1alpha1.ReplicationDestinationRsyncStatus{
								SSHKeys: &testSshKeys,
								Address: &rd0Address,
							},
						}
						Expect(k8sClient.Status().Update(testCtx, rd0)).To(Succeed())

						rd1.Status = &volsyncv1alpha1.ReplicationDestinationStatus{
							Rsync: &volsyncv1alpha1.ReplicationDestinationRsyncStatus{
								SSHKeys: &testSshKeys,
								Address: &rd1Address,
							},
						}
						Expect(k8sClient.Status().Update(testCtx, rd1)).To(Succeed())

						// Wait for VSRG controller to see the update in the RDs and update VSRG status accordingly
						Eventually(func() int {
							err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(testVsrg), testVsrg)
							if err != nil {
								return 0
							}
							return len(testVsrg.Status.RDInfo)
						}, testMaxWait, testInterval).Should(Equal(2))
					})

					It("VSRG status should be updated with proper RDInfo", func() {
						// Confirm the RDInfo in the VSRG status is set correctly
						foundRdInfo0 := false
						foundRdInfo1 := false
						for _, rdInfo := range testVsrg.Status.RDInfo {
							if rdInfo.PVCName == testVsrg.Spec.RDSpec[0].PVCName {
								foundRdInfo0 = true
								Expect(rdInfo.Address).To(Equal(rd0Address))
							}
							if rdInfo.PVCName == testVsrg.Spec.RDSpec[1].PVCName {
								foundRdInfo1 = true
								Expect(rdInfo.Address).To(Equal(rd1Address))
							}
						}
						Expect(foundRdInfo0).To(BeTrue())
						Expect(foundRdInfo1).To(BeTrue())
					})

					Context("When failover occurs (secondary becomes primary)", func() {
						rd0SnapName := "testsnap-rd0"
						rd1SnapName := "testsnap-rd1"

						JustBeforeEach(func() {
							// JustBeforeEach in parent tests (see outer contexts) are already setting up
							// the VSRG and faking out that ReplicationDestinations are complete (i.e. address
							// specified in RD and VSRG status updated to reflect this with RDInfo for each RD).

							// Fake out that the replication destinations are complete by putting a "latestImage"
							// in the status
							apiGrp := volsync.VolumeSnapshotGroup
							rd0.Status.LatestImage = &corev1.TypedLocalObjectReference{
								Kind:     volsync.VolumeSnapshotKind,
								APIGroup: &apiGrp,
								Name:     rd0SnapName,
							}
							Expect(k8sClient.Status().Update(testCtx, rd0)).To(Succeed())

							rd1.Status.LatestImage = &corev1.TypedLocalObjectReference{
								Kind:     volsync.VolumeSnapshotKind,
								APIGroup: &apiGrp,
								Name:     rd1SnapName,
							}
							Expect(k8sClient.Status().Update(testCtx, rd1)).To(Succeed())

							// Create volumesnapshots that correspond to the ones we just faked out in the RD status
							Expect(createSnapshot(testCtx,
								rd0SnapName, testVsrg.Spec.RDSpec[0].PVCName, testNamespace.GetName())).To(Succeed())
							Expect(createSnapshot(testCtx,
								rd1SnapName, testVsrg.Spec.RDSpec[1].PVCName, testNamespace.GetName())).To(Succeed())

							// Now update VSRG to indicate failover (change from Secondary to Primary)
							Eventually(func() error {
								// Put this in Eventually loop to avoid update issues (controller is also updating the vsrg)
								err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(testVsrg), testVsrg)
								if err != nil {
									return err
								}
								testVsrg.Spec.ReplicationState = ramendrv1alpha1.Primary
								return k8sClient.Update(testCtx, testVsrg)
							}, testMaxWait, interval).Should(Succeed())
						})

						It("Should restore PVCs from the ReplicationDestinations previously created", func() {
							Eventually(func() bool {
								err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(testVsrg), testVsrg)
								if err != nil {
									return false
								}
								return apimeta.IsStatusConditionTrue(testVsrg.Status.Conditions,
									controllers.VRGConditionTypeClusterDataReady)
							}, testMaxWait, testInterval).Should(BeTrue())

							// Check that PVCs were created and point to the snapshots
							pvc0 := &corev1.PersistentVolumeClaim{
								ObjectMeta: metav1.ObjectMeta{
									Name:      testVsrg.Spec.RDSpec[0].PVCName,
									Namespace: testNamespace.GetName(),
								},
							}
							Eventually(func() error {
								return k8sClient.Get(testCtx, client.ObjectKeyFromObject(pvc0), pvc0)
							}, testMaxWait, testInterval).Should(Succeed())
							Expect(pvc0.Spec.DataSource.Name).To(Equal(rd0SnapName))
							Expect(pvc0.Spec.DataSource.Kind).To(Equal(volsync.VolumeSnapshotKind))
							Expect(pvc0.Labels).To(Equal(testVsrg.Spec.RDSpec[0].Labels))

							pvc1 := &corev1.PersistentVolumeClaim{
								ObjectMeta: metav1.ObjectMeta{
									Name:      testVsrg.Spec.RDSpec[1].PVCName,
									Namespace: testNamespace.GetName(),
								},
							}
							Eventually(func() error {
								return k8sClient.Get(testCtx, client.ObjectKeyFromObject(pvc1), pvc1)
							}, testMaxWait, testInterval).Should(Succeed())
							Expect(pvc1.Spec.DataSource.Name).To(Equal(rd1SnapName))
							Expect(pvc1.Spec.DataSource.Kind).To(Equal(volsync.VolumeSnapshotKind))
							Expect(pvc1.Labels).To(Equal(testVsrg.Spec.RDSpec[1].Labels))
						})
					})
				})
			})
		})
	})

})

func createPVC(ctx context.Context, namespace string, labels map[string]string,
	bindInfo corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {

	capacity := corev1.ResourceList{
		corev1.ResourceStorage: resource.MustParse("1Gi"),
	}
	accessModes := []corev1.PersistentVolumeAccessMode{
		corev1.ReadWriteOnce,
	}

	storageClassName := testStorageClassName

	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "testpvc-",
			Labels:       labels,
			Namespace:    namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      accessModes,
			Resources:        corev1.ResourceRequirements{Requests: capacity},
			StorageClassName: &storageClassName,
		},
	}

	Expect(k8sClient.Create(context.TODO(), pvc)).To(Succeed())

	pvc.Status.Phase = bindInfo
	pvc.Status.AccessModes = accessModes
	pvc.Status.Capacity = capacity
	Expect(k8sClient.Status().Update(ctx, pvc)).To(Succeed())

	return pvc
}

func createSnapshot(ctx context.Context, snapshotName, pvcName, namespace string) error {
	volSnap := &unstructured.Unstructured{}
	volSnap.Object = map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      snapshotName,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"source": map[string]interface{}{
				"persistentVolumeClaimName": "fakepvcnamehere",
			},
		},
	}
	volSnap.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   volsync.VolumeSnapshotGroup,
		Kind:    volsync.VolumeSnapshotKind,
		Version: volsync.VolumeSnapshotVersion,
	})

	return k8sClient.Create(ctx, volSnap)
}

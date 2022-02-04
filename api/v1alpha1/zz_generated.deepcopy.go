// +build !ignore_autogenerated

/*
Copyright 2021 The RamenDR authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterStatus) DeepCopyInto(out *ClusterStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterStatus.
func (in *ClusterStatus) DeepCopy() *ClusterStatus {
	if in == nil {
		return nil
	}
	out := new(ClusterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DRPlacementControl) DeepCopyInto(out *DRPlacementControl) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DRPlacementControl.
func (in *DRPlacementControl) DeepCopy() *DRPlacementControl {
	if in == nil {
		return nil
	}
	out := new(DRPlacementControl)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DRPlacementControl) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DRPlacementControlList) DeepCopyInto(out *DRPlacementControlList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DRPlacementControl, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DRPlacementControlList.
func (in *DRPlacementControlList) DeepCopy() *DRPlacementControlList {
	if in == nil {
		return nil
	}
	out := new(DRPlacementControlList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DRPlacementControlList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DRPlacementControlSpec) DeepCopyInto(out *DRPlacementControlSpec) {
	*out = *in
	out.PlacementRef = in.PlacementRef
	out.DRPolicyRef = in.DRPolicyRef
	in.PVCSelector.DeepCopyInto(&out.PVCSelector)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DRPlacementControlSpec.
func (in *DRPlacementControlSpec) DeepCopy() *DRPlacementControlSpec {
	if in == nil {
		return nil
	}
	out := new(DRPlacementControlSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DRPlacementControlStatus) DeepCopyInto(out *DRPlacementControlStatus) {
	*out = *in
	out.PreferredDecision = in.PreferredDecision
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.ResourceConditions.DeepCopyInto(&out.ResourceConditions)
	in.LastUpdateTime.DeepCopyInto(&out.LastUpdateTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DRPlacementControlStatus.
func (in *DRPlacementControlStatus) DeepCopy() *DRPlacementControlStatus {
	if in == nil {
		return nil
	}
	out := new(DRPlacementControlStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DRPolicy) DeepCopyInto(out *DRPolicy) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DRPolicy.
func (in *DRPolicy) DeepCopy() *DRPolicy {
	if in == nil {
		return nil
	}
	out := new(DRPolicy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DRPolicy) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DRPolicyList) DeepCopyInto(out *DRPolicyList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DRPolicy, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DRPolicyList.
func (in *DRPolicyList) DeepCopy() *DRPolicyList {
	if in == nil {
		return nil
	}
	out := new(DRPolicyList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DRPolicyList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DRPolicySpec) DeepCopyInto(out *DRPolicySpec) {
	*out = *in
	in.ReplicationClassSelector.DeepCopyInto(&out.ReplicationClassSelector)
	if in.DRClusterSet != nil {
		in, out := &in.DRClusterSet, &out.DRClusterSet
		*out = make([]ManagedCluster, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DRPolicySpec.
func (in *DRPolicySpec) DeepCopy() *DRPolicySpec {
	if in == nil {
		return nil
	}
	out := new(DRPolicySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DRPolicyStatus) DeepCopyInto(out *DRPolicyStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.DRClusters != nil {
		in, out := &in.DRClusters, &out.DRClusters
		*out = make(map[string]ClusterStatus, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DRPolicyStatus.
func (in *DRPolicyStatus) DeepCopy() *DRPolicyStatus {
	if in == nil {
		return nil
	}
	out := new(DRPolicyStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ManagedCluster) DeepCopyInto(out *ManagedCluster) {
	*out = *in
	if in.CIDRs != nil {
		in, out := &in.CIDRs, &out.CIDRs
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ManagedCluster.
func (in *ManagedCluster) DeepCopy() *ManagedCluster {
	if in == nil {
		return nil
	}
	out := new(ManagedCluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProtectedPVC) DeepCopyInto(out *ProtectedPVC) {
	*out = *in
	if in.StorageClassName != nil {
		in, out := &in.StorageClassName, &out.StorageClassName
		*out = new(string)
		**out = **in
	}
	if in.AccessModes != nil {
		in, out := &in.AccessModes, &out.AccessModes
		*out = make([]corev1.PersistentVolumeAccessMode, len(*in))
		copy(*out, *in)
	}
	in.Resources.DeepCopyInto(&out.Resources)
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProtectedPVC.
func (in *ProtectedPVC) DeepCopy() *ProtectedPVC {
	if in == nil {
		return nil
	}
	out := new(ProtectedPVC)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RamenConfig) DeepCopyInto(out *RamenConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ControllerManagerConfigurationSpec.DeepCopyInto(&out.ControllerManagerConfigurationSpec)
	if in.S3StoreProfiles != nil {
		in, out := &in.S3StoreProfiles, &out.S3StoreProfiles
		*out = make([]S3StoreProfile, len(*in))
		copy(*out, *in)
	}
	if in.VolSyncProfiles != nil {
		in, out := &in.VolSyncProfiles, &out.VolSyncProfiles
		*out = make([]VolSyncProfile, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	out.DrClusterOperator = in.DrClusterOperator
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RamenConfig.
func (in *RamenConfig) DeepCopy() *RamenConfig {
	if in == nil {
		return nil
	}
	out := new(RamenConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RamenConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *S3StoreProfile) DeepCopyInto(out *S3StoreProfile) {
	*out = *in
	out.S3SecretRef = in.S3SecretRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new S3StoreProfile.
func (in *S3StoreProfile) DeepCopy() *S3StoreProfile {
	if in == nil {
		return nil
	}
	out := new(S3StoreProfile)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VRGAsyncSpec) DeepCopyInto(out *VRGAsyncSpec) {
	*out = *in
	in.ReplicationClassSelector.DeepCopyInto(&out.ReplicationClassSelector)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VRGAsyncSpec.
func (in *VRGAsyncSpec) DeepCopy() *VRGAsyncSpec {
	if in == nil {
		return nil
	}
	out := new(VRGAsyncSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VRGConditions) DeepCopyInto(out *VRGConditions) {
	*out = *in
	out.ResourceMeta = in.ResourceMeta
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VRGConditions.
func (in *VRGConditions) DeepCopy() *VRGConditions {
	if in == nil {
		return nil
	}
	out := new(VRGConditions)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VRGResourceMeta) DeepCopyInto(out *VRGResourceMeta) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VRGResourceMeta.
func (in *VRGResourceMeta) DeepCopy() *VRGResourceMeta {
	if in == nil {
		return nil
	}
	out := new(VRGResourceMeta)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VRGSyncSpec) DeepCopyInto(out *VRGSyncSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VRGSyncSpec.
func (in *VRGSyncSpec) DeepCopy() *VRGSyncSpec {
	if in == nil {
		return nil
	}
	out := new(VRGSyncSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolSyncProfile) DeepCopyInto(out *VolSyncProfile) {
	*out = *in
	if in.ServiceType != nil {
		in, out := &in.ServiceType, &out.ServiceType
		*out = new(corev1.ServiceType)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolSyncProfile.
func (in *VolSyncProfile) DeepCopy() *VolSyncProfile {
	if in == nil {
		return nil
	}
	out := new(VolSyncProfile)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolSyncReplicationDestinationInfo) DeepCopyInto(out *VolSyncReplicationDestinationInfo) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolSyncReplicationDestinationInfo.
func (in *VolSyncReplicationDestinationInfo) DeepCopy() *VolSyncReplicationDestinationInfo {
	if in == nil {
		return nil
	}
	out := new(VolSyncReplicationDestinationInfo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolSyncReplicationDestinationSpec) DeepCopyInto(out *VolSyncReplicationDestinationSpec) {
	*out = *in
	in.ProtectedPVC.DeepCopyInto(&out.ProtectedPVC)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolSyncReplicationDestinationSpec.
func (in *VolSyncReplicationDestinationSpec) DeepCopy() *VolSyncReplicationDestinationSpec {
	if in == nil {
		return nil
	}
	out := new(VolSyncReplicationDestinationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolSyncReplicationSourceSpec) DeepCopyInto(out *VolSyncReplicationSourceSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolSyncReplicationSourceSpec.
func (in *VolSyncReplicationSourceSpec) DeepCopy() *VolSyncReplicationSourceSpec {
	if in == nil {
		return nil
	}
	out := new(VolSyncReplicationSourceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolSyncReplicationStatus) DeepCopyInto(out *VolSyncReplicationStatus) {
	*out = *in
	if in.RDInfo != nil {
		in, out := &in.RDInfo, &out.RDInfo
		*out = make([]VolSyncReplicationDestinationInfo, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolSyncReplicationStatus.
func (in *VolSyncReplicationStatus) DeepCopy() *VolSyncReplicationStatus {
	if in == nil {
		return nil
	}
	out := new(VolSyncReplicationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolSyncRsyncSpec) DeepCopyInto(out *VolSyncRsyncSpec) {
	*out = *in
	if in.RDSpec != nil {
		in, out := &in.RDSpec, &out.RDSpec
		*out = make([]VolSyncReplicationDestinationSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.RSSpec != nil {
		in, out := &in.RSSpec, &out.RSSpec
		*out = make([]VolSyncReplicationSourceSpec, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolSyncRsyncSpec.
func (in *VolSyncRsyncSpec) DeepCopy() *VolSyncRsyncSpec {
	if in == nil {
		return nil
	}
	out := new(VolSyncRsyncSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolumeReplicationGroup) DeepCopyInto(out *VolumeReplicationGroup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolumeReplicationGroup.
func (in *VolumeReplicationGroup) DeepCopy() *VolumeReplicationGroup {
	if in == nil {
		return nil
	}
	out := new(VolumeReplicationGroup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VolumeReplicationGroup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolumeReplicationGroupList) DeepCopyInto(out *VolumeReplicationGroupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]VolumeReplicationGroup, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolumeReplicationGroupList.
func (in *VolumeReplicationGroupList) DeepCopy() *VolumeReplicationGroupList {
	if in == nil {
		return nil
	}
	out := new(VolumeReplicationGroupList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VolumeReplicationGroupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolumeReplicationGroupSpec) DeepCopyInto(out *VolumeReplicationGroupSpec) {
	*out = *in
	in.PVCSelector.DeepCopyInto(&out.PVCSelector)
	if in.S3Profiles != nil {
		in, out := &in.S3Profiles, &out.S3Profiles
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	in.Async.DeepCopyInto(&out.Async)
	out.Sync = in.Sync
	if in.VolSync != nil {
		in, out := &in.VolSync, &out.VolSync
		*out = new(VolSyncRsyncSpec)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolumeReplicationGroupSpec.
func (in *VolumeReplicationGroupSpec) DeepCopy() *VolumeReplicationGroupSpec {
	if in == nil {
		return nil
	}
	out := new(VolumeReplicationGroupSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VolumeReplicationGroupStatus) DeepCopyInto(out *VolumeReplicationGroupStatus) {
	*out = *in
	if in.ProtectedPVCs != nil {
		in, out := &in.ProtectedPVCs, &out.ProtectedPVCs
		*out = make([]ProtectedPVC, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.VSRStatus != nil {
		in, out := &in.VSRStatus, &out.VSRStatus
		*out = new(VolSyncReplicationStatus)
		(*in).DeepCopyInto(*out)
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.LastUpdateTime.DeepCopyInto(&out.LastUpdateTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VolumeReplicationGroupStatus.
func (in *VolumeReplicationGroupStatus) DeepCopy() *VolumeReplicationGroupStatus {
	if in == nil {
		return nil
	}
	out := new(VolumeReplicationGroupStatus)
	in.DeepCopyInto(out)
	return out
}

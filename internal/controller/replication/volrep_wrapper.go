// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	volrep "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VolrepVolumeGroupReplication wraps volrep.VolumeGroupReplication to implement the interface
type VolrepVolumeGroupReplication struct {
	*volrep.VolumeGroupReplication
}

// GetSpec returns the spec
func (v *VolrepVolumeGroupReplication) GetSpec() VolumeGroupReplicationSpecInterface {
	return &VolrepVolumeGroupReplicationSpec{Spec: &v.VolumeGroupReplication.Spec}
}

// GetStatus returns the status
func (v *VolrepVolumeGroupReplication) GetStatus() VolumeGroupReplicationStatusInterface {
	return &VolrepVolumeGroupReplicationStatus{Status: &v.VolumeGroupReplication.Status}
}

// SetSpec sets the spec
func (v *VolrepVolumeGroupReplication) SetSpec(spec VolumeGroupReplicationSpecInterface) {
	if s, ok := spec.(*VolrepVolumeGroupReplicationSpec); ok {
		v.VolumeGroupReplication.Spec = *s.Spec
	}
}

// SetStatus sets the status
func (v *VolrepVolumeGroupReplication) SetStatus(status VolumeGroupReplicationStatusInterface) {
	if s, ok := status.(*VolrepVolumeGroupReplicationStatus); ok {
		v.VolumeGroupReplication.Status = *s.Status
	}
}

// VolrepVolumeGroupReplicationSpec wraps volrep.VolumeGroupReplicationSpec
type VolrepVolumeGroupReplicationSpec struct {
	Spec *volrep.VolumeGroupReplicationSpec
}

func (s *VolrepVolumeGroupReplicationSpec) GetReplicationState() ReplicationState {
	return ReplicationState(s.Spec.ReplicationState)
}

func (s *VolrepVolumeGroupReplicationSpec) GetVolumeGroupReplicationClassName() string {
	return s.Spec.VolumeGroupReplicationClassName
}

func (s *VolrepVolumeGroupReplicationSpec) GetVolumeGroupReplicationContentName() string {
	return s.Spec.VolumeGroupReplicationContentName
}

func (s *VolrepVolumeGroupReplicationSpec) GetSource() VolumeGroupReplicationSourceInterface {
	return &VolrepVolumeGroupReplicationSource{Source: &s.Spec.Source}
}

func (s *VolrepVolumeGroupReplicationSpec) GetAutoResync() bool {
	return s.Spec.AutoResync
}

func (s *VolrepVolumeGroupReplicationSpec) GetReplicationHandle() string {
	// volrep doesn't have ReplicationHandle field, return empty string
	return ""
}

func (s *VolrepVolumeGroupReplicationSpec) GetExternal() bool {
	return s.Spec.External
}

func (s *VolrepVolumeGroupReplicationSpec) SetReplicationState(state ReplicationState) {
	s.Spec.ReplicationState = volrep.ReplicationState(state)
}

func (s *VolrepVolumeGroupReplicationSpec) SetExternal(external bool) {
	s.Spec.External = external
}

func (s *VolrepVolumeGroupReplicationSpec) SetVolumeGroupReplicationClassName(name string) {
	s.Spec.VolumeGroupReplicationClassName = name
}

func (s *VolrepVolumeGroupReplicationSpec) SetVolumeGroupReplicationContentName(name string) {
	s.Spec.VolumeGroupReplicationContentName = name
}

func (s *VolrepVolumeGroupReplicationSpec) SetAutoResync(autoResync bool) {
	s.Spec.AutoResync = autoResync
}

func (s *VolrepVolumeGroupReplicationSpec) SetReplicationHandle(handle string) {
	// volrep doesn't have ReplicationHandle field, no-op
}

// VolrepVolumeGroupReplicationSource wraps volrep.VolumeGroupReplicationSource
type VolrepVolumeGroupReplicationSource struct {
	Source *volrep.VolumeGroupReplicationSource
}

func (s *VolrepVolumeGroupReplicationSource) GetSelector() *metav1.LabelSelector {
	return s.Source.Selector
}

func (s *VolrepVolumeGroupReplicationSource) GetVolumeGroupReplicationContentName() *string {
	// volrep doesn't have this field, return nil
	return nil
}

func (s *VolrepVolumeGroupReplicationSource) SetSelector(selector *metav1.LabelSelector) {
	s.Source.Selector = selector
}

// VolrepVolumeGroupReplicationStatus wraps volrep.VolumeGroupReplicationStatus
type VolrepVolumeGroupReplicationStatus struct {
	Status *volrep.VolumeGroupReplicationStatus
}

func (s *VolrepVolumeGroupReplicationStatus) GetState() State {
	return State(s.Status.State)
}

func (s *VolrepVolumeGroupReplicationStatus) GetMessage() string {
	return s.Status.Message
}

func (s *VolrepVolumeGroupReplicationStatus) GetObservedGeneration() int64 {
	return s.Status.ObservedGeneration
}

func (s *VolrepVolumeGroupReplicationStatus) GetLastSyncTime() *metav1.Time {
	return s.Status.LastSyncTime
}

func (s *VolrepVolumeGroupReplicationStatus) GetLastSyncDuration() *metav1.Duration {
	return s.Status.LastSyncDuration
}

func (s *VolrepVolumeGroupReplicationStatus) GetLastSyncBytes() *int64 {
	return s.Status.LastSyncBytes
}

func (s *VolrepVolumeGroupReplicationStatus) GetLastCompletionTime() *metav1.Time {
	return s.Status.LastCompletionTime
}

func (s *VolrepVolumeGroupReplicationStatus) GetLastStartTime() *metav1.Time {
	return s.Status.LastStartTime
}

func (s *VolrepVolumeGroupReplicationStatus) GetConditions() []metav1.Condition {
	return s.Status.Conditions
}

func (s *VolrepVolumeGroupReplicationStatus) GetPersistentVolumeClaimsRefList() []corev1.LocalObjectReference {
	return s.Status.PersistentVolumeClaimsRefList
}

func (s *VolrepVolumeGroupReplicationStatus) SetState(state State) {
	s.Status.State = volrep.State(state)
}

func (s *VolrepVolumeGroupReplicationStatus) SetMessage(message string) {
	s.Status.Message = message
}

func (s *VolrepVolumeGroupReplicationStatus) SetObservedGeneration(gen int64) {
	s.Status.ObservedGeneration = gen
}

func (s *VolrepVolumeGroupReplicationStatus) SetConditions(conditions []metav1.Condition) {
	s.Status.Conditions = conditions
}

func (s *VolrepVolumeGroupReplicationStatus) SetPersistentVolumeClaimsRefList(refs []corev1.LocalObjectReference) {
	s.Status.PersistentVolumeClaimsRefList = refs
}

// VolrepVolumeGroupReplicationClass wraps volrep.VolumeGroupReplicationClass
type VolrepVolumeGroupReplicationClass struct {
	*volrep.VolumeGroupReplicationClass
}

func (v *VolrepVolumeGroupReplicationClass) GetSpec() VolumeGroupReplicationClassSpecInterface {
	return &VolrepVolumeGroupReplicationClassSpec{Spec: &v.VolumeGroupReplicationClass.Spec}
}

// VolrepVolumeGroupReplicationClassSpec wraps volrep.VolumeGroupReplicationClassSpec
type VolrepVolumeGroupReplicationClassSpec struct {
	Spec *volrep.VolumeGroupReplicationClassSpec
}

func (s *VolrepVolumeGroupReplicationClassSpec) GetProvisioner() string {
	return s.Spec.Provisioner
}

func (s *VolrepVolumeGroupReplicationClassSpec) GetParameters() map[string]string {
	return s.Spec.Parameters
}

// VolrepVolumeGroupReplicationClassList wraps volrep.VolumeGroupReplicationClassList
type VolrepVolumeGroupReplicationClassList struct {
	*volrep.VolumeGroupReplicationClassList
}

func (v *VolrepVolumeGroupReplicationClassList) GetItems() []VolumeGroupReplicationClassInterface {
	items := make([]VolumeGroupReplicationClassInterface, len(v.VolumeGroupReplicationClassList.Items))
	for i := range v.VolumeGroupReplicationClassList.Items {
		items[i] = &VolrepVolumeGroupReplicationClass{VolumeGroupReplicationClass: &v.VolumeGroupReplicationClassList.Items[i]}
	}
	return items
}

func (v *VolrepVolumeGroupReplicationClassList) SetItems(items []VolumeGroupReplicationClassInterface) {
	v.VolumeGroupReplicationClassList.Items = make([]volrep.VolumeGroupReplicationClass, len(items))
	for i, item := range items {
		if vgrc, ok := item.(*VolrepVolumeGroupReplicationClass); ok {
			v.VolumeGroupReplicationClassList.Items[i] = *vgrc.VolumeGroupReplicationClass
		}
	}
}

// VolrepVolumeGroupReplicationContent wraps volrep.VolumeGroupReplicationContent
type VolrepVolumeGroupReplicationContent struct {
	*volrep.VolumeGroupReplicationContent
}

func (v *VolrepVolumeGroupReplicationContent) GetSpec() VolumeGroupReplicationContentSpecInterface {
	return &VolrepVolumeGroupReplicationContentSpec{Spec: &v.VolumeGroupReplicationContent.Spec}
}

func (v *VolrepVolumeGroupReplicationContent) GetStatus() VolumeGroupReplicationContentStatusInterface {
	return &VolrepVolumeGroupReplicationContentStatus{Status: &v.VolumeGroupReplicationContent.Status}
}

// VolrepVolumeGroupReplicationContentSpec wraps volrep.VolumeGroupReplicationContentSpec
type VolrepVolumeGroupReplicationContentSpec struct {
	Spec *volrep.VolumeGroupReplicationContentSpec
}

func (s *VolrepVolumeGroupReplicationContentSpec) GetVolumeGroupReplicationRef() corev1.ObjectReference {
	if s.Spec.VolumeGroupReplicationRef != nil {
		return *s.Spec.VolumeGroupReplicationRef
	}
	return corev1.ObjectReference{}
}

func (s *VolrepVolumeGroupReplicationContentSpec) GetVolumeGroupReplicationClassName() string {
	return s.Spec.VolumeGroupReplicationClassName
}

func (s *VolrepVolumeGroupReplicationContentSpec) GetProvisioner() string {
	return s.Spec.Provisioner
}

func (s *VolrepVolumeGroupReplicationContentSpec) GetParameters() map[string]string {
	// volrep doesn't have Parameters field in Content spec
	return nil
}

// VolrepVolumeGroupReplicationContentStatus wraps volrep.VolumeGroupReplicationContentStatus
type VolrepVolumeGroupReplicationContentStatus struct {
	Status *volrep.VolumeGroupReplicationContentStatus
}

func (s *VolrepVolumeGroupReplicationContentStatus) GetVolumeReplicationContentRefList() []corev1.LocalObjectReference {
	// volrep doesn't have this field
	return nil
}

func (s *VolrepVolumeGroupReplicationContentStatus) GetCreationTime() *int64 {
	// volrep doesn't have this field
	return nil
}

func (s *VolrepVolumeGroupReplicationContentStatus) GetReadyToUse() *bool {
	// volrep doesn't have this field
	return nil
}

// Made with Bob

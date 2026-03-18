// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	neutral "github.com/BenamarMk/replication-storage-io-crds/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NeutralVolumeGroupReplication wraps neutral.VolumeGroupReplication to implement the interface
type NeutralVolumeGroupReplication struct {
	*neutral.VolumeGroupReplication
}

// GetSpec returns the spec
func (v *NeutralVolumeGroupReplication) GetSpec() VolumeGroupReplicationSpecInterface {
	return &NeutralVolumeGroupReplicationSpec{Spec: &v.VolumeGroupReplication.Spec}
}

// GetStatus returns the status
func (v *NeutralVolumeGroupReplication) GetStatus() VolumeGroupReplicationStatusInterface {
	return &NeutralVolumeGroupReplicationStatus{Status: &v.VolumeGroupReplication.Status}
}

// SetSpec sets the spec
func (v *NeutralVolumeGroupReplication) SetSpec(spec VolumeGroupReplicationSpecInterface) {
	if s, ok := spec.(*NeutralVolumeGroupReplicationSpec); ok {
		v.VolumeGroupReplication.Spec = *s.Spec
	}
}

// SetStatus sets the status
func (v *NeutralVolumeGroupReplication) SetStatus(status VolumeGroupReplicationStatusInterface) {
	if s, ok := status.(*NeutralVolumeGroupReplicationStatus); ok {
		v.VolumeGroupReplication.Status = *s.Status
	}
}

// NeutralVolumeGroupReplicationSpec wraps neutral.VolumeGroupReplicationSpec
type NeutralVolumeGroupReplicationSpec struct {
	Spec *neutral.VolumeGroupReplicationSpec
}

func (s *NeutralVolumeGroupReplicationSpec) GetReplicationState() ReplicationState {
	return ReplicationState(s.Spec.ReplicationState)
}

func (s *NeutralVolumeGroupReplicationSpec) GetVolumeGroupReplicationClassName() string {
	return s.Spec.VolumeGroupReplicationClassName
}

func (s *NeutralVolumeGroupReplicationSpec) GetVolumeGroupReplicationContentName() string {
	return s.Spec.VolumeGroupReplicationContentName
}

func (s *NeutralVolumeGroupReplicationSpec) GetSource() VolumeGroupReplicationSourceInterface {
	return &NeutralVolumeGroupReplicationSource{Source: &s.Spec.Source}
}

func (s *NeutralVolumeGroupReplicationSpec) GetAutoResync() bool {
	return s.Spec.AutoResync
}

func (s *NeutralVolumeGroupReplicationSpec) GetReplicationHandle() string {
	return s.Spec.ReplicationHandle
}

func (s *NeutralVolumeGroupReplicationSpec) SetReplicationState(state ReplicationState) {
	s.Spec.ReplicationState = neutral.ReplicationState(state)
}

func (s *NeutralVolumeGroupReplicationSpec) SetVolumeGroupReplicationClassName(name string) {
	s.Spec.VolumeGroupReplicationClassName = name
}

func (s *NeutralVolumeGroupReplicationSpec) SetVolumeGroupReplicationContentName(name string) {
	s.Spec.VolumeGroupReplicationContentName = name
}

func (s *NeutralVolumeGroupReplicationSpec) SetAutoResync(autoResync bool) {
	s.Spec.AutoResync = autoResync
}

func (s *NeutralVolumeGroupReplicationSpec) SetReplicationHandle(handle string) {
	s.Spec.ReplicationHandle = handle
}

// NeutralVolumeGroupReplicationSource wraps neutral.VolumeGroupReplicationSource
type NeutralVolumeGroupReplicationSource struct {
	Source *neutral.VolumeGroupReplicationSource
}

func (s *NeutralVolumeGroupReplicationSource) GetSelector() *metav1.LabelSelector {
	return s.Source.Selector
}

func (s *NeutralVolumeGroupReplicationSource) GetVolumeGroupReplicationContentName() *string {
	return s.Source.VolumeGroupReplicationContentName
}

func (s *NeutralVolumeGroupReplicationSource) SetSelector(selector *metav1.LabelSelector) {
	s.Source.Selector = selector
}

// NeutralVolumeGroupReplicationStatus wraps neutral.VolumeGroupReplicationStatus
type NeutralVolumeGroupReplicationStatus struct {
	Status *neutral.VolumeGroupReplicationStatus
}

func (s *NeutralVolumeGroupReplicationStatus) GetState() State {
	return State(s.Status.State)
}

func (s *NeutralVolumeGroupReplicationStatus) GetMessage() string {
	return s.Status.Message
}

func (s *NeutralVolumeGroupReplicationStatus) GetObservedGeneration() int64 {
	return s.Status.ObservedGeneration
}

func (s *NeutralVolumeGroupReplicationStatus) GetLastSyncTime() *metav1.Time {
	return s.Status.LastSyncTime
}

func (s *NeutralVolumeGroupReplicationStatus) GetLastSyncDuration() *metav1.Duration {
	return s.Status.LastSyncDuration
}

func (s *NeutralVolumeGroupReplicationStatus) GetLastSyncBytes() *int64 {
	return s.Status.LastSyncBytes
}

func (s *NeutralVolumeGroupReplicationStatus) GetLastCompletionTime() *metav1.Time {
	return s.Status.LastCompletionTime
}

func (s *NeutralVolumeGroupReplicationStatus) GetLastStartTime() *metav1.Time {
	return s.Status.LastStartTime
}

func (s *NeutralVolumeGroupReplicationStatus) GetConditions() []metav1.Condition {
	return s.Status.Conditions
}

func (s *NeutralVolumeGroupReplicationStatus) GetPersistentVolumeClaimsRefList() []corev1.LocalObjectReference {
	return s.Status.PersistentVolumeClaimsRefList
}

func (s *NeutralVolumeGroupReplicationStatus) SetState(state State) {
	s.Status.State = neutral.State(state)
}

func (s *NeutralVolumeGroupReplicationStatus) SetMessage(message string) {
	s.Status.Message = message
}

func (s *NeutralVolumeGroupReplicationStatus) SetObservedGeneration(gen int64) {
	s.Status.ObservedGeneration = gen
}

func (s *NeutralVolumeGroupReplicationStatus) SetConditions(conditions []metav1.Condition) {
	s.Status.Conditions = conditions
}

func (s *NeutralVolumeGroupReplicationStatus) SetPersistentVolumeClaimsRefList(refs []corev1.LocalObjectReference) {
	s.Status.PersistentVolumeClaimsRefList = refs
}

// NeutralVolumeGroupReplicationClass wraps neutral.VolumeGroupReplicationClass
type NeutralVolumeGroupReplicationClass struct {
	*neutral.VolumeGroupReplicationClass
}

func (v *NeutralVolumeGroupReplicationClass) GetSpec() VolumeGroupReplicationClassSpecInterface {
	return &NeutralVolumeGroupReplicationClassSpec{Spec: &v.VolumeGroupReplicationClass.Spec}
}

// NeutralVolumeGroupReplicationClassSpec wraps neutral.VolumeGroupReplicationClassSpec
type NeutralVolumeGroupReplicationClassSpec struct {
	Spec *neutral.VolumeGroupReplicationClassSpec
}

func (s *NeutralVolumeGroupReplicationClassSpec) GetProvisioner() string {
	return s.Spec.Provisioner
}

func (s *NeutralVolumeGroupReplicationClassSpec) GetParameters() map[string]string {
	return s.Spec.Parameters
}

// NeutralVolumeGroupReplicationContent wraps neutral.VolumeGroupReplicationContent
type NeutralVolumeGroupReplicationContent struct {
	*neutral.VolumeGroupReplicationContent
}

func (v *NeutralVolumeGroupReplicationContent) GetSpec() VolumeGroupReplicationContentSpecInterface {
	return &NeutralVolumeGroupReplicationContentSpec{Spec: &v.VolumeGroupReplicationContent.Spec}
}

func (v *NeutralVolumeGroupReplicationContent) GetStatus() VolumeGroupReplicationContentStatusInterface {
	return &NeutralVolumeGroupReplicationContentStatus{Status: &v.VolumeGroupReplicationContent.Status}
}

// NeutralVolumeGroupReplicationContentSpec wraps neutral.VolumeGroupReplicationContentSpec
type NeutralVolumeGroupReplicationContentSpec struct {
	Spec *neutral.VolumeGroupReplicationContentSpec
}

func (s *NeutralVolumeGroupReplicationContentSpec) GetVolumeGroupReplicationRef() corev1.ObjectReference {
	return s.Spec.VolumeGroupReplicationRef
}

func (s *NeutralVolumeGroupReplicationContentSpec) GetVolumeGroupReplicationClassName() string {
	return s.Spec.VolumeGroupReplicationClassName
}

func (s *NeutralVolumeGroupReplicationContentSpec) GetProvisioner() string {
	return s.Spec.Provisioner
}

func (s *NeutralVolumeGroupReplicationContentSpec) GetParameters() map[string]string {
	return s.Spec.Parameters
}

// NeutralVolumeGroupReplicationContentStatus wraps neutral.VolumeGroupReplicationContentStatus
type NeutralVolumeGroupReplicationContentStatus struct {
	Status *neutral.VolumeGroupReplicationContentStatus
}

func (s *NeutralVolumeGroupReplicationContentStatus) GetVolumeReplicationContentRefList() []corev1.LocalObjectReference {
	return s.Status.VolumeReplicationContentRefList
}

func (s *NeutralVolumeGroupReplicationContentStatus) GetCreationTime() *int64 {
	return s.Status.CreationTime
}

func (s *NeutralVolumeGroupReplicationContentStatus) GetReadyToUse() *bool {
	return s.Status.ReadyToUse
}

// Made with Bob

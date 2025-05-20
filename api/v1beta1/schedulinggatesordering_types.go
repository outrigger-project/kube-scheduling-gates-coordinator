/*
Copyright 2025 outrigger-project.

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

package v1beta1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SchedulingGatesOrderingSpec defines the desired state of SchedulingGatesOrdering
type SchedulingGatesOrderingSpec struct {
	// +kubebuilder:validation:Required
	// SchedulingGates is a list of scheduling gates that are used to control the scheduling of pods.
	// kube-scheduling-gates-coordinator ensures that the scheduling gates are set in the correct order by the other
	// webhooks by re-ordering the scheduling gates in the pod spec at admission time.
	SchedulingGates []v1.PodSchedulingGate `json:"schedulingGates"`
}

// SchedulingGatesOrderingStatus defines the observed state of SchedulingGatesOrdering
type SchedulingGatesOrderingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// SchedulingGatesOrdering is the Schema for the schedulinggatesorderings API
type SchedulingGatesOrdering struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SchedulingGatesOrderingSpec   `json:"spec,omitempty"`
	Status SchedulingGatesOrderingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SchedulingGatesOrderingList contains a list of SchedulingGatesOrdering
type SchedulingGatesOrderingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SchedulingGatesOrdering `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SchedulingGatesOrdering{}, &SchedulingGatesOrderingList{})
}

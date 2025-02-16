/*
Copyright 2025.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EventSpec defines the desired state of Event
type EventSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Optional
	Intruder Intruder `json:"intruder,omitempty"`

	Operations []*Operation `json:"operations,omitempty"`
}

type Intruder struct {
	// +kubebuilder:validation:Optional
	User *IntruderUser `json:"user,omitempty"`
	// +kubebuilder:validation:Optional
	ServiceAccount string `json:"serviceAccount,omitempty"`
	// +kubebuilder:validation:Optional
	Token string `json:"token,omitempty"`
}

type IntruderUser struct {
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:Optional
	Groups []string `json:"groups,omitempty"`
}
type Operation struct {
	// +kubebuilder:validation:Required
	ID string `json:"id,omitempty"`
	// +kubebuilder:validation:Required
	Resource runtime.RawExtension `json:"resource,omitempty"`

	Verb string `json:"verb,omitempty"`
}

// EventStatus defines the observed state of Event
type EventStatus struct {
	Result string `json:"result,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Result",type="string",JSONPath=".status.result",description="Result"

// Event is the Schema for the events API
type Event struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventSpec   `json:"spec,omitempty"`
	Status EventStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EventList contains a list of Event
type EventList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Event `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Event{}, &EventList{})
}

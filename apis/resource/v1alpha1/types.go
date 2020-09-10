/*
Copyright 2020 The Crossplane Authors.

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

package v1alpha1

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KubernetesResourceParameters are the configurable fields of a KubernetesResource.
type KubernetesResourceParameters struct {
	// A Template for a Kubernetes resource to be submitted to the
	// KubernetesCluster to which this application resource is scheduled. The
	// resource must be understood by the KubernetesCluster. Crossplane requires
	// only that the resource contains standard Kubernetes type and object
	// metadata.
	Object runtime.RawExtension `json:"object"`
}

// KubernetesResourceObservation are the observable fields of a KubernetesResource.
type KubernetesResourceObservation struct {
	// Raw JSON representation of the remote status as a byte array.
	Object runtime.RawExtension `json:"object,omitempty"`
}

// A KubernetesResourceSpec defines the desired state of a KubernetesResource.
type KubernetesResourceSpec struct {
	runtimev1alpha1.ResourceSpec `json:",inline"`
	ForProvider                  KubernetesResourceParameters `json:"forProvider"`
}

// A KubernetesResourceStatus represents the observed state of a KubernetesResource.
type KubernetesResourceStatus struct {
	runtimev1alpha1.ResourceStatus `json:",inline"`
	AtProvider                     KubernetesResourceObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A KubernetesResource is an example API type
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="STATE",type="string",JSONPath=".status.atProvider.state"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster
type KubernetesResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubernetesResourceSpec   `json:"spec"`
	Status KubernetesResourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubernetesResourceList contains a list of KubernetesResource
type KubernetesResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubernetesResource `json:"items"`
}

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodeConfigurationSpec struct {
	Taints []corev1.Taint `json:"taints,omitempty"`
}

type NodeConfigurationMetadata struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type NodeConfiguration struct {
	Metadata NodeConfigurationMetadata `json:"metadata,omitempty"`
	Spec     NodeConfigurationSpec     `json:"spec,omitempty"`
}

type NodeTemplateSpec struct {
	// +kubebuilder:default: false
	DryRun bool `json:"dryRun,omitempty"`
	// +kubebuilder:default: 1
	Priority int                   `json:"priority"`
	Selector *metav1.LabelSelector `json:"selector"`
	// +kubebuilder:validation:Minimum=1
	Nodes    int               `json:"nodes"`
	Template NodeConfiguration `json:"template,omitempty"`
}

type ReconcileInfo struct {
	Generation int64 `json:"generation,omitempty"`
}

type NodeTemplateStatus struct {
	CurrentNodes  int           `json:"currentNodes"`
	Sufficient    bool          `json:"sufficient"`
	ReconcileInfo ReconcileInfo `json:"reconcileInfo"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="current",type="integer",JSONPath=`.status.currentNodes`,description="Current Nodes"
// +kubebuilder:printcolumn:name="desired",type="integer",JSONPath=`.spec.nodes`,description="Desired Nodes"
// +kubebuilder:printcolumn:name="sufficient",type="boolean",JSONPath=`.status.sufficient`,description="Sufficient Nodes"
// NodeTemplate is the Schema for the nodetemplates API.
type NodeTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   NodeTemplateSpec   `json:"spec"`
	Status NodeTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type NodeTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeTemplate{}, &NodeTemplateList{})
}

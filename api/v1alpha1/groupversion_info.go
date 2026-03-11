package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is the group version for GPU Inference Autoscaler.
	GroupVersion = schema.GroupVersion{Group: "autoscaling.gpu.k8s.infra", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the Scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

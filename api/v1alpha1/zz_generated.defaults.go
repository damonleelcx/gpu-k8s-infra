package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// RegisterDefaults sets defaults for GPUInferenceAutoscaler.
func RegisterDefaults(scheme *runtime.Scheme) error {
	scheme.AddTypeDefaultingFunc(&GPUInferenceAutoscaler{}, func(obj interface{}) {
		a := obj.(*GPUInferenceAutoscaler)
		setDefaultsGPUInferenceAutoscaler(a)
	})
	return nil
}

func setDefaultsGPUInferenceAutoscaler(a *GPUInferenceAutoscaler) {
	if a.Spec.MinReplicas == nil {
		min := int32(0)
		a.Spec.MinReplicas = &min
	}
	if a.Spec.ScaleTargetRef.APIVersion == "" {
		a.Spec.ScaleTargetRef.APIVersion = "apps/v1"
	}
	if a.Spec.ScaleTargetRef.Kind == "" {
		a.Spec.ScaleTargetRef.Kind = "Deployment"
	}
	if a.Spec.SyncPeriodSeconds == nil {
		s := int32(15)
		a.Spec.SyncPeriodSeconds = &s
	}
	if a.Spec.Prediction != nil && a.Spec.Prediction.Enable {
		if a.Spec.Prediction.LookbackWindowSeconds == 0 {
			a.Spec.Prediction.LookbackWindowSeconds = 300
		}
		if a.Spec.Prediction.PreScaleSeconds == 0 {
			a.Spec.Prediction.PreScaleSeconds = 60
		}
		if a.Spec.Prediction.Method == "" {
			a.Spec.Prediction.Method = "exponential"
		}
	}
	if a.Spec.ColdStart != nil {
		if a.Spec.ColdStart.EstimatedStartupSeconds == 0 {
			a.Spec.ColdStart.EstimatedStartupSeconds = 60
		}
		if a.Spec.ColdStart.ScaleUpDelaySeconds == 0 {
			a.Spec.ColdStart.ScaleUpDelaySeconds = 120
		}
	}
}

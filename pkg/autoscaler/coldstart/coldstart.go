package coldstart

import (
	"time"

	"github.com/damonleelcx/gpu-k8s-infra/api/v1alpha1"
)

// Adjust applies cold-start logic: when scaling up, add extra replicas to account for
// pods that are still starting (so we don't under-provision during startup).
func Adjust(desired int32, currentReplicas int32, spec *v1alpha1.ColdStartSpec) int32 {
	if spec == nil || desired <= currentReplicas {
		return desired
	}
	// Optional: add warm pool when scaling from zero
	add := spec.WarmPoolSize
	if currentReplicas == 0 && add > 0 {
		desired += add
	}
	return desired
}

// ScaleUpStabilizationWindow returns the minimum time to wait after a scale-up before scaling down.
func ScaleUpStabilizationWindow(spec *v1alpha1.ColdStartSpec) time.Duration {
	if spec == nil || spec.ScaleUpDelaySeconds <= 0 {
		return 0
	}
	return time.Duration(spec.ScaleUpDelaySeconds) * time.Second
}

// EstimatedStartupDuration is how long we assume a new pod takes to become ready.
func EstimatedStartupDuration(spec *v1alpha1.ColdStartSpec) time.Duration {
	if spec == nil || spec.EstimatedStartupSeconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(spec.EstimatedStartupSeconds) * time.Second
}

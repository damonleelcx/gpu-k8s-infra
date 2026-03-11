package scaler

import (
	"context"
	"fmt"
	"math"

	"github.com/damonleelcx/gpu-k8s-infra/api/v1alpha1"
	"github.com/damonleelcx/gpu-k8s-infra/pkg/autoscaler/coldstart"
	"github.com/damonleelcx/gpu-k8s-infra/pkg/autoscaler/fetcher"
	"github.com/damonleelcx/gpu-k8s-infra/pkg/autoscaler/predictor"
)

// Scaler computes desired replicas from GPUInferenceAutoscaler spec and current/historical metrics.
type Scaler struct {
	Fetcher   *fetcher.Fetcher
	Predictor *predictor.Predictor
}

// Compute returns desired replica count and the metric values used.
func (s *Scaler) Compute(ctx context.Context, a *v1alpha1.GPUInferenceAutoscaler) (desired int32, metricsUsed []v1alpha1.MetricValue, err error) {
	spec := &a.Spec
	minRep := int32(0)
	if spec.MinReplicas != nil {
		minRep = *spec.MinReplicas
	}
	maxRep := spec.MaxReplicas
	if maxRep < 1 {
		maxRep = 1
	}

	currentReplicas := a.Status.CurrentReplicas
	if currentReplicas < 0 {
		currentReplicas = 0
	}

	var desiredFromMetrics int32 = minRep
	metricsUsed = make([]v1alpha1.MetricValue, 0, len(spec.Metrics))

	for _, m := range spec.Metrics {
		value, err := s.Fetcher.GetCurrent(ctx, m)
		if err != nil {
			return currentReplicas, metricsUsed, fmt.Errorf("metric %s: %w", m.Type, err)
		}
		metricsUsed = append(metricsUsed, v1alpha1.MetricValue{Type: m.Type, Value: value})

		// Optional: use predicted value for this metric
		if spec.Prediction != nil && spec.Prediction.Enable && m.PrometheusQuery != "" {
			hist, err := s.Fetcher.GetHistorical(ctx, m.PrometheusQuery,
				int(spec.Prediction.LookbackWindowSeconds),
				int(spec.Prediction.PreScaleSeconds)/2)
			if err == nil && len(hist) > 0 && s.Predictor != nil {
				pred := s.Predictor.Predict(hist)
				if pred > value {
					value = pred
				}
			}
		}

		// desired replicas from this metric: ceil(value / targetPerReplica)
		target := m.TargetPerReplica
		if target <= 0 {
			target = 1
		}
		d := int32(math.Ceil(value / target))
		if d > desiredFromMetrics {
			desiredFromMetrics = d
		}
	}

	if len(spec.Metrics) == 0 {
		desiredFromMetrics = minRep
	}

	if desiredFromMetrics < minRep {
		desiredFromMetrics = minRep
	}
	if desiredFromMetrics > maxRep {
		desiredFromMetrics = maxRep
	}

	// Cold-start: add buffer when scaling up
	desiredFromMetrics = coldstart.Adjust(desiredFromMetrics, currentReplicas, spec.ColdStart)
	if desiredFromMetrics > maxRep {
		desiredFromMetrics = maxRep
	}

	return desiredFromMetrics, metricsUsed, nil
}

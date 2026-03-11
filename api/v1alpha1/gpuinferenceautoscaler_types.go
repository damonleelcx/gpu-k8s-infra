// Package v1alpha1 contains API types for GPU Inference Autoscaler.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=gia
// +kubebuilder:printcolumn:name="Min",type=integer,JSONPath=`.spec.minReplicas`
// +kubebuilder:printcolumn:name="Max",type=integer,JSONPath=`.spec.maxReplicas`
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.status.desiredReplicas`
// +kubebuilder:printcolumn:name="Current",type=integer,JSONPath=`.status.currentReplicas`

// GPUInferenceAutoscaler scales a Deployment based on GPU inference QPS,
// GPU utilization, queue length, with optional prediction and cold-start handling.
type GPUInferenceAutoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GPUInferenceAutoscalerSpec   `json:"spec,omitempty"`
	Status GPUInferenceAutoscalerStatus `json:"status,omitempty"`
}

// GPUInferenceAutoscalerSpec defines the desired scaling behavior.
type GPUInferenceAutoscalerSpec struct {
	// ScaleTargetRef is the Deployment to scale (must request nvidia.com/gpu if using GPU metrics).
	ScaleTargetRef CrossVersionObjectReference `json:"scaleTargetRef"`

	// MinReplicas is the minimum number of replicas (default 0 for scale-to-zero).
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the maximum number of replicas.
	MaxReplicas int32 `json:"maxReplicas"`

	// Metrics defines what to scale on (QPS, GPU utilization, queue length).
	Metrics []MetricSpec `json:"metrics"`

	// Prediction enables time-series based pre-scaling (scale before load arrives).
	Prediction *PredictionSpec `json:"prediction,omitempty"`

	// ColdStart compensates for pod startup delay when scaling from zero or adding replicas.
	ColdStart *ColdStartSpec `json:"coldStart,omitempty"`

	// SyncPeriodSeconds is how often to run the scaling decision (default 15).
	SyncPeriodSeconds *int32 `json:"syncPeriodSeconds,omitempty"`
}

// CrossVersionObjectReference points to a Deployment.
type CrossVersionObjectReference struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name"`
}

// MetricType is the type of scaling metric.
type MetricType string

const (
	MetricTypeQPS            MetricType = "QPS"            // inference requests per second
	MetricTypeGPUUtilization MetricType = "GPUUtilization" // 0-100
	MetricTypeQueueLength    MetricType = "QueueLength"    // e.g. Redis list length
)

// MetricSpec defines one metric to scale on.
type MetricSpec struct {
	Type MetricType `json:"type"`

	// Target value per replica (for QPS: target QPS per pod; for GPU: target %; for Queue: target length per pod).
	TargetPerReplica float64 `json:"targetPerReplica"`

	// PrometheusQuery is used for QPS and GPUUtilization. Must return a single value or vector.
	// Example QPS: sum(rate(inference_requests_total{job="my-inference"}[1m]))
	// Example GPU: avg(DCGM_FI_DEV_GPU_UTIL) by (pod)
	PrometheusQuery string `json:"prometheusQuery,omitempty"`

	// QueueConfig is used for MetricTypeQueueLength (Redis).
	QueueConfig *QueueMetricConfig `json:"queueConfig,omitempty"`
}

// QueueMetricConfig configures queue-length metric (e.g. Redis).
type QueueMetricConfig struct {
	// RedisAddr is "host:port" or URL.
	RedisAddr string `json:"redisAddr"`
	// Key is the Redis key (e.g. "inference:queue" for list length).
	Key string `json:"key"`
	// KeyType: "list" (LLEN), "stream" (XLEN), "set" (SCARD).
	KeyType string `json:"keyType,omitempty"`
}

// PredictionSpec configures predictive scaling.
type PredictionSpec struct {
	// Enable prediction (use recent metric history to predict and pre-scale).
	Enable bool `json:"enable"`

	// LookbackWindowSeconds is how far back to consider for prediction (default 300).
	LookbackWindowSeconds int32 `json:"lookbackWindowSeconds,omitempty"`

	// PreScaleSeconds: scale as if the predicted load will happen this many seconds in the future (default 60).
	PreScaleSeconds int32 `json:"preScaleSeconds,omitempty"`

	// Method: "linear" (linear regression) or "exponential" (exponential smoothing).
	Method string `json:"method,omitempty"`
}

// ColdStartSpec configures cold-start handling.
type ColdStartSpec struct {
	// EstimatedStartupSeconds is how long a new pod takes to be ready (default 60 for GPU load).
	EstimatedStartupSeconds int32 `json:"estimatedStartupSeconds,omitempty"`

	// WarmPoolSize: when scale-from-zero, keep at least this many extra replicas warming (default 0).
	WarmPoolSize int32 `json:"warmPoolSize,omitempty"`

	// ScaleUpDelaySeconds: wait this long after scaling up before scaling down again (stabilization).
	ScaleUpDelaySeconds int32 `json:"scaleUpDelaySeconds,omitempty"`
}

// GPUInferenceAutoscalerStatus is the current state.
type GPUInferenceAutoscalerStatus struct {
	// CurrentReplicas is the current replica count of the target Deployment.
	CurrentReplicas int32 `json:"currentReplicas"`

	// DesiredReplicas is the last computed desired replica count.
	DesiredReplicas int32 `json:"desiredReplicas"`

	// LastScaleTime is when we last scaled.
	LastScaleTime *metav1.Time `json:"lastScaleTime,omitempty"`

	// Conditions represent the latest observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// CurrentMetrics is the latest values used for scaling (for debugging).
	CurrentMetrics []MetricValue `json:"currentMetrics,omitempty"`
}

// MetricValue is a single metric value used in the last computation.
type MetricValue struct {
	Type  MetricType `json:"type"`
	Value float64    `json:"value"`
}

// +kubebuilder:object:root=true

// GPUInferenceAutoscalerList contains a list of GPUInferenceAutoscaler.
type GPUInferenceAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GPUInferenceAutoscaler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GPUInferenceAutoscaler{}, &GPUInferenceAutoscalerList{})
}

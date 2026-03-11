package k8s

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	GPUResourceName = "nvidia.com/gpu"
)

// Client wraps Kubernetes client for GPU job operations.
type Client struct {
	clientset *kubernetes.Clientset
	namespace string
}

// NewInClusterClient creates a client using in-cluster config (for running inside K8s).
func NewInClusterClient(namespace string) (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("clientset: %w", err)
	}
	return &Client{clientset: clientset, namespace: namespace}, nil
}

// NewOutOfClusterClient creates a client using kubeconfig (for local/dev).
func NewOutOfClusterClient(kubeconfigPath, namespace string) (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("clientset: %w", err)
	}
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}
	return &Client{clientset: clientset, namespace: namespace}, nil
}

// GPUJobSpec defines a GPU job to be submitted.
type GPUJobSpec struct {
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Command     []string          `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	GPUCount    int32             `json:"gpuCount"`
	CPURequest  string            `json:"cpuRequest,omitempty"`
	MemRequest  string            `json:"memoryRequest,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	TTLSeconds  int32             `json:"ttlSecondsAfterFinished,omitempty"`
}

// JobStatus represents high-level job status.
type JobStatus struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"` // Pending, Running, Succeeded, Failed
	Ready     int32  `json:"ready"`
	Active    int32  `json:"active"`
	Succeeded int32  `json:"succeeded"`
	Failed    int32  `json:"failed"`
	StartTime string `json:"startTime,omitempty"`
	Message   string `json:"message,omitempty"`
}

// CreateGPUJob creates a Kubernetes Job that requests GPU.
func (c *Client) CreateGPUJob(ctx context.Context, spec GPUJobSpec) (*batchv1.Job, error) {
	if spec.GPUCount < 1 {
		spec.GPUCount = 1
	}
	if spec.CPURequest == "" {
		spec.CPURequest = "500m"
	}
	if spec.MemRequest == "" {
		spec.MemRequest = "2Gi"
	}
	if spec.Image == "" {
		spec.Image = "nvidia/cuda:12.0.0-base-ubuntu22.04"
	}

	jobName := spec.Name
	if jobName == "" {
		jobName = fmt.Sprintf("gpu-job-%d", metav1.Now().Unix())
	}

	labels := map[string]string{
		"app":       "gpu-job",
		"job-name":  jobName,
		"platform":  "gpu-k8s-infra",
	}
	for k, v := range spec.Labels {
		labels[k] = v
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        jobName,
			Namespace:   c.namespace,
			Labels:      labels,
			Annotations: spec.Annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr(int32(0)),
			TTLSecondsAfterFinished: nil,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "gpu-worker",
							Image:   spec.Image,
							Command: spec.Command,
							Args:    spec.Args,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(spec.CPURequest),
									corev1.ResourceMemory: resource.MustParse(spec.MemRequest),
									corev1.ResourceName(GPUResourceName): *resource.NewQuantity(int64(spec.GPUCount), resource.DecimalSI),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceName(GPUResourceName): *resource.NewQuantity(int64(spec.GPUCount), resource.DecimalSI),
								},
							},
						},
					},
				},
			},
		},
	}
	if spec.TTLSeconds > 0 {
		job.Spec.TTLSecondsAfterFinished = ptr(spec.TTLSeconds)
	}

	return c.clientset.BatchV1().Jobs(c.namespace).Create(ctx, job, metav1.CreateOptions{})
}

// GetJob returns job by name.
func (c *Client) GetJob(ctx context.Context, name string) (*batchv1.Job, error) {
	return c.clientset.BatchV1().Jobs(c.namespace).Get(ctx, name, metav1.GetOptions{})
}

// ListJobs lists GPU platform jobs (with label selector).
func (c *Client) ListJobs(ctx context.Context) ([]batchv1.Job, error) {
	list, err := c.clientset.BatchV1().Jobs(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "platform=gpu-k8s-infra",
	})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// DeleteJob deletes a job and its pods.
func (c *Client) DeleteJob(ctx context.Context, name string) error {
	return c.clientset.BatchV1().Jobs(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// JobToStatus converts a Job to JobStatus.
func JobToStatus(j *batchv1.Job) JobStatus {
	phase := "Pending"
	if j.Status.Succeeded > 0 {
		phase = "Succeeded"
	} else if j.Status.Failed > 0 {
		phase = "Failed"
	} else if j.Status.Active > 0 {
		phase = "Running"
	}
	startTime := ""
	if j.Status.StartTime != nil {
		startTime = j.Status.StartTime.Format("2006-01-02T15:04:05Z07:00")
	}
	ready := int32(0)
	if j.Status.Ready != nil {
		ready = *j.Status.Ready
	}
	return JobStatus{
		Name:      j.Name,
		Namespace: j.Namespace,
		Phase:     phase,
		Ready:     ready,
		Active:    j.Status.Active,
		Succeeded: j.Status.Succeeded,
		Failed:    j.Status.Failed,
		StartTime: startTime,
	}
}

// ListPodsForJob returns pods belonging to the job.
func (c *Client) ListPodsForJob(ctx context.Context, jobName string) ([]corev1.Pod, error) {
	list, err := c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// GetPodLogs streams logs for a pod (caller can pass limit bytes/lines via options).
func (c *Client) GetPodLogs(ctx context.Context, podName string, opts *corev1.PodLogOptions) ([]byte, error) {
	if opts == nil {
		opts = &corev1.PodLogOptions{}
	}
	req := c.clientset.CoreV1().Pods(c.namespace).GetLogs(podName, opts)
	return req.DoRaw(ctx)
}

func ptr[T any](v T) *T { return &v }

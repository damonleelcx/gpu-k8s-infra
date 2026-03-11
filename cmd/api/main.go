package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/damonleelcx/gpu-k8s-infra/pkg/k8s"
	"github.com/damonleelcx/gpu-k8s-infra/pkg/metrics"
	"github.com/damonleelcx/gpu-k8s-infra/pkg/queue"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	namespace := os.Getenv("GPU_JOBS_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	var k8sClient *k8s.Client
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		var err error
		k8sClient, err = k8s.NewOutOfClusterClient(kubeconfig, namespace)
		if err != nil {
			logger.Fatal("k8s client (out-of-cluster)", zap.Error(err))
		}
	} else {
		var err error
		k8sClient, err = k8s.NewInClusterClient(namespace)
		if err != nil {
			logger.Fatal("k8s client (in-cluster)", zap.Error(err))
		}
	}

	taskStore := queue.NewStore()

	r := mux.NewRouter()

	// Health
	r.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	r.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Metrics
	r.Handle("/metrics", promhttp.Handler())

	// API: submit GPU job
	r.HandleFunc("/api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs", "405").Observe(time.Since(start).Seconds())
			return
		}
		var spec k8s.GPUJobSpec
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs", "400").Observe(time.Since(start).Seconds())
			return
		}
		job, err := k8sClient.CreateGPUJob(r.Context(), spec)
		if err != nil {
			logger.Error("create job", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			metrics.JobsTotal.WithLabelValues("error").Inc()
			metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs", "500").Observe(time.Since(start).Seconds())
			return
		}
		task := &queue.Task{
			ID:      job.Name,
			JobName: job.Name,
			Spec:    spec,
			Status:  "running",
		}
		_ = taskStore.Put(r.Context(), task)
		metrics.JobsTotal.WithLabelValues("submitted").Inc()
		metrics.JobsInFlight.Inc()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jobName": job.Name,
			"taskId": task.ID,
			"status": task.Status,
		})
		metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs", "201").Observe(time.Since(start).Seconds())
	}).Methods("POST")

	// API: list jobs
	r.HandleFunc("/api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		jobs, err := k8sClient.ListJobs(r.Context())
		if err != nil {
			logger.Error("list jobs", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs", "500").Observe(time.Since(start).Seconds())
			return
		}
		statuses := make([]k8s.JobStatus, 0, len(jobs))
		for i := range jobs {
			statuses = append(statuses, k8s.JobToStatus(&jobs[i]))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"jobs": statuses})
		metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs", "200").Observe(time.Since(start).Seconds())
	}).Methods("GET")

	// API: get job
	r.HandleFunc("/api/v1/jobs/{name}", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		name := mux.Vars(r)["name"]
		job, err := k8sClient.GetJob(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs/{name}", "404").Observe(time.Since(start).Seconds())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(k8s.JobToStatus(job))
		metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs/{name}", "200").Observe(time.Since(start).Seconds())
	}).Methods("GET")

	// API: delete job
	r.HandleFunc("/api/v1/jobs/{name}", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		name := mux.Vars(r)["name"]
		if err := k8sClient.DeleteJob(r.Context(), name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs/{name}", "500").Observe(time.Since(start).Seconds())
			return
		}
		w.WriteHeader(http.StatusNoContent)
		metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs/{name}", "204").Observe(time.Since(start).Seconds())
	}).Methods("DELETE")

	// API: get logs for a job (first pod, main container)
	r.HandleFunc("/api/v1/jobs/{name}/logs", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		name := mux.Vars(r)["name"]
		pods, err := k8sClient.ListPodsForJob(r.Context(), name)
		if err != nil || len(pods) == 0 {
			http.Error(w, "no pods for job", http.StatusNotFound)
			metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs/{name}/logs", "404").Observe(time.Since(start).Seconds())
			return
		}
		podName := pods[0].Name
		tailLines := int64(500)
		logs, err := k8sClient.GetPodLogs(r.Context(), podName, &corev1.PodLogOptions{TailLines: &tailLines})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs/{name}/logs", "500").Observe(time.Since(start).Seconds())
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(logs)
		metrics.APIRequestDuration.WithLabelValues(r.Method, "/api/v1/jobs/{name}/logs", "200").Observe(time.Since(start).Seconds())
	}).Methods("GET")

	handler := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	}).Handler(r)

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}
	logger.Info("gpu platform api listening", zap.String("addr", addr))
	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Fatal("server", zap.Error(err))
	}
}

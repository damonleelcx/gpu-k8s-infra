package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/damonleelcx/gpu-k8s-infra/api/v1alpha1"
	"github.com/damonleelcx/gpu-k8s-infra/pkg/autoscaler/coldstart"
	"github.com/damonleelcx/gpu-k8s-infra/pkg/autoscaler/fetcher"
	"github.com/damonleelcx/gpu-k8s-infra/pkg/autoscaler/predictor"
	"github.com/damonleelcx/gpu-k8s-infra/pkg/autoscaler/scaler"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// GPUInferenceAutoscalerReconciler reconciles a GPUInferenceAutoscaler object.
type GPUInferenceAutoscalerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Fetcher  *fetcher.Fetcher
	Scaler   *scaler.Scaler
	lastScale map[types.NamespacedName]time.Time
}

// NewGPUInferenceAutoscalerReconciler builds the reconciler with metrics fetcher and scaler.
func NewGPUInferenceAutoscalerReconciler(c client.Client, scheme *runtime.Scheme, promURL, redisAddr string) *GPUInferenceAutoscalerReconciler {
	f := fetcher.NewFetcher(promURL, redisAddr)
	pred := predictor.NewPredictor("exponential", 0.3)
	sc := &scaler.Scaler{Fetcher: f, Predictor: pred}
	return &GPUInferenceAutoscalerReconciler{
		Client:    c,
		Scheme:    scheme,
		Fetcher:   f,
		Scaler:    sc,
		lastScale: make(map[types.NamespacedName]time.Time),
	}
}

// Reconcile implements controller-runtime Reconciler.
func (r *GPUInferenceAutoscalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var gia v1alpha1.GPUInferenceAutoscaler
	if err := r.Get(ctx, req.NamespacedName, &gia); err != nil {
		if errors.IsNotFound(err) {
			delete(r.lastScale, req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	spec := &gia.Spec
	ref := &spec.ScaleTargetRef
	if ref.Kind == "" {
		ref.Kind = "Deployment"
	}
	if ref.APIVersion == "" {
		ref.APIVersion = "apps/v1"
	}

	var deploy appsv1.Deployment
	key := types.NamespacedName{Namespace: req.Namespace, Name: ref.Name}
	if err := r.Get(ctx, key, &deploy); err != nil {
		if errors.IsNotFound(err) {
			setCondition(&gia, "ScalingActive", metav1.ConditionFalse, "DeploymentNotFound", "target deployment not found")
			_ = r.Status().Update(ctx, &gia)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	currentReplicas := int32(0)
	if deploy.Spec.Replicas != nil {
		currentReplicas = *deploy.Spec.Replicas
	}
	gia.Status.CurrentReplicas = currentReplicas

	desired, metricsUsed, err := r.Scaler.Compute(ctx, &gia)
	if err != nil {
		logger.Error(err, "compute desired replicas")
		setCondition(&gia, "ScalingActive", metav1.ConditionFalse, "ComputeError", err.Error())
		gia.Status.CurrentMetrics = metricsUsed
		_ = r.Status().Update(ctx, &gia)
		syncPeriod := 15
		if spec.SyncPeriodSeconds != nil {
			syncPeriod = int(*spec.SyncPeriodSeconds)
		}
		return ctrl.Result{RequeueAfter: time.Duration(syncPeriod) * time.Second}, nil
	}

	// Stabilization: do not scale down too soon after scale-up
	if lastUp := r.lastScale[req.NamespacedName]; !lastUp.IsZero() && desired < currentReplicas {
		window := coldstart.ScaleUpStabilizationWindow(spec.ColdStart)
		if window > 0 && time.Since(lastUp) < window {
			logger.Info("scale-down delayed by stabilization window", "desired", desired, "current", currentReplicas)
			gia.Status.DesiredReplicas = desired
			gia.Status.CurrentMetrics = metricsUsed
			setCondition(&gia, "ScalingActive", metav1.ConditionTrue, "Stabilizing", "scale-down delayed")
			_ = r.Status().Update(ctx, &gia)
			syncPeriod := 15
			if spec.SyncPeriodSeconds != nil {
				syncPeriod = int(*spec.SyncPeriodSeconds)
			}
			return ctrl.Result{RequeueAfter: time.Duration(syncPeriod) * time.Second}, nil
		}
	}

	gia.Status.DesiredReplicas = desired
	gia.Status.CurrentMetrics = metricsUsed

	if desired != currentReplicas {
		deploy.Spec.Replicas = &desired
		if err := r.Update(ctx, &deploy); err != nil {
			return ctrl.Result{}, err
		}
		now := metav1.NewTime(time.Now())
		gia.Status.LastScaleTime = &now
		if desired > currentReplicas {
			r.lastScale[req.NamespacedName] = time.Now()
		} else {
			delete(r.lastScale, req.NamespacedName)
		}
		setCondition(&gia, "ScalingActive", metav1.ConditionTrue, "Scaling", fmt.Sprintf("replicas %d -> %d", currentReplicas, desired))
		logger.Info("scaled deployment", "replicas", desired, "previous", currentReplicas)
	} else {
		setCondition(&gia, "ScalingActive", metav1.ConditionTrue, "Ready", "replicas at target")
	}

	if err := r.Status().Update(ctx, &gia); err != nil {
		return ctrl.Result{}, err
	}

	syncPeriod := 15
	if spec.SyncPeriodSeconds != nil {
		syncPeriod = int(*spec.SyncPeriodSeconds)
	}
	return ctrl.Result{RequeueAfter: time.Duration(syncPeriod) * time.Second}, nil
}

func setCondition(gia *v1alpha1.GPUInferenceAutoscaler, condType string, status metav1.ConditionStatus, reason, message string) {
	cond := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
	existing := false
	for i := range gia.Status.Conditions {
		if gia.Status.Conditions[i].Type == condType {
			gia.Status.Conditions[i] = cond
			existing = true
			break
		}
	}
	if !existing {
		gia.Status.Conditions = append(gia.Status.Conditions, cond)
	}
}

// SetupWithManager registers the controller with the manager.
func (r *GPUInferenceAutoscalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.GPUInferenceAutoscaler{}).
		Complete(r)
}

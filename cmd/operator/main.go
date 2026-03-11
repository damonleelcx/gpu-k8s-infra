package main

import (
	"flag"
	"os"

	"github.com/damonleelcx/gpu-k8s-infra/api/v1alpha1"
	"github.com/damonleelcx/gpu-k8s-infra/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		prometheusURL        string
		redisAddr            string
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8081", "Metrics server address.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true, "Enable leader election.")
	flag.StringVar(&prometheusURL, "prometheus-url", "", "Prometheus base URL (e.g. http://prometheus:9090) for QPS/GPU metrics.")
	flag.StringVar(&redisAddr, "redis-addr", "", "Redis address (e.g. redis:6379) for queue-length metric.")
	flag.Parse()

	if prometheusURL == "" {
		prometheusURL = os.Getenv("PROMETHEUS_URL")
	}
	if redisAddr == "" {
		redisAddr = os.Getenv("REDIS_ADDR")
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: false})))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "gpu-inference-autoscaler.gpu.k8s.infra",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = v1alpha1.RegisterDefaults(scheme); err != nil {
		setupLog.Error(err, "register defaults")
		os.Exit(1)
	}

	reconciler := controllers.NewGPUInferenceAutoscalerReconciler(mgr.GetClient(), mgr.GetScheme(), prometheusURL, redisAddr)
	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GPUInferenceAutoscaler")
		os.Exit(1)
	}

	setupLog.Info("starting GPU Inference Autoscaler operator", "prometheus", prometheusURL != "", "redis", redisAddr != "")
	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

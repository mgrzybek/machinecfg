/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package main

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"strconv"

	tinkerbellv1 "github.com/tinkerbell/tink/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"machinecfg/internal/controller/netbox_updater"
)

var scheme = k8sruntime.NewScheme()

func init() {
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(tinkerbellv1.AddToScheme(scheme))
}

func main() {
	// Autotune: honour the container CPU limit injected via the Downward API.
	autotuneMaxProcs()

	// Production Zap logger: JSON output to stdout.
	zapOpts := ctrlzap.Options{Development: false}
	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseFlagOptions(&zapOpts)))

	namespace := os.Getenv("CONTROLLER_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	// LEADER_ELECT defaults to true; set to "false" to disable (e.g. in local dev).
	leaderElect := os.Getenv("LEADER_ELECT") != "false"

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: ":8080",
		},
		HealthProbeBindAddress:  ":8081",
		LeaderElection:          leaderElect,
		LeaderElectionID:        "netbox-updater.machinecfg",
		LeaderElectionNamespace: namespace,
	})
	if err != nil {
		slog.Error("cannot create manager", "error", err.Error())
		os.Exit(1)
	}

	cfg := netbox_updater.NewConfig()

	// Bootstrap config before the informer cache is populated.
	if err := cfg.Bootstrap(mgr.GetAPIReader(), context.Background(), namespace); err != nil {
		slog.Warn("initial config bootstrap failed — waiting for ConfigReconciler", "error", err.Error())
	}

	// Initialise OpenTelemetry if requested by the initial config.
	if cfg.GetOtelEnabled() {
		shutdown, otelErr := netbox_updater.InitOtel(context.Background(), cfg.GetOtelEndpoint())
		if otelErr != nil {
			slog.Warn("cannot initialise OpenTelemetry, tracing disabled", "error", otelErr.Error())
		} else if shutdown != nil {
			defer func() {
				if err := shutdown(context.Background()); err != nil {
					slog.Warn("OTEL shutdown error", "error", err.Error())
				}
			}()
			slog.Info("OpenTelemetry tracing enabled", "endpoint", cfg.GetOtelEndpoint())
		}
	}

	// ConfigReconciler: dynamically reloads config on ConfigMap / Secret changes.
	if err := (&netbox_updater.ConfigReconciler{
		Client:    mgr.GetClient(),
		Config:    cfg,
		Namespace: namespace,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("cannot setup config reconciler", "error", err.Error())
		os.Exit(1)
	}

	// Backend-specific reconciler selected from the ConfigMap.
	switch cfg.GetBackend() {
	case "tinkerbell", "":
		if err := (&netbox_updater.HardwareReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Config: cfg,
		}).SetupWithManager(mgr); err != nil {
			slog.Error("cannot setup hardware reconciler", "error", err.Error())
			os.Exit(1)
		}
	case "metal3":
		// BaremetalHost reconciler is planned in Epic 8.
		slog.Info("metal3 backend selected — BaremetalHost reconciler not yet implemented")
	default:
		slog.Warn("unknown backend, defaulting to tinkerbell", "backend", cfg.GetBackend())
	}

	// CAPI Cluster reconciler: always active regardless of backend.
	if err := (&netbox_updater.ClusterReconciler{
		Client: mgr.GetClient(),
		Config: cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("cannot setup cluster reconciler", "error", err.Error())
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		slog.Error("cannot add healthz check", "error", err.Error())
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		slog.Error("cannot add readyz check", "error", err.Error())
		os.Exit(1)
	}

	slog.Info("starting machinecfg-controller-netbox-updater",
		"namespace", namespace,
		"backend", cfg.GetBackend(),
		"leader_elect", leaderElect,
	)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		slog.Error("manager exited with error", "error", err.Error())
		os.Exit(1)
	}
}

// autotuneMaxProcs sets GOMAXPROCS from the LIMITS_CPU environment variable
// injected by the Kubernetes Downward API. This ensures the Go scheduler uses
// at most the number of CPU cores allocated to the container.
func autotuneMaxProcs() {
	v := os.Getenv("LIMITS_CPU")
	if v == "" {
		return
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return
	}
	runtime.GOMAXPROCS(n)
	slog.Info("GOMAXPROCS tuned from container CPU limit", "value", n)
}

/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package netbox_updater

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/netbox-community/go-netbox/v4"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// ConfigMapName is the name of the ConfigMap holding the controller configuration.
	ConfigMapName = "netbox-updater-config"
	// SecretName is the name of the Secret holding the NetBox token.
	SecretName = "netbox-updater-secret"

	defaultNetBoxEndpoint = "http://netbox.svc"
	defaultBackend        = "tinkerbell"
)

// Config holds the dynamic runtime configuration shared across all reconcilers.
// Fields are protected by a RWMutex for concurrent access.
type Config struct {
	mu             sync.RWMutex
	netboxEndpoint string
	netboxToken    string
	backend        string
	otelEnabled    bool
	otelEndpoint   string
	netboxClient   *netbox.APIClient
}

// NewConfig returns a Config initialised with sensible defaults.
func NewConfig() *Config {
	return &Config{
		netboxEndpoint: defaultNetBoxEndpoint,
		backend:        defaultBackend,
	}
}

// GetNetBoxClient returns the current NetBox API client (nil if not yet configured).
func (c *Config) GetNetBoxClient() *netbox.APIClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.netboxClient
}

// GetBackend returns the configured backend type ("tinkerbell" or "metal3").
func (c *Config) GetBackend() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.backend == "" {
		return defaultBackend
	}
	return c.backend
}

// GetOtelEnabled reports whether OpenTelemetry tracing is enabled.
func (c *Config) GetOtelEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.otelEnabled
}

// GetOtelEndpoint returns the configured OpenTelemetry collector endpoint.
func (c *Config) GetOtelEndpoint() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.otelEndpoint
}

// update rebuilds the NetBox client and atomically replaces all config fields.
func (c *Config) update(endpoint, token, backend string, otelEnabled bool, otelEndpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.netboxEndpoint = endpoint
	c.netboxToken = token
	c.backend = backend
	c.otelEnabled = otelEnabled
	c.otelEndpoint = otelEndpoint
	if endpoint != "" && token != "" {
		c.netboxClient = netbox.NewAPIClientFor(endpoint, token)
		slog.Info("NetBox client reconfigured", "endpoint", endpoint)
	}
}

// applyConfigMapAndSecret extracts values from cm and secret and applies them via update.
func (c *Config) applyConfigMapAndSecret(cm *corev1.ConfigMap, secret *corev1.Secret) {
	endpoint := cm.Data["netbox_endpoint"]
	if endpoint == "" {
		endpoint = defaultNetBoxEndpoint
	}
	backend := cm.Data["backend"]
	if backend == "" {
		backend = defaultBackend
	}
	otelEnabled := cm.Data["otel_enabled"] == "true"
	otelEndpoint := cm.Data["otel_endpoint"]
	token := string(secret.Data["netbox_token"])
	c.update(endpoint, token, backend, otelEnabled, otelEndpoint)
}

// Bootstrap performs an initial read of the ConfigMap and Secret using the
// direct API reader (mgr.GetAPIReader()) before the informer cache is populated.
func (c *Config) Bootstrap(reader client.Reader, ctx context.Context, namespace string) error {
	cm := &corev1.ConfigMap{}
	if err := reader.Get(ctx, types.NamespacedName{Name: ConfigMapName, Namespace: namespace}, cm); err != nil {
		return fmt.Errorf("cannot read ConfigMap %s/%s: %w", namespace, ConfigMapName, err)
	}
	secret := &corev1.Secret{}
	if err := reader.Get(ctx, types.NamespacedName{Name: SecretName, Namespace: namespace}, secret); err != nil {
		return fmt.Errorf("cannot read Secret %s/%s: %w", namespace, SecretName, err)
	}
	c.applyConfigMapAndSecret(cm, secret)
	return nil
}

// ConfigReconciler watches the controller ConfigMap and Secret and dynamically
// updates the shared Config whenever either changes.
type ConfigReconciler struct {
	client.Client
	Config    *Config
	Namespace string
}

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile reads the ConfigMap and Secret then updates the shared Config.
func (r *ConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: ConfigMapName, Namespace: r.Namespace}, cm); err != nil {
		slog.Warn("cannot read ConfigMap", "name", ConfigMapName, "namespace", r.Namespace, "error", err.Error())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: SecretName, Namespace: r.Namespace}, secret); err != nil {
		slog.Warn("cannot read Secret", "name", SecretName, "namespace", r.Namespace, "error", err.Error())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	r.Config.applyConfigMapAndSecret(cm, secret)
	slog.Info("configuration reloaded", "backend", r.Config.GetBackend(), "otel", r.Config.GetOtelEnabled())
	return ctrl.Result{}, nil
}

// SetupWithManager registers the ConfigReconciler. It watches the ConfigMap
// (primary trigger) and the Secret (mapped to the ConfigMap key on change).
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	cmPredicate := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return o.GetName() == ConfigMapName && o.GetNamespace() == r.Namespace
	})
	secretPredicate := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return o.GetName() == SecretName && o.GetNamespace() == r.Namespace
	})
	// When the Secret changes, enqueue a reconcile for the ConfigMap key so that
	// Reconcile always reads both sources together.
	secretHandler := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, o client.Object) []reconcile.Request {
		if o.GetName() == SecretName && o.GetNamespace() == r.Namespace {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{Name: ConfigMapName, Namespace: r.Namespace},
			}}
		}
		return nil
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}, builder.WithPredicates(cmPredicate)).
		Watches(&corev1.Secret{}, secretHandler, builder.WithPredicates(secretPredicate)).
		Named("config").
		Complete(r)
}

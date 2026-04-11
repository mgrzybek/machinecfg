/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package netbox_updater

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const otelServiceName = "machinecfg-controller-netbox-updater"

// InitOtel initialises an OTLP gRPC trace provider and sets it as the global
// OpenTelemetry provider. Returns a shutdown function to flush and stop the
// exporter on controller exit.
//
// If endpoint is empty, no provider is configured and (nil, nil) is returned.
func InitOtel(ctx context.Context, endpoint string) (shutdown func(context.Context) error, err error) {
	if endpoint == "" {
		return nil, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot create OTEL trace exporter for %s: %w", endpoint, err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(otelServiceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot create OTEL resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

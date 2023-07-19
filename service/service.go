// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package service // import "go.opentelemetry.io/collector/service"

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/internal/obsreportconfig"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	semconv "go.opentelemetry.io/collector/semconv/v1.18.0"
	"go.opentelemetry.io/collector/service/extensions"
	"go.opentelemetry.io/collector/service/internal/graph"
	"go.opentelemetry.io/collector/service/internal/proctelemetry"
	"go.opentelemetry.io/collector/service/telemetry"
)

// Settings holds configuration for building a new service.
type Settings struct {
	// BuildInfo provides collector start information.
	BuildInfo component.BuildInfo

	ConfigURIs []string

	Factories Factories

	// AsyncErrorChannel is the channel that is used to report fatal errors.
	AsyncErrorChannel chan error

	// LoggingOptions provides a way to change behavior of zap logging.
	LoggingOptions []zap.Option

	// For testing purpose only.
	useOtel *bool
}

// Service represents the implementation of a component.Host.
type Service struct {
	buildInfo            component.BuildInfo
	telemetry            *telemetry.Telemetry
	telemetrySettings    component.TelemetrySettings
	host                 *serviceHost
	telemetryInitializer *telemetryInitializer
	configProvider       ConfigProvider
	set                  Settings
}

func New(set Settings) (*Service, error) {
	if len(set.ConfigURIs) == 0 {
		return nil, errors.New("must provide at least one config URI")
	}
	// TODO: It seems like we should be able to provide custom providers and converters
	cp, err := NewConfigProvider(NewDefaultConfigProviderSettings(set.ConfigURIs))

	if err != nil {
		return nil, err
	}

	disableHighCard := obsreportconfig.DisableHighCardinalityMetricsfeatureGate.IsEnabled()
	extendedConfig := obsreportconfig.UseOtelWithSDKConfigurationForInternalTelemetryFeatureGate.IsEnabled()

	useOtel := obsreportconfig.UseOtelForInternalMetricsfeatureGate.IsEnabled()
	if set.useOtel != nil {
		useOtel = *set.useOtel
	}

	srv := &Service{
		buildInfo: set.BuildInfo,
		host: &serviceHost{
			buildInfo:         set.BuildInfo,
			asyncErrorChannel: set.AsyncErrorChannel,
		},
		telemetryInitializer: newColTelemetry(useOtel, disableHighCard, extendedConfig),
		configProvider:       cp,
	}

	return srv, nil
}

func (srv *Service) init(ctx context.Context) error {
	cfg, err := srv.configProvider.Get(ctx, srv.set.Factories)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	if err = cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	srv.host.receivers = receiver.NewBuilder(cfg.Receivers, srv.set.Factories.Receivers)
	srv.host.processors = processor.NewBuilder(cfg.Processors, srv.set.Factories.Processors)
	srv.host.exporters = exporter.NewBuilder(cfg.Exporters, srv.set.Factories.Exporters)
	srv.host.connectors = connector.NewBuilder(cfg.Connectors, srv.set.Factories.Connectors)
	srv.host.extensions = extension.NewBuilder(cfg.Extensions, srv.set.Factories.Extensions)
	srv.telemetry, err = telemetry.New(ctx, telemetry.Settings{ZapOptions: srv.set.LoggingOptions}, cfg.Service.Telemetry)
	if err != nil {
		return fmt.Errorf("failed to get logger: %w", err)
	}
	res := buildResource(srv.set.BuildInfo, cfg.Service.Telemetry)
	pcommonRes := pdataFromSdk(res)

	srv.telemetrySettings = component.TelemetrySettings{
		Logger:         srv.telemetry.Logger(),
		TracerProvider: srv.telemetry.TracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		MetricsLevel:   cfg.Service.Telemetry.Metrics.Level,

		// Construct telemetry attributes from build info and config's resource attributes.
		Resource: pcommonRes,
	}

	if err = srv.telemetryInitializer.init(res, srv.telemetrySettings, cfg.Service.Telemetry, srv.set.AsyncErrorChannel); err != nil {
		return fmt.Errorf("failed to initialize telemetry: %w", err)
	}
	srv.telemetrySettings.MeterProvider = srv.telemetryInitializer.mp

	// process the configuration and initialize the pipeline
	if err = srv.initExtensionsAndPipeline(ctx, srv.set, cfg); err != nil {
		// If pipeline initialization fails then shut down the telemetry server
		if shutdownErr := srv.telemetryInitializer.shutdown(); shutdownErr != nil {
			err = multierr.Append(err, fmt.Errorf("failed to shutdown collector telemetry: %w", shutdownErr))
		}

		return err
	}

	return nil
}

// Start starts the extensions and pipelines. If Start fails Shutdown should be called to ensure a clean state.
func (srv *Service) Start(ctx context.Context) error {
	srv.init(ctx)
	srv.telemetrySettings.Logger.Info("Starting "+srv.buildInfo.Command+"...",
		zap.String("Version", srv.buildInfo.Version),
		zap.Int("NumCPU", runtime.NumCPU()),
	)

	if err := srv.host.serviceExtensions.Start(ctx, srv.host); err != nil {
		return fmt.Errorf("failed to start extensions: %w", err)
	}

	if conf, err := srv.configProvider.(*configProvider).GetConfmap(ctx); err != nil {
		srv.host.serviceExtensions.NotifyConfig(ctx, conf)
	}

	if err := srv.host.pipelines.StartAll(ctx, srv.host); err != nil {
		return fmt.Errorf("cannot start pipelines: %w", err)
	}

	if err := srv.host.serviceExtensions.NotifyPipelineReady(); err != nil {
		return err
	}

	srv.telemetrySettings.Logger.Info("Everything is ready. Begin running and processing data.")

	go srv.watchConfig(ctx)

	return nil
}

func (srv *Service) Stop(ctx context.Context) error {
	// Accumulate errors and proceed with shutting down remaining components.
	var errs error

	// Begin shutdown sequence.
	srv.telemetrySettings.Logger.Info("Starting shutdown...")

	if err := srv.host.serviceExtensions.NotifyPipelineNotReady(); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to notify that pipeline is not ready: %w", err))
	}

	if err := srv.host.pipelines.ShutdownAll(ctx); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to shutdown pipelines: %w", err))
	}

	if err := srv.host.serviceExtensions.Shutdown(ctx); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to shutdown extensions: %w", err))
	}

	srv.telemetrySettings.Logger.Info("Shutdown complete.")

	if err := srv.telemetry.Shutdown(ctx); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to shutdown telemetry: %w", err))
	}

	if err := srv.telemetryInitializer.shutdown(); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to shutdown collector telemetry: %w", err))
	}
	return errs
}

func (srv *Service) Shutdown(ctx context.Context) error {

	// if err := col.set.ConfigProvider.Shutdown(ctx); err != nil {
	// 	errs = multierr.Append(errs, fmt.Errorf("failed to shutdown config provider: %w", err))
	// }
}

func (srv *Service) watchConfig(ctx context.Context) {
LOOP:
	for {
		select {
		case err := <-srv.configProvider.Watch():
			if err != nil {
				srv.Logger().Error("Config watch failed", zap.Error(err))
				break LOOP
			}
			err = srv.Shutdown(ctx)
			if err != nil {
				srv.Logger().Error("Shutdown failed", zap.Error(err))
				break LOOP
			}
			err = srv.Start(ctx)
			if err != nil {
				srv.Logger().Error("Start failed", zap.Error(err))
				break LOOP
			}
		case <-ctx.Done():
			break LOOP
		}
	}
}

func (srv *Service) initExtensionsAndPipeline(ctx context.Context, set Settings, cfg *Config) error {
	var err error
	extensionsSettings := extensions.Settings{
		Telemetry:  srv.telemetrySettings,
		BuildInfo:  srv.buildInfo,
		Extensions: srv.host.extensions,
	}
	if srv.host.serviceExtensions, err = extensions.New(ctx, extensionsSettings, cfg.Service.Extensions); err != nil {
		return fmt.Errorf("failed to build extensions: %w", err)
	}

	pSet := graph.Settings{
		Telemetry:        srv.telemetrySettings,
		BuildInfo:        srv.buildInfo,
		ReceiverBuilder:  srv.host.receivers,
		ProcessorBuilder: srv.host.processors,
		ExporterBuilder:  srv.host.exporters,
		ConnectorBuilder: srv.host.connectors,
		PipelineConfigs:  cfg.Service.Pipelines,
	}

	if srv.host.pipelines, err = graph.Build(ctx, pSet); err != nil {
		return fmt.Errorf("failed to build pipelines: %w", err)
	}

	if cfg.Service.Telemetry.Metrics.Level != configtelemetry.LevelNone && cfg.Service.Telemetry.Metrics.Address != "" {
		// The process telemetry initialization requires the ballast size, which is available after the extensions are initialized.
		if err = proctelemetry.RegisterProcessMetrics(srv.telemetryInitializer.ocRegistry, srv.telemetryInitializer.mp, obsreportconfig.UseOtelForInternalMetricsfeatureGate.IsEnabled(), getBallastSize(srv.host)); err != nil {
			return fmt.Errorf("failed to register process metrics: %w", err)
		}
	}

	return nil
}

// Logger returns the logger created for this service.
// This is a temporary API that may be removed soon after investigating how the collector should record different events.
func (srv *Service) Logger() *zap.Logger {
	return srv.telemetrySettings.Logger
}

func getBallastSize(host component.Host) uint64 {
	for _, ext := range host.GetExtensions() {
		if bExt, ok := ext.(interface{ GetBallastSize() uint64 }); ok {
			return bExt.GetBallastSize()
		}
	}
	return 0
}

func buildResource(buildInfo component.BuildInfo, cfg telemetry.Config) *resource.Resource {
	var telAttrs []attribute.KeyValue

	for k, v := range cfg.Resource {
		// nil value indicates that the attribute should not be included in the telemetry.
		if v != nil {
			telAttrs = append(telAttrs, attribute.String(k, *v))
		}
	}

	if _, ok := cfg.Resource[semconv.AttributeServiceName]; !ok {
		// AttributeServiceName is not specified in the config. Use the default service name.
		telAttrs = append(telAttrs, attribute.String(semconv.AttributeServiceName, buildInfo.Command))
	}

	if _, ok := cfg.Resource[semconv.AttributeServiceInstanceID]; !ok {
		// AttributeServiceInstanceID is not specified in the config. Auto-generate one.
		instanceUUID, _ := uuid.NewRandom()
		instanceID := instanceUUID.String()
		telAttrs = append(telAttrs, attribute.String(semconv.AttributeServiceInstanceID, instanceID))
	}

	if _, ok := cfg.Resource[semconv.AttributeServiceVersion]; !ok {
		// AttributeServiceVersion is not specified in the config. Use the actual
		// build version.
		telAttrs = append(telAttrs, attribute.String(semconv.AttributeServiceVersion, buildInfo.Version))
	}
	return resource.NewWithAttributes(semconv.SchemaURL, telAttrs...)
}

func pdataFromSdk(res *resource.Resource) pcommon.Resource {
	// pcommon.NewResource is the best way to generate a new resource currently and is safe to use outside of tests.
	// Because the resource is signal agnostic, and we need a net new resource, not an existing one, this is the only
	// method of creating it without exposing internal packages.
	pcommonRes := pcommon.NewResource()
	for _, keyValue := range res.Attributes() {
		pcommonRes.Attributes().PutStr(string(keyValue.Key), keyValue.Value.AsString())
	}
	return pcommonRes
}

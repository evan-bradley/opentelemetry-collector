// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package otelcol // import "go.opentelemetry.io/collector/otelcol"

import (
	"time"

	"go.uber.org/zap/zapcore"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol/internal/configunmarshaler"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/service"
	"go.opentelemetry.io/collector/service/telemetry"
)

type configSettings struct {
	Receivers  *configunmarshaler.Configs[receiver.Factory]  `mapstructure:"receivers"`
	Processors *configunmarshaler.Configs[processor.Factory] `mapstructure:"processors"`
	Exporters  *configunmarshaler.Configs[exporter.Factory]  `mapstructure:"exporters"`
	Connectors *configunmarshaler.Configs[connector.Factory] `mapstructure:"connectors"`
	Extensions *configunmarshaler.Configs[extension.Factory] `mapstructure:"extensions"`
	Service    service.Config                                `mapstructure:"service"`
}

// unmarshal the configSettings from a confmap.Conf.
// After the config is unmarshalled, `Validate()` must be called to validate.
func unmarshal(v *confmap.Conf, factories Factories, processors []ConfigProcessor) (*configSettings, error) {
	// Unmarshal top level sections and validate.
	cfg := &configSettings{
		Receivers:  configunmarshaler.NewConfigs(factories.Receivers),
		Processors: configunmarshaler.NewConfigs(factories.Processors),
		Exporters:  configunmarshaler.NewConfigs(factories.Exporters),
		Connectors: configunmarshaler.NewConfigs(factories.Connectors),
		Extensions: configunmarshaler.NewConfigs(factories.Extensions),
		// TODO: Add a component.ServiceFactory to allow this to be defined by the Service.
		Service: service.Config{
			Telemetry: telemetry.Config{
				Logs: telemetry.LogsConfig{
					Level:       zapcore.InfoLevel,
					Development: false,
					Encoding:    "console",
					Sampling: &telemetry.LogsSamplingConfig{
						Enabled:    true,
						Tick:       10 * time.Second,
						Initial:    10,
						Thereafter: 100,
					},
					OutputPaths:       []string{"stderr"},
					ErrorOutputPaths:  []string{"stderr"},
					DisableCaller:     false,
					DisableStacktrace: false,
					InitialFields:     map[string]any(nil),
				},
				Metrics: telemetry.MetricsConfig{
					Level:   configtelemetry.LevelBasic,
					Address: ":8888",
				},
			},
		},
	}

	// Unmarshal the config into a partial form.
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// TODO Loop over processors and apply them to configSettings
	for _, p := range processors {
		inc := cfg.Processors.Incomplete()
		for _, incVal := range inc {
			key, procConf := p.ProcessorConfig()
			sub, _ := v.Sub(key)
			_ = sub.Unmarshal(&procConf)
			p.Process(&PartialConfig{
				Receivers:  cfg.Receivers.Configs(),
				Processors: cfg.Processors.Configs(),
				Exporters:  cfg.Exporters.Configs(),
				Connectors: cfg.Connectors.Configs(),
				Extensions: cfg.Extensions.Configs(),
				Service:    cfg.Service,
			}, factories, procConf)
		}
	}

	return cfg, nil
}

type ConfigProcessor interface {
	// Prefix for configs like "template/"
	Prefix() string

	// Config type for component configs that need
	// processing like `template/my_receiver`
	ComponentConfig() any

	// Key and type for the processor's config (e.g. "templates" and `templatesConfig`)
	ProcessorConfig() (string, any)

	// Handles configs starting with the processor's prefix
	// given the available factories and its config.
	// All instances of the ComponentConfig type should be
	// replaced by component.Config when this function returns.
	Process(pc *PartialConfig, factories Factories, config any) error
}

type PartialConfig struct {
	Receivers  map[component.ID]component.Config // Factory or types returned by ConfigProcessor.ComponentConfig()
	Processors map[component.ID]component.Config // Factory or types returned by ConfigProcessor.ComponentConfig()
	Exporters  map[component.ID]component.Config // Factory or types returned by ConfigProcessor.ComponentConfig()
	Extensions map[component.ID]component.Config // Factory or types returned by ConfigProcessor.ComponentConfig()
	Connectors map[component.ID]component.Config // Factory or types returned by ConfigProcessor.ComponentConfig()
	Service    service.Config                    // Some type that enables accessing/creating pipelines for the case of templating
}

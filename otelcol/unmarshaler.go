// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package otelcol // import "go.opentelemetry.io/collector/otelcol"

import (
	"fmt"
	"text/template"
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

type ConfigSettings struct {
	Receivers  *configunmarshaler.Configs[receiver.Factory]  `mapstructure:"receivers"`
	Processors *configunmarshaler.Configs[processor.Factory] `mapstructure:"processors"`
	Exporters  *configunmarshaler.Configs[exporter.Factory]  `mapstructure:"exporters"`
	Connectors *configunmarshaler.Configs[connector.Factory] `mapstructure:"connectors"`
	Extensions *configunmarshaler.Configs[extension.Factory] `mapstructure:"extensions"`
	Service    service.Config                                `mapstructure:"service"`
}

// unmarshal the configSettings from a confmap.Conf.
// After the config is unmarshalled, `Validate()` must be called to validate.
func Unmarshal(v *confmap.Conf, factories Factories, converters []ConfigConverter) (*Config, error) {
	// Unmarshal top level sections and validate.
	cfg := &ConfigSettings{
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

	converterConfigs := map[string]any{}
	vMap := v.ToStringMap()

	for _, c := range converters {
		name, conf := c.ConverterConfig()
		if v.IsSet(name) {
			sub, err := v.Sub(name)
			if err != nil {
				return nil, err
			}
			var str string
			sub.Marshal(str)
			fmt.Println(str)
			// TODO fix this
			// if name == "templates" {
			// 	converterConfigs[name], _ = parseTemplates
			// }
			if err := sub.Unmarshal(&conf); err != nil {
				fmt.Println("hi")
				return nil, err
			}
			converterConfigs[name] = conf

		}
		delete(vMap, name)
	}

	// Unmarshal the config into a partial form.
	if err := confmap.NewFromStringMap(vMap).Unmarshal(&cfg); err != nil {
		return nil, err
	}

	config := &Config{
		Receivers:  cfg.Receivers.Configs(),
		Processors: cfg.Processors.Configs(),
		Exporters:  cfg.Exporters.Configs(),
		Connectors: cfg.Connectors.Configs(),
		Extensions: cfg.Extensions.Configs(),
		Service:    cfg.Service,
	}

	// TODO Loop over processors and apply them to configSettings
	for _, c := range converters {
		name, _ := c.ConverterConfig()
		if err := c.Convert(config, factories, converterConfigs[name]); err != nil {
			return config, err
		}

	}

	return config, nil
}

type ConfigConverter interface {
	// Prefix for configs like "template/"
	// Prefix() string

	// Config type for component configs that need
	// processing like `template/my_receiver`
	// ComponentConfig() any

	// Key and type for the processor's config (e.g. "templates" and `templatesConfig`)
	ConverterConfig() (string, any)

	// Handles configs starting with the processor's prefix
	// given the available factories and its config.
	// All instances of the ComponentConfig type should be
	// replaced by component.Config when this function returns.
	Convert(c *Config, factories Factories, config any) error
}

type PartialConfig struct {
	Receivers  map[component.ID]component.Config // Factory or types returned by ConfigProcessor.ComponentConfig()
	Processors map[component.ID]component.Config // Factory or types returned by ConfigProcessor.ComponentConfig()
	Exporters  map[component.ID]component.Config // Factory or types returned by ConfigProcessor.ComponentConfig()
	Extensions map[component.ID]component.Config // Factory or types returned by ConfigProcessor.ComponentConfig()
	Connectors map[component.ID]component.Config // Factory or types returned by ConfigProcessor.ComponentConfig()
	Service    service.Config                    // Some type that enables accessing/creating pipelines for the case of templating
}

func parseTemplates(conf *confmap.Conf) (map[string]any, error) {
	if !conf.IsSet("templates") {
		return nil, nil
	}

	templatesMap, ok := conf.ToStringMap()["templates"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("'templates' must be a map")
	}
	if templatesMap["receivers"] == nil {
		return nil, fmt.Errorf("'templates' must contain a 'receivers' section")
	}

	receiverTemplates, ok := templatesMap["receivers"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("'templates::receivers' must be a map")
	}

	newTempl := map[string]*template.Template{}
	for templateType, templateVal := range receiverTemplates {
		templateStr, ok := templateVal.(string)
		if !ok {
			return nil, fmt.Errorf("'templates::receivers::%s' must be a string", templateType)
		}
		parsedTemplate, err := template.New(templateType).Parse(templateStr)
		if err != nil {
			return nil, err
		}
		newTempl[templateType] = parsedTemplate
	}
	return receiverTemplates, nil
}

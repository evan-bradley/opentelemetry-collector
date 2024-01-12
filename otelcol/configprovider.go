// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package otelcol // import "go.opentelemetry.io/collector/otelcol"

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"gopkg.in/yaml.v3"
)

// ConfigProvider provides the service configuration.
//
// The typical usage is the following:
//
//	cfgProvider.Get(...)
//	cfgProvider.Watch() // wait for an event.
//	cfgProvider.Get(...)
//	cfgProvider.Watch() // wait for an event.
//	// repeat Get/Watch cycle until it is time to shut down the Collector process.
//	cfgProvider.Shutdown()
type ConfigProvider interface {
	// Get returns the service configuration, or error otherwise.
	//
	// Should never be called concurrently with itself, Watch or Shutdown.
	Get(ctx context.Context, factories Factories) (*Config, error)

	// Watch blocks until any configuration change was detected or an unrecoverable error
	// happened during monitoring the configuration changes.
	//
	// Error is nil if the configuration is changed and needs to be re-fetched. Any non-nil
	// error indicates that there was a problem with watching the config changes.
	//
	// Should never be called concurrently with itself or Get.
	Watch() <-chan error

	// Shutdown signals that the provider is no longer in use and the that should close
	// and release any resources that it may have created.
	//
	// This function must terminate the Watch channel.
	//
	// Should never be called concurrently with itself or Get.
	Shutdown(ctx context.Context) error
}

// ConfmapProvider is an optional interface to be implemented by ConfigProviders
// to provide confmap.Conf objects representing a marshaled version of the
// Collector's configuration.
//
// The purpose of this interface is that otelcol.ConfigProvider structs do not
// necessarily need to use confmap.Conf as their underlying config structure.
type ConfmapProvider interface {
	// GetConfmap resolves the Collector's configuration and provides it as a confmap.Conf object.
	//
	// Should never be called concurrently with itself or any ConfigProvider method.
	GetConfmap(ctx context.Context) (*confmap.Conf, error)
}

type configProvider struct {
	mapResolver *confmap.Resolver

	converters []ConfigConverter
}

var _ ConfigProvider = &configProvider{}
var _ ConfmapProvider = &configProvider{}

// ConfigProviderSettings are the settings to configure the behavior of the ConfigProvider.
type ConfigProviderSettings struct {
	// ResolverSettings are the settings to configure the behavior of the confmap.Resolver.
	ResolverSettings confmap.ResolverSettings

	converters []ConfigConverter
}

// NewConfigProvider returns a new ConfigProvider that provides the service configuration:
// * Initially it resolves the "configuration map":
//   - Retrieve the confmap.Conf by merging all retrieved maps from the given `locations` in order.
//   - Then applies all the confmap.Converter in the given order.
//
// * Then unmarshalls the confmap.Conf into the service Config.
func NewConfigProvider(set ConfigProviderSettings) (ConfigProvider, error) {
	mr, err := confmap.NewResolver(set.ResolverSettings)
	if err != nil {
		return nil, err
	}

	return &configProvider{
		mapResolver: mr,
		converters:  set.converters,
	}, nil
}

func (cm *configProvider) Get(ctx context.Context, factories Factories) (*Config, error) {
	conf, err := cm.mapResolver.Resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve the configuration: %w", err)
	}

	var cfg *Config
	if cfg, err = Unmarshal(conf, factories, cm.converters); err != nil {
		return nil, fmt.Errorf("cannot unmarshal the configuration: %w", err)
	}

	out, _ := yaml.Marshal(cfg)
	fmt.Println(string(out))

	return cfg, nil
}

func (cm *configProvider) Watch() <-chan error {
	return cm.mapResolver.Watch()
}

func (cm *configProvider) Shutdown(ctx context.Context) error {
	return cm.mapResolver.Shutdown(ctx)
}

func (cm *configProvider) GetConfmap(ctx context.Context) (*confmap.Conf, error) {
	conf, err := cm.mapResolver.Resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve the configuration: %w", err)
	}

	return conf, nil
}

func newDefaultConfigProviderSettings(uris []string) ConfigProviderSettings {
	return ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs:       uris,
			Providers:  makeMapProvidersMap(fileprovider.New(), envprovider.New(), yamlprovider.New(), httpprovider.New(), httpsprovider.New()),
			Converters: []confmap.Converter{expandconverter.New()},
		},
	}
}

func makeMapProvidersMap(providers ...confmap.Provider) map[string]confmap.Provider {
	ret := make(map[string]confmap.Provider, len(providers))
	for _, provider := range providers {
		ret[provider.Scheme()] = provider
	}
	return ret
}

// type templateProcessor struct{}

// type templateConfig struct {
// 	Receivers  map[component.ID]any       `yaml:"receivers,omitempty"`
// 	Processors map[component.ID]any       `yaml:"processors,omitempty"`
// 	Pipelines  map[string]partialPipeline `yaml:"pipelines,omitempty"`
// }

// type partialPipeline struct {
// 	Receivers  []component.ID `yaml:"receivers,omitempty"`
// 	Processors []component.ID `yaml:"processors,omitempty"`
// }

// func (t *templateProcessor) Prefix() string {
// 	return "template"
// }

// // TODO Is this necessary?
// func (t *templateProcessor) ComponentConfig() any {
// 	return map[string]any{}
// }

// func (t *templateProcessor) ProcessorConfig() any {
// 	return struct{}{}
// }

// func (t *templateProcessor) Process(pc *PartialConfig, factories Factories, config any) error {
// 	tc, ok := config.(templateConfig)

// 	if !ok {
// 		return errors.New("bad config")
// 	}

// 	if factories.Connectors["forward"] == nil {
// 		return errors.New("forward connector is required for the template processor")
// 	}

// 	for i, c := range pc.Receivers {
// 		// Take out the template/ name
// 		conf := c.(map[string]any)

// 		f := factories.Receivers[i.Type()]
// 		cfg := f.CreateDefaultConfig()

// 		// Put in something else
// 		if err := component.UnmarshalConfig(confmap.NewFromStringMap(conf), cfg); err != nil {
// 			return errors.New("ayyo")
// 		}
// 	}

// 	return nil
// }

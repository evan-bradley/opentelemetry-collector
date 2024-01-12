// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package configunmarshaler // import "go.opentelemetry.io/collector/otelcol/internal/configunmarshaler"

import (
	"fmt"
	"reflect"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
)

type Configs[F component.Factory] struct {
	cfgs map[component.ID]component.Config
	// Map of processor -> component `type/name` -> config
	incomplete map[string]map[component.ID]map[string]any

	factories map[component.Type]F
	// processorPrefixes []string
}

func NewConfigs[F component.Factory](factories map[component.Type]F) *Configs[F] {
	return &Configs[F]{factories: factories}
}

func (c *Configs[F]) Unmarshal(conf *confmap.Conf) error {
	rawCfgs := make(map[component.ID]map[string]any)
	if err := conf.Unmarshal(&rawCfgs); err != nil {
		return err
	}

	// Prepare resulting map.
	c.cfgs = make(map[component.ID]component.Config)
	// Iterate over raw configs and create a config for each.
	for id, value := range rawCfgs {
		// Find factory based on component kind and type that we read from config source.
		factory, ok := c.factories[id.Type()]
		if !ok {
			// The component isn't known to us right now, but may get expanded later.
			// for _, pp := range c.processorPrefixes {
			// 	if strings.HasPrefix(id.String(), pp) {
			// 		if c.incomplete[pp] == nil {
			// 			c.incomplete[pp] = map[component.ID]map[string]any{}
			// 		}

			// 		c.incomplete[pp][id] = value
			// 	}
			// }
			c.cfgs[id] = value
		} else {
			// Create the default config for this component.
			cfg := factory.CreateDefaultConfig()

			// Now that the default config struct is created we can Unmarshal into it,
			// and it will apply user-defined config on top of the default.
			if err := component.UnmarshalConfig(confmap.NewFromStringMap(value), cfg); err != nil {
				return errorUnmarshalError(id, err)
			}

			c.cfgs[id] = cfg
		}
	}

	return nil
}

func (c *Configs[F]) Configs() map[component.ID]component.Config {
	return c.cfgs
}

func (c *Configs[F]) Incomplete() map[string]map[component.ID]map[string]any {
	return c.incomplete
}

func errorUnknownType(id component.ID, factories []reflect.Value) error {
	return fmt.Errorf("unknown type: %q for id: %q (valid values: %v)", id.Type(), id, factories)
}

func errorUnmarshalError(id component.ID, err error) error {
	return fmt.Errorf("error reading configuration for %q: %w", id, err)
}

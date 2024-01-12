// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package templateconverter // import "go.opentelemetry.io/collector/confmap/converter/templateconverter"

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"
	"gopkg.in/yaml.v3"
)

type instanceID struct {
	tmplType string
	instName string
}

func newInstanceID(id string) (*instanceID, error) {
	parts := strings.SplitN(id, "/", 3)
	switch len(parts) {
	case 2: // template/type
		return &instanceID{
			tmplType: parts[1],
		}, nil
	case 3: // template/type/name
		return &instanceID{
			tmplType: parts[1],
			instName: parts[2],
		}, nil
	}
	return nil, fmt.Errorf("'template' must be followed by type")
}

func (id *instanceID) withPrefix(prefix string) component.ID {
	if id.instName == "" {
		return component.NewIDWithName(component.Type(prefix), id.tmplType)
	}
	return component.NewIDWithName(component.Type(prefix), id.tmplType+"/"+id.instName)
}

type templateConfig struct {
	Receivers  map[component.ID]component.Config `yaml:"receivers,omitempty"`
	Processors map[component.ID]component.Config `yaml:"processors,omitempty"`
	Pipelines  map[component.ID]partialPipeline  `yaml:"pipelines,omitempty"`
}

type partialPipeline struct {
	Receivers  []component.ID `yaml:"receivers,omitempty"`
	Processors []component.ID `yaml:"processors,omitempty"`
}

func newTemplateConfig(id *instanceID, tmpl *template.Template, parameters any, factories otelcol.Factories) (*templateConfig, error) {
	var rendered bytes.Buffer
	var err error
	if err = tmpl.Execute(&rendered, parameters); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}

	cfg := new(templateConfig)

	m := map[string]any{}
	if err = yaml.Unmarshal(rendered.Bytes(), m); err != nil {
		return nil, fmt.Errorf("malformed: %w", err)
	}

	conf, err := otelcol.Unmarshal(confmap.NewFromStringMap(m), factories, []otelcol.ConfigConverter{})
	if err != nil {
		return nil, err
	}
	cfg.Receivers = conf.Receivers
	cfg.Processors = conf.Processors
	cfg.Pipelines = map[component.ID]partialPipeline{}
	for k, v := range conf.Service.Pipelines {
		cfg.Pipelines[k] = partialPipeline{
			Receivers:  v.Receivers,
			Processors: v.Processors,
		}
	}

	if len(cfg.Receivers) == 0 {
		return nil, errors.New("must have at least one receiver")
	}

	if len(cfg.Processors) > 0 && len(cfg.Pipelines) == 0 {
		return nil, errors.New("template containing processors must have at least one pipeline")
	}
	cfg.applyScope(id.tmplType, id.instName)
	return cfg, nil
}

func (cfg *templateConfig) applyScope(tmplType, instName string) {
	// Apply a scope to all component IDs in the template.
	// At a minimum, the this appends the template type, but will
	// also apply the template instance name if possible.
	scopeMapKeys(cfg.Receivers, tmplType, instName)
	scopeMapKeys(cfg.Processors, tmplType, instName)
	for _, p := range cfg.Pipelines {
		scopeSliceValues(p.Receivers, tmplType, instName)
		scopeSliceValues(p.Processors, tmplType, instName)
	}
}

// In order to ensure the component IDs in this template are globally unique,
// appending the template instance name to the ID of each component.
func scopeMapKeys(cfgs map[component.ID]component.Config, tmplType, instName string) {
	// To avoid risk of collision, build a list of IDs,
	// sort them by length, and then append starting with the longest.
	componentIDs := make([]component.ID, 0, len(cfgs))
	for componentID := range cfgs {
		componentIDs = append(componentIDs, componentID)
	}
	sort.Slice(componentIDs, func(i, j int) bool {
		return len(componentIDs[i].String()) > len(componentIDs[j].String())
	})
	for _, componentID := range componentIDs {
		cfg := cfgs[componentID]
		delete(cfgs, componentID)
		cfgs[scopedID(componentID, tmplType, instName)] = cfg
	}
}

func scopeSliceValues(componentIDs []component.ID, tmplType, instName string) {
	for i, componentID := range componentIDs {
		componentIDs[i] = scopedID(componentID, tmplType, instName)
	}
}

func scopedID(componentID component.ID, templateType, instanceName string) component.ID {
	if instanceName == "" {
		return component.NewIDWithName(componentID.Type(), templateType)
	}
	return component.NewIDWithName(componentID.Type(), templateType+"/"+instanceName)
}

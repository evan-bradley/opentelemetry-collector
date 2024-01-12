// Code generated by "go.opentelemetry.io/collector/cmd/builder". DO NOT EDIT.

// Program otelcorecol is an OpenTelemetry Collector binary.
package main

import (
	"log"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/otelcol/templateconverter"
)

func main() {
	info := component.BuildInfo{
		Command:     "otelcorecol",
		Description: "Local OpenTelemetry Collector binary, testing only.",
		Version:     "0.91.0-dev",
	}

	if err := run(otelcol.CollectorSettings{BuildInfo: info, Factories: components}); err != nil {
		log.Fatal(err)
	}
}

func runInteractive(params otelcol.CollectorSettings) error {
	params.Converters = []otelcol.ConfigConverter{templateconverter.New()}
	cmd := otelcol.NewCommand(params)
	if err := cmd.Execute(); err != nil {
		log.Fatalf("collector server run finished with error: %v", err)
	}

	return nil
}

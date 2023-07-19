// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package otelcol defines utilities aid in managing a Collector process.
// These utilities are intended to be configured by a Collector distribution to
// provide metadata about the distribution and provide a set of supported Collector
// components.
// This package is split into two parts:
// Command: Does initial setup of the package to handle things like command line arguments.
// Collector: The structure that comprises the Collector process. Primarily intended to
// handle OS signals and do any other basic interfacing with the OS. Starts and manages
// the Collector Service.
package otelcol // import "go.opentelemetry.io/collector/otelcol"

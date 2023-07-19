// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package service comprises the Collector service that receives, processes, and exports telemetry.
// As a host for components, it also provides an implementation of component.Host to these components.
// The service is tasked with instantiating telemetry pipelines and a component.Host interface
// to provide to components within those pipelines according to the configuration it
// receives from a set of URIs.
package service // import "go.opentelemetry.io/collector/service"

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package service handles the command-line, configuration, and runs the
// OpenTelemetry Collector.
package otelcol // import "go.opentelemetry.io/collector/otelcol"

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"go.uber.org/multierr"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/service"
)

// State defines Collector's state.
type State int

const (
	StateStarting State = iota
	StateRunning
	StateClosing
	StateClosed
)

func (s State) String() string {
	switch s {
	case StateStarting:
		return "Starting"
	case StateRunning:
		return "Running"
	case StateClosing:
		return "Closing"
	case StateClosed:
		return "Closed"
	}
	return "UNKNOWN"
}

// CollectorSettings holds configuration for creating a new Collector.
type CollectorSettings struct {
	// Factories service factories.
	Factories service.Factories

	// BuildInfo provides collector start information.
	BuildInfo component.BuildInfo

	// DisableGracefulShutdown disables the automatic graceful shutdown
	// of the collector on SIGINT or SIGTERM.
	// Users who want to handle signals themselves can disable this behavior
	// and manually handle the signals to shutdown the collector.
	DisableGracefulShutdown bool

	configURIs []string

	// LoggingOptions provides a way to change behavior of zap logging.
	LoggingOptions []zap.Option

	// SkipSettingGRPCLogger avoids setting the grpc logger
	SkipSettingGRPCLogger bool
}

// (Internal note) Collector Lifecycle:
// - New constructs a new Collector.
// - Run starts the collector.
// - Run calls setupConfigurationComponents to handle configuration.
//   If configuration parser fails, collector's config can be reloaded.
//   Collector can be shutdown if parser gets a shutdown error.
// - Run runs runAndWaitForShutdownEvent and waits for a shutdown event.
//   SIGINT and SIGTERM, errors, and (*Collector).Shutdown can trigger the shutdown events.
// - Upon shutdown, pipelines are notified, then pipelines and extensions are shut down.
// - Users can call (*Collector).Shutdown anytime to shut down the collector.

// Collector represents a server providing the OpenTelemetry Collector service.
type Collector struct {
	set CollectorSettings

	service *service.Service
	state   *atomic.Int32

	// shutdownChan is used to terminate the collector.
	shutdownChan chan struct{}
	// signalsChannel is used to receive termination signals from the OS.
	signalsChannel chan os.Signal
	// asyncErrorChannel is used to signal a fatal error from any component.
	asyncErrorChannel chan error
}

// NewCollector creates and returns a new instance of Collector.
func NewCollector(set CollectorSettings) (*Collector, error) {
	state := &atomic.Int32{}
	state.Store(int32(StateStarting))
	asyncErrorChannel := make(chan error)
	serviceSettings := service.Settings{
		ConfigURIs:        set.configURIs,
		BuildInfo:         set.BuildInfo,
		AsyncErrorChannel: asyncErrorChannel,
		LoggingOptions:    set.LoggingOptions,
	}
	srv, err := service.New(serviceSettings)
	if err != nil {
		return nil, err
	}
	return &Collector{
		set:          set,
		state:        state,
		shutdownChan: make(chan struct{}),
		// Per signal.Notify documentation, a size of the channel equaled with
		// the number of signals getting notified on is recommended.
		signalsChannel:    make(chan os.Signal, 3),
		asyncErrorChannel: asyncErrorChannel,
		service:           srv,
	}, nil
}

// GetState returns current state of the collector server.
func (col *Collector) GetState() State {
	return State(col.state.Load())
}

// Shutdown shuts down the collector server.
func (col *Collector) Shutdown() {
	// Only shutdown if we're in a Running or Starting State else noop
	state := col.GetState()
	if state == StateRunning || state == StateStarting {
		defer func() {
			recover() // nolint:errcheck
		}()
		close(col.shutdownChan)
	}
}

// setupConfigurationComponents loads the config and starts the components. If all the steps succeeds it
// sets the col.service with the service currently running.
func (col *Collector) setupConfigurationComponents(ctx context.Context) error {
	col.setCollectorState(StateStarting)

	var err error

	if err != nil {
		return err
	}

	if !col.set.SkipSettingGRPCLogger {
		// TODO: Do this here or in the service?
		// grpclog.SetLogger(col.service.Logger(), cfg.Telemetry.Logs.Level)
	}

	if err = col.service.Start(ctx); err != nil {
		return multierr.Combine(err, col.service.Shutdown(ctx))
	}
	col.setCollectorState(StateRunning)

	return nil
}

func (col *Collector) reloadConfiguration(ctx context.Context) error {
	col.service.Logger().Warn("Config updated, restart service")
	col.setCollectorState(StateClosing)

	if err := col.service.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown the retiring config: %w", err)
	}

	if err := col.setupConfigurationComponents(ctx); err != nil {
		return fmt.Errorf("failed to setup configuration components: %w", err)
	}

	return nil
}

func (col *Collector) DryRun(ctx context.Context) error {
	set := service.Settings{
		ConfigURIs:        col.set.configURIs,
		BuildInfo:         col.set.BuildInfo,
		AsyncErrorChannel: col.asyncErrorChannel,
		LoggingOptions:    col.set.LoggingOptions,
	}

	_, err := service.New(set)
	// Call service.init here?
	if err != nil {
		return err
	}
	return nil
}

// Run starts the collector according to the given configuration, and waits for it to complete.
// Consecutive calls to Run are not allowed, Run shouldn't be called once a collector is shut down.
func (col *Collector) Run(ctx context.Context) error {
	if err := col.setupConfigurationComponents(ctx); err != nil {
		col.setCollectorState(StateClosed)
		return err
	}

	// Always notify with SIGHUP for configuration reloading.
	signal.Notify(col.signalsChannel, syscall.SIGHUP)
	defer signal.Stop(col.signalsChannel)

	// Only notify with SIGTERM and SIGINT if graceful shutdown is enabled.
	if !col.set.DisableGracefulShutdown {
		signal.Notify(col.signalsChannel, os.Interrupt, syscall.SIGTERM)
	}

LOOP:
	for {
		select {
		case err := <-col.asyncErrorChannel:
			col.service.Logger().Error("Asynchronous error received, terminating process", zap.Error(err))
			break LOOP
		case s := <-col.signalsChannel:
			col.service.Logger().Info("Received signal from OS", zap.String("signal", s.String()))
			if s != syscall.SIGHUP {
				break LOOP
			}
			if err := col.reloadConfiguration(ctx); err != nil {
				return err
			}
		case <-col.shutdownChan:
			col.service.Logger().Info("Received shutdown request")
			break LOOP
		case <-ctx.Done():
			col.service.Logger().Info("Context done, terminating process", zap.Error(ctx.Err()))
			// Call shutdown with background context as the passed in context has been canceled
			return col.shutdown(context.Background())
		}
	}
	return col.shutdown(ctx)
}

func (col *Collector) shutdown(ctx context.Context) error {
	col.setCollectorState(StateClosing)

	// Accumulate errors and proceed with shutting down remaining components.
	var errs error

	// if err := col.set.ConfigProvider.Shutdown(ctx); err != nil {
	// 	errs = multierr.Append(errs, fmt.Errorf("failed to shutdown config provider: %w", err))
	// }

	// shutdown service
	if err := col.service.Shutdown(ctx); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to shutdown service after error: %w", err))
	}

	col.setCollectorState(StateClosed)

	return errs
}

// setCollectorState provides current state of the collector
func (col *Collector) setCollectorState(state State) {
	col.state.Store(int32(state))
}

// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package consumer // import "go.opentelemetry.io/collector/consumer"

import (
	"context"
	"errors"
)

// Capabilities describes the capabilities of a Processor.
type Capabilities struct {
	// MutatesData is set to true if Consume* function of the
	// processor modifies the input TraceData or MetricsData argument.
	// Processors which modify the input data MUST set this flag to true. If the processor
	// does not modify the data it MUST set this flag to false. If the processor creates
	// a copy of the data before modifying then this flag can be safely set to false.
	MutatesData bool
}

type consumer[S any] interface {
	Capabilities() Capabilities

	Consume(ctx context.Context, data S) error
}

// consumeFunc is a helper function that is similar to Consume
type consumeFunc[S any] func(ctx context.Context, data S) error

// Consume calls f(ctx, ld).
func (f consumeFunc[S]) Consume(ctx context.Context, data S) error {
	return f(ctx, data)
}

type baseConsumer[S any] struct {
	*baseImpl
	consumeFunc[S]
}

// newConsumer returns a Consumer configured with the provided options.
func newConsumer[S any](consume consumeFunc[S], options ...Option) (consumer[S], error) {
	if consume == nil {
		return nil, errNilFunc
	}
	return &baseConsumer[S]{
		baseImpl:    newBaseImpl(options...),
		consumeFunc: consume,
	}, nil
}

var errNilFunc = errors.New("nil consumer func")

type baseImpl struct {
	capabilities Capabilities
}

// Option to construct new consumers.
type Option func(*baseImpl)

// WithCapabilities overrides the default GetCapabilities function for a processor.
// The default GetCapabilities function returns mutable capabilities.
func WithCapabilities(capabilities Capabilities) Option {
	return func(o *baseImpl) {
		o.capabilities = capabilities
	}
}

// Capabilities implementation of the base
func (bs baseImpl) Capabilities() Capabilities {
	return bs.capabilities
}

func newBaseImpl(options ...Option) *baseImpl {
	bs := &baseImpl{
		capabilities: Capabilities{MutatesData: false},
	}

	for _, op := range options {
		op(bs)
	}

	return bs
}

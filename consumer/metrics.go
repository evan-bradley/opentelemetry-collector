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

	"go.opentelemetry.io/collector/pdata/pmetric"
)

// Metrics is an interface that receives pmetric.Metrics, processes it
// as needed, and sends it to the next processing node if any or to the destination.
type Metrics interface {
	Consumer[pmetric.Metrics]

	// deprecated: use Metrics.Consume instead
	ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error
}

type baseMetrics struct {
	*baseConsumer[pmetric.Metrics]
}

func (m baseMetrics) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	return m.Consume(ctx, md)
}

// ConsumeMetricsFunc is a helper function that is similar to ConsumeMetrics.
type ConsumeMetricsFunc = ConsumeFunc[pmetric.Metrics]

// NewMetrics returns a Metrics configured with the provided options.
func NewMetrics(consume ConsumeMetricsFunc, options ...Option) (Metrics, error) {
	if consume == nil {
		return nil, errNilFunc
	}
	c, err := NewConsumer(consume, options...)

	if err != nil {
		return nil, err
	}

	bc := c.(baseConsumer[pmetric.Metrics])
	return &baseMetrics{
		baseConsumer: &bc,
	}, nil
}

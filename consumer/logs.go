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

	"go.opentelemetry.io/collector/pdata/plog"
)

// Logs is an interface that receives plog.Logs, processes it
// as needed, and sends it to the next processing node if any or to the destination.
type Logs interface {
	consumer[plog.Logs]

	// deprecated: use Logs.Consume instead
	ConsumeLogs(ctx context.Context, md plog.Logs) error
}

type baseLogs struct {
	*baseConsumer[plog.Logs]
}

func (m baseLogs) ConsumeLogs(ctx context.Context, md plog.Logs) error {
	return m.Consume(ctx, md)
}

// ConsumeLogsFunc is a helper function that is similar to ConsumeLogs.
type ConsumeLogsFunc = consumeFunc[plog.Logs]

// NewLogs returns a Logs configured with the provided options.
func NewLogs(consume ConsumeLogsFunc, options ...Option) (Logs, error) {
	if consume == nil {
		return nil, errNilFunc
	}
	c, err := newConsumer(consume, options...)

	if err != nil {
		return nil, err
	}

	bc := c.(baseConsumer[plog.Logs])
	return &baseLogs{
		baseConsumer: &bc,
	}, nil
}

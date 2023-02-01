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
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// Metrics is an interface that receives pmetric.Metrics, processes it
// as needed, and sends it to the next processing node if any or to the destination.
type Metrics = Consumer[pmetric.Metrics]

// ConsumeMetricsFunc is a helper function that is similar to ConsumeMetrics.
type ConsumeMetricsFunc = ConsumeFunc[pmetric.Metrics]

// NewMetrics returns a Metrics configured with the provided options.
var NewMetrics = NewConsumer[pmetric.Metrics]

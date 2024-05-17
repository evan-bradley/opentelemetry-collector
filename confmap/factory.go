// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package confmap // import "go.opentelemetry.io/collector/confmap"

type moduleFactory[T any, S any] interface {
	Create(s S) T

	unexportedFactoryFunc()
}

type createConfmapFunc[T any, S any] func(s S) T

type factory[T any, S any] struct {
	f createConfmapFunc[T, S]
}

var _ moduleFactory[any, any] = (*factory[any, any])(nil)

func (c *factory[T, S]) Create(s S) T {
	return c.f(s)
}

func (f *factory[T, S]) unexportedFactoryFunc() {}

func newConfmapModuleFactory[T any, S any](f createConfmapFunc[T, S]) moduleFactory[T, S] {
	return &factory[T, S]{
		f: f,
	}
}

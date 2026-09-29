// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package evaluator

import (
	"fmt"
	"maps"
	"slices"
)

// Options holds backend-agnostic settings for creating an Evaluator.
type Options struct {
	// Model selects the backend model; empty means the backend default.
	Model string
}

// Factory creates an Evaluator for a backend.
type Factory func(opts Options) (Evaluator, error)

// factories is written only by Register during program initialization and is
// read-only afterwards, so it needs no lock.
var factories = map[string]Factory{}

// Register registers a Factory under the given backend name.
// It must be called only from init functions: factories is not guarded by a
// lock, and New and Backends read it without one.
func Register(name string, factory Factory) {
	factories[name] = factory
}

// New creates an Evaluator for the named backend. It only reads the backend
// registry, which is written only during initialization.
func New(name string, opts Options) (Evaluator, error) {
	factory, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("unknown evaluator backend %q (available: %v)", name, Backends())
	}
	return factory(opts)
}

// Backends returns the registered backend names in sorted order. It only reads
// the backend registry, which is written only during initialization.
func Backends() []string {
	return slices.Sorted(maps.Keys(factories))
}

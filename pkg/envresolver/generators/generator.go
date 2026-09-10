package generators

import (
	"fmt"
	"sync"
)

// Generator produces a unique value for a dynamic header on each call.
// Implementations must be safe for concurrent use.
type Generator interface {
	// Name returns the identifier used in ${generate:NAME} tokens.
	Name() string
	// Generate produces a fresh value suitable for use as an HTTP header value.
	Generate() string
}

var (
	mu         sync.RWMutex
	registry   = make(map[string]Generator)
	registered bool
)

// Register adds a generator to the global registry. Panics on duplicate names.
// Call from init() functions in generator implementation files.
func Register(g Generator) {
	mu.Lock()
	defer mu.Unlock()
	name := g.Name()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("generators: duplicate registration for %q", name))
	}
	registry[name] = g
}

// Get returns the named generator or nil if not found.
func Get(name string) Generator {
	mu.RLock()
	defer mu.RUnlock()
	return registry[name]
}

// Has returns true if a generator with the given name is registered.
func Has(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := registry[name]
	return ok
}

// List returns the names of all registered generators.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

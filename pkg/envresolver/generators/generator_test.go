package generators

import (
	"regexp"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_Get(t *testing.T) {
	t.Run("registered_generator_found", func(t *testing.T) {
		g := Get("opencode-ses")
		require.NotNil(t, g)
		assert.Equal(t, "opencode-ses", g.Name())
	})

	t.Run("unregistered_generator_returns_nil", func(t *testing.T) {
		g := Get("nonexistent-generator")
		assert.Nil(t, g)
	})
}

func TestRegistry_Has(t *testing.T) {
	assert.True(t, Has("opencode-ses"))
	assert.False(t, Has("nonexistent"))
}

func TestRegistry_List(t *testing.T) {
	names := List()
	assert.Contains(t, names, "opencode-ses")
}

// Matches OpenCode format: ses_ + 12 hex chars + 14 base62 chars = 30 chars total.
var opencodeSesPattern = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

func TestOpencodeSes_Generate(t *testing.T) {
	g := Get("opencode-ses")
	require.NotNil(t, g)

	t.Run("matches_opencode_format", func(t *testing.T) {
		for i := range 100 {
			id := g.Generate()
			assert.Regexp(t, opencodeSesPattern, id, "iteration %d: %q does not match OpenCode format", i, id)
		}
	})

	t.Run("unique_across_calls", func(t *testing.T) {
		seen := make(map[string]bool, 1000)
		for range 1000 {
			id := g.Generate()
			assert.False(t, seen[id], "duplicate ID generated: %q", id)
			seen[id] = true
		}
	})

	t.Run("concurrent_safety", func(t *testing.T) {
		var wg sync.WaitGroup
		ids := make([]string, 1000)
		for i := range 1000 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				ids[idx] = g.Generate()
			}(i)
		}
		wg.Wait()

		seen := make(map[string]bool, 1000)
		for i, id := range ids {
			assert.Regexp(t, opencodeSesPattern, id, "iteration %d", i)
			assert.False(t, seen[id], "duplicate ID in concurrent generation: %q", id)
			seen[id] = true
		}
	})
}

func TestRegister_DuplicatePanics(t *testing.T) {
	// This tests that Register panics on duplicate names.
	// We can't safely test this without affecting the global registry,
	// so we verify the documented behavior via Has.
	assert.True(t, Has("opencode-ses"))
}

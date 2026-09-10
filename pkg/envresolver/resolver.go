// Package envresolver expands ${VAR} and ${VAR:-default} placeholders in
// configuration strings using environment variable lookups. It intentionally
// does not support the bare $VAR form: config files often contain literal
// dollar signs (shell scripts, cost strings, regex), and requiring braces
// eliminates ambiguity without meaningful ergonomic cost.
//
// Dynamic header generation is supported via ${generate:NAME} tokens. Each
// generator produces a unique value per request. Multiple headers sharing
// the same generator name within a single request receive the same value,
// enabling session affinity patterns (e.g. x-session-id and x-session-affinity
// both receiving the same ses_xxx identifier).
package envresolver

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/thushan/olla/pkg/envresolver/generators"
)

// generatePattern matches ${generate:NAME} tokens for dynamic header values.
var generatePattern = regexp.MustCompile(`\$\{generate:([^}]+)\}`)

// HasGenerateToken reports whether s contains any ${generate:...} token.
// Use this to decide whether a header value needs per-request resolution.
func HasGenerateToken(s string) bool {
	return strings.Contains(s, "${generate:")
}

// DynamicHeaderContext carries generated values across multiple header
// expansions within a single request. All headers that reference the same
// generator name (e.g. ${generate:opencode-ses}) receive the same value.
// Create one per request via NewDynamicHeaderContext, then pass it to
// ExpandDynamic for each header value.
type DynamicHeaderContext struct {
	values map[string]string
}

// NewDynamicHeaderContext creates a fresh context for a single request.
func NewDynamicHeaderContext() *DynamicHeaderContext {
	return &DynamicHeaderContext{values: make(map[string]string)}
}

// Get returns the cached value for the named generator, generating it on
// first access. Returns empty string for unknown generator names.
func (c *DynamicHeaderContext) Get(name string) string {
	if v, ok := c.values[name]; ok {
		return v
	}
	g := generators.Get(name)
	if g == nil {
		return ""
	}
	v := g.Generate()
	c.values[name] = v
	return v
}

// tokenPattern matches ${VAR} and ${VAR:-default}. No nesting, no bare $VAR.
var tokenPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Expand replaces every ${VAR} and ${VAR:-default} placeholder in s with its
// resolved value. An unset variable with no default resolves to the empty
// string. Expand never returns an error; use ExpandStrict when a missing
// variable must be fatal. ${generate:...} tokens are left untouched for
// ExpandDynamic to resolve per-request.
func Expand(s string) string {
	if s == "" || !strings.Contains(s, "${") {
		return s
	}

	return tokenPattern.ReplaceAllStringFunc(s, func(token string) string {
		expr := token[2 : len(token)-1] // strip ${ and }
		// Skip ${generate:...} tokens - these are resolved per-request by ExpandDynamic.
		if strings.HasPrefix(expr, "generate:") {
			return token
		}
		name, fallback, hasFallback := strings.Cut(expr, ":-")

		v, set := os.LookupEnv(name)
		// POSIX :- semantics: use default when the variable is unset OR empty.
		// An explicitly set but empty variable still triggers the default, matching
		// shell behaviour and making empty-string auth values detectable downstream.
		if set && v != "" {
			return v
		}
		if hasFallback {
			return fallback
		}
		return ""
	})
}

// ExpandStrict is like Expand but returns an error when a placeholder has no
// environment value and no default. The error message names the variable but
// never echoes the surrounding string or any partial value, so secrets in
// adjacent placeholders do not leak into logs. ${generate:...} tokens are
// left untouched for ExpandDynamicStrict to resolve per-request.
func ExpandStrict(s string) (string, error) {
	if s == "" || !strings.Contains(s, "${") {
		return s, nil
	}

	var missing []string

	expanded := tokenPattern.ReplaceAllStringFunc(s, func(token string) string {
		expr := token[2 : len(token)-1]
		// Skip ${generate:...} tokens - these are resolved per-request.
		if strings.HasPrefix(expr, "generate:") {
			return token
		}
		name, fallback, hasFallback := strings.Cut(expr, ":-")

		v, set := os.LookupEnv(name)
		// POSIX :- semantics: empty triggers default just like unset.
		if set && v != "" {
			return v
		}
		if hasFallback {
			return fallback
		}
		// Only report as missing when the variable is genuinely unset;
		// an explicit empty value is a valid (if unusual) operator choice
		// and is handled by the downstream empty-token validation.
		if !set {
			missing = append(missing, name)
		}
		return ""
	})

	if len(missing) > 0 {
		errs := make([]error, len(missing))
		for i, name := range missing {
			errs[i] = fmt.Errorf("required environment variable %q is not set", name)
		}
		return "", errors.Join(errs...)
	}

	return expanded, nil
}

// ExpandWithFile resolves a config value that may come from either a literal
// string or a file path (the _file sibling-field convention). Callers pass the
// literal value and the file path; exactly one must be non-empty.
//
// When fileValue is set, the file is read and its contents are returned with
// leading/trailing whitespace trimmed. This mirrors the Docker Secrets / k8s
// mounted-secret pattern where a file holds a single secret value.
//
// Both values being non-empty is a configuration error the operator must fix
// before the process starts. This function fails fast so the mistake surfaces
// immediately rather than silently preferring one source.
func ExpandWithFile(value, fileValue string) (string, error) {
	hasValue := value != ""
	hasFile := fileValue != ""

	if hasValue && hasFile {
		return "", errors.New("both value and value_file are set; use exactly one")
	}

	if hasFile {
		raw, err := os.ReadFile(fileValue)
		if err != nil {
			// Report the path but not any partial content.
			return "", fmt.Errorf("reading secret file %q: %w", fileValue, err)
		}
		return strings.TrimSpace(string(raw)), nil
	}

	// Plain value path: still expand any ${VAR} placeholders inside it.
	return Expand(value), nil
}

// ExpandDynamic resolves both ${VAR} environment placeholders and ${generate:NAME}
// dynamic tokens in s. Generate tokens are resolved via ctx, which caches values
// so that multiple headers sharing the same generator name receive the same value.
// Env vars are resolved first, then generate tokens.
func ExpandDynamic(s string, ctx *DynamicHeaderContext) string {
	if s == "" || (!strings.Contains(s, "${") && !strings.Contains(s, "${generate:")) {
		return s
	}

	// First pass: expand environment variables.
	result := Expand(s)

	// Second pass: resolve ${generate:NAME} tokens.
	if ctx != nil && strings.Contains(result, "${generate:") {
		result = generatePattern.ReplaceAllStringFunc(result, func(token string) string {
			name := token[11 : len(token)-1] // strip ${generate: and }
			return ctx.Get(name)
		})
	}

	return result
}

// ExpandDynamicStrict is like ExpandDynamic but returns an error when a
// ${generate:NAME} token references an unregistered generator.
func ExpandDynamicStrict(s string, ctx *DynamicHeaderContext) (string, error) {
	if s == "" || (!strings.Contains(s, "${") && !strings.Contains(s, "${generate:")) {
		return s, nil
	}

	// First pass: expand environment variables (may produce errors).
	result, err := ExpandStrict(s)
	if err != nil {
		return "", err
	}

	// Second pass: resolve ${generate:NAME} tokens.
	if ctx != nil && strings.Contains(result, "${generate:") {
		var missing []string
		result = generatePattern.ReplaceAllStringFunc(result, func(token string) string {
			name := token[11 : len(token)-1]
			if !generators.Has(name) {
				missing = append(missing, name)
				return ""
			}
			return ctx.Get(name)
		})
		if len(missing) > 0 {
			errs := make([]error, len(missing))
			for i, name := range missing {
				errs[i] = fmt.Errorf("unknown header generator %q", name)
			}
			return "", errors.Join(errs...)
		}
	}

	return result, nil
}

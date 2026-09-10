---
title: Dynamic Headers - Auto-Generated Session Headers for OpenCode
description: Configure auto-generated session headers on Olla endpoints with ${generate:} syntax. Enable OpenCode session affinity with x-session-affinity and x-session-id headers regenerated per request.
keywords: olla dynamic headers, auto-generated headers, session affinity, OpenCode session headers, ${generate:opencode-ses}, x-session-affinity, x-session-id
---

# Dynamic Headers

Olla supports auto-generated header values that are resolved on every request rather than at config load time. The primary use case is **OpenCode session affinity**: OpenCode sends `x-session-affinity` and `x-session-id` headers, and Olla can auto-generate matching values for backends that need them.

## Quick Start

Add `${generate:opencode-ses}` to your endpoint headers:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        headers:
          x-session-affinity: "${generate:opencode-ses}"
          x-session-id: "${generate:opencode-ses}"
```

On each request from OpenCode, Olla generates a unique session ID and sets both headers to the same value (e.g. `ses_f740de905ffegYXvX2mVOV2M8C`).

## OpenCode Integration

### How It Works

OpenCode connects to Olla via the OpenAI-compatible endpoint (`/olla/openai/v1`). When OpenCode sends a request, Olla can inject auto-generated session headers before forwarding to the backend:

```
OpenCode  ──(request)──>  Olla  ──(adds headers)──>  Backend
                          │
                          ├─ x-session-affinity: ses_f740de905ffegYXvX2mVOV2M8C
                          └─ x-session-id:       ses_f740de905ffegYXvX2mVOV2M8C
```

### Value Sharing

Both headers receive the **same generated value** per request. This is intentional: backends that check session affinity use the same ID from both headers to verify the session is consistent.

```yaml
headers:
  x-session-affinity: "${generate:opencode-ses}"   # ses_f740de905ffegYXvX2mVOV2M8C
  x-session-id: "${generate:opencode-ses}"         # ses_f740de905ffegYXvX2mVOV2M8C (same value)
```

### With Auth

Combine dynamic session headers with endpoint authentication:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://protected-vllm:8000"
        name: "vllm-prod"
        type: "vllm"
        auth:
          type: bearer
          token: "${VLLM_API_KEY}"
        headers:
          x-session-affinity: "${generate:opencode-ses}"
          x-session-id: "${generate:opencode-ses}"
```

### Multiple Backends

Add session headers to every endpoint that OpenCode should use with affinity:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
        headers:
          x-session-affinity: "${generate:opencode-ses}"
          x-session-id: "${generate:opencode-ses}"

      - url: "http://localhost:1234"
        name: "local-lm-studio"
        type: "lm-studio"
        priority: 100
        headers:
          x-session-affinity: "${generate:opencode-ses}"
          x-session-id: "${generate:opencode-ses}"

      - url: "http://localhost:8000"
        name: "local-vllm"
        type: "vllm"
        priority: 100
        headers:
          x-session-affinity: "${generate:opencode-ses}"
          x-session-id: "${generate:opencode-ses}"
```

### Mixing with Environment Variables

Dynamic headers work alongside `${VAR}` environment interpolation:

```yaml
headers:
  x-session-affinity: "${generate:opencode-ses}"        # auto-generated per request
  x-session-id: "${generate:opencode-ses}"              # same value as above
  x-api-key: "${BACKEND_API_KEY}"                       # static env var
  x-source: "opencode"                                  # static literal
  x-trace: "olla-${generate:opencode-ses}"             # mixed dynamic + static
```

## The `${generate:opencode-ses}` Format

The `opencode-ses` generator produces identifiers matching the OpenCode session format:

```
ses_XXXXXXXXXXXXYYYYYYYYYYYYYY
│    │              │
│    │              └── 14 random base62 characters (83 bits entropy)
│    └── 12 hex characters (timestamp + counter encoded)
└── prefix
```

- **Total length**: 30 characters (`ses_` + 26 chars)
- **Entropy**: ~131 bits (48 bits timestamp/counter + 83 bits random)
- **Collision resistance**: Unique per request via `crypto/rand`
- **Thread-safe**: Concurrent requests generate independent values

Example output: `ses_f740de905ffegYXvX2mVOV2M8C`

## Order of Precedence

When a forwarded request is assembled, headers are applied in this order:

1. **Client request headers** are copied (hop-by-hop and sensitive headers stripped)
2. **Static `headers:`** values are set
3. **Dynamic `${generate:...} headers:** are resolved per-request and set
4. **`auth:`** sets the credential header (overrides everything for its header name)

The `auth:` block always wins. If you put the same header name in both `headers:` and `auth:`, the `auth:` value is used.

## Adding Custom Generators

The generator system is pluggable. To add a new generator, create a file in `pkg/envresolver/generators/`:

```go
package generators

func init() {
    Register(&myGenerator{})
}

type myGenerator struct{}

func (g *myGenerator) Name() string {
    return "my-custom-gen"
}

func (g *myGenerator) Generate() string {
    // Return a unique value for each call.
    // Must be safe for concurrent use.
    return "unique-value-here"
}
```

The generator is automatically available as `${generate:my-custom-gen}` after rebuilding.

## Debugging

Dynamic headers are applied at the proxy layer. To verify they're being set:

1. Check the outbound request with debug logging:

    ```yaml
    logging:
      level: "debug"
    ```

2. Use the inspector to capture request/response pairs:

    ```yaml
    translators:
      anthropic:
        inspector:
          enabled: true
          output_dir: "logs/inspector"
    ```

3. Check response headers for `X-Olla-Endpoint` and `X-Olla-Backend-Type` to confirm the request reached the backend

## Security Notes

- Generated values are cryptographically random (using `crypto/rand`)
- Header templates with `${generate:...}` are never exposed through status or dashboard endpoints
- Dynamic headers are stripped from backend responses (like all custom `headers:` entries) to prevent reflection attacks
- Generated session IDs do not carry meaningful state; they are identifiers, not authentication tokens

## Related Documentation

- [OpenCode Integration](../integrations/frontend/opencode.md) - Full OpenCode setup guide
- [Endpoint Authentication](endpoint-auth.md) - Static auth and credential management
- [Configuration Reference](reference.md) - Full endpoint configuration reference
- [Sticky Sessions](../concepts/sticky-sessions.md) - KV-cache affinity routing

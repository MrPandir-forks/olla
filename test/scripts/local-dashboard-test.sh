#!/usr/bin/env bash
# Local dashboard development with mock backends.
#
# Starts 2 ollamock instances + Olla + Vite dev server.
# Dashboard: http://localhost:5173/internal/ui/
#
# The Models tab will show a family group with models having different
# endpoint counts (shared-model on 2 endpoints, others on 1).
#
# Usage:
#   ./test/scripts/local-dashboard-test.sh          # uses default ports
#   OLLA_PORT=41141 ./test/scripts/local-dashboard-test.sh  # custom Olla port
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
MOCK_A_PORT=19431
MOCK_B_PORT=19432
OLLA_PORT="${OLLA_PORT:-41141}"
VITE_PORT=5173

cleanup() {
  echo ""
  echo "Shutting down..."
  kill "${MOCK_A_PID:-}" "${MOCK_B_PID:-}" "${OLLA_PID:-}" "${VITE_PID:-}" 2>/dev/null || true
  wait "${MOCK_A_PID:-}" "${MOCK_B_PID:-}" "${OLLA_PID:-}" "${VITE_PID:-}" 2>/dev/null || true
  rm -f /tmp/olla-test.XXXXXX
}
trap cleanup EXIT

# 1) Start ollamock instances
echo "==> Starting ollamock-a on :${MOCK_A_PORT} (models: llama3.2,shared-model,gemma3)..."
go run ./test/cmd/ollamock \
  --addr "127.0.0.1:${MOCK_A_PORT}" \
  --name "mock-a" \
  --models "llama3.2,shared-model,gemma3" &
MOCK_A_PID=$!

echo "==> Starting ollamock-b on :${MOCK_B_PORT} (models: phi4,shared-model)..."
go run ./test/cmd/ollamock \
  --addr "127.0.0.1:${MOCK_B_PORT}" \
  --name "mock-b" \
  --models "phi4,shared-model" &
MOCK_B_PID=$!

sleep 2

# 2) Write Olla config
OLLACFG=$(mktemp /tmp/olla-test.XXXXXX)
cat > "$OLLACFG" <<EOF
server:
  host: "127.0.0.1"
  port: ${OLLA_PORT}
  read_timeout: 20s
  read_header_timeout: 10s
  write_timeout: 0s
  shutdown_timeout: 5s
  request_logging: false
  rate_limits:
    global_requests_per_minute: 100000
    per_ip_requests_per_minute: 50000
    health_requests_per_minute: 100000
    burst_size: 1000
    cleanup_interval: 5m
proxy:
  engine: "olla"
  profile: "auto"
  load_balancer: "least-connections"
  connection_timeout: 10s
  response_timeout: 60s
  read_timeout: 60s
discovery:
  type: "static"
  refresh_interval: 10s
  health_check:
    initial_delay: 1s
  static:
    endpoints:
      - url: "http://127.0.0.1:${MOCK_A_PORT}"
        name: "mock-a"
        type: "openai-compatible"
        priority: 100
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 2s
        check_timeout: 1s
      - url: "http://127.0.0.1:${MOCK_B_PORT}"
        name: "mock-b"
        type: "openai-compatible"
        priority: 100
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 2s
        check_timeout: 1s
  model_discovery:
    enabled: true
    interval: 30s
    timeout: 5s
model_registry:
  type: "memory"
  enable_unifier: true
  routing_strategy:
    type: "optimistic"
    options:
      fallback_behavior: "all"
logging:
  level: "info"
  format: "text"
  output: "stdout"
EOF

# 3) Build and start Olla
echo "==> Building Olla..."
cd "$ROOT"
go build -o /tmp/olla-test-bin .
echo "==> Starting Olla on :${OLLA_PORT}..."
/tmp/olla-test-bin -config "$OLLACFG" &
OLLA_PID=$!

sleep 2

# 4) Verify
echo ""
echo "==> Verifying models endpoint..."
curl -s "http://127.0.0.1:${OLLA_PORT}/internal/status/models?detailed=true&group=family" | python3 -m json.tool 2>/dev/null || \
  curl -s "http://127.0.0.1:${OLLA_PORT}/internal/status/models?detailed=true&group=family"
echo ""

# 5) Start Vite dev server with proxy to Olla
echo ""
echo "==> Starting Vite dev server (dashboard at http://localhost:${VITE_PORT}/internal/ui/)..."
cd "$ROOT/web/dashboard"
export OLLA_URL="http://127.0.0.1:${OLLA_PORT}"
npx vite --port "${VITE_PORT}" --host 0.0.0.0 &
VITE_PID=$!

echo ""
echo "==> All services running. Open http://localhost:${VITE_PORT}/internal/ui/"
echo "    Press Ctrl+C to stop all services."
wait

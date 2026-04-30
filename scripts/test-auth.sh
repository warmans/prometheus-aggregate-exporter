#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mkdir -p .build
BUILD_DIR="$ROOT_DIR/.build"
FIXTURE_AUTH_DIR="$ROOT_DIR/fixture/auth"

echo "test-password-123" > "$BUILD_DIR/test-auth-password.txt"
echo "test-bearer-token-123" > "$BUILD_DIR/test-auth-token.txt"
cat > "$BUILD_DIR/test-targets-config.json" <<'EOF'
{
  "targets": [
    {
      "name": "secure_basic",
      "url": "http://localhost:3011/histogram.txt",
      "auth": {
        "type": "basic",
        "username": "prometheus_scraper",
        "password_file": ".build/test-auth-password.txt"
      }
    },
    {
      "name": "secure_bearer",
      "url": "http://localhost:3011/histogram.txt",
      "auth": {
        "type": "bearer",
        "token_file": ".build/test-auth-token.txt"
      }
    },
    {
      "name": "no_auth",
      "url": "http://localhost:3011/histogram.txt"
    }
  ]
}
EOF

cleanup() {
  for f in "$BUILD_DIR/test-auth-failfast.pid" "$BUILD_DIR/test-auth-success.pid" "$BUILD_DIR/test-auth-bearer.pid" "$BUILD_DIR/test-auth-fixture.pid"; do
    if [[ -f "$f" ]]; then
      kill "$(cat "$f")" 2>/dev/null || true
    fi
  done
}
trap cleanup EXIT

# Start fixture server (Basic or Bearer)
(
  cd "$FIXTURE_AUTH_DIR"
  go run serve.go > "$BUILD_DIR/test-auth-fixture.log" 2>&1 & echo $! > "$BUILD_DIR/test-auth-fixture.pid"
)
sleep 1

# 1) Fail-fast: embedded credentials in target URL should be rejected
./bin/prometheus-aggregate-exporter \
  -targets="bad=http://prometheus_scraper:test-password-123@localhost:3011/histogram.txt" \
  -server.bind=":18080" \
  -verbose=true > "$BUILD_DIR/test-auth-failfast.log" 2>&1 & echo $! > "$BUILD_DIR/test-auth-failfast.pid"
sleep 1
curl -s localhost:18080/metrics > "$BUILD_DIR/test-auth-failfast.metrics"
awk '/credentials in target URL are not allowed/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-failfast.log"

# 2) Basic auth via flags should succeed
./bin/prometheus-aggregate-exporter \
  -targets="secure=http://localhost:3011/histogram.txt" \
  -server.bind=":18081" \
  -targets.auth.username="prometheus_scraper" \
  -targets.auth.password_file="$BUILD_DIR/test-auth-password.txt" \
  -verbose=true > "$BUILD_DIR/test-auth-success.log" 2>&1 & echo $! > "$BUILD_DIR/test-auth-success.pid"
sleep 1
curl -s localhost:18081/metrics > "$BUILD_DIR/test-auth-success.metrics"
awk '/^http_requests_total\{.*ae_source="secure"/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-success.metrics"

# 3) Bearer auth via flags should succeed
./bin/prometheus-aggregate-exporter \
  -targets="secure=http://localhost:3011/histogram.txt" \
  -server.bind=":18082" \
  -targets.auth.type="bearer" \
  -targets.auth.token_file="$BUILD_DIR/test-auth-token.txt" \
  -verbose=true > "$BUILD_DIR/test-auth-bearer.log" 2>&1 & echo $! > "$BUILD_DIR/test-auth-bearer.pid"
sleep 1
curl -s localhost:18082/metrics > "$BUILD_DIR/test-auth-bearer.metrics"
awk '/^http_requests_total\{.*ae_source="secure"/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-bearer.metrics"

# 4) Per-target auth via -targets.config should work (basic + bearer) and allow no-auth target
./bin/prometheus-aggregate-exporter \
  -targets.config="$BUILD_DIR/test-targets-config.json" \
  -server.bind=":18083" \
  -verbose=true > "$BUILD_DIR/test-auth-config.log" 2>&1 & echo $! > "$BUILD_DIR/test-auth-config.pid"
sleep 1
curl -s localhost:18083/metrics > "$BUILD_DIR/test-auth-config.metrics"
awk '/^http_requests_total\{.*ae_source="secure_basic"/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-config.metrics"
awk '/^http_requests_total\{.*ae_source="secure_bearer"/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-config.metrics"
awk '/failed to fetch URL .* due to status code: 401/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-config.log"

echo "OK: auth integration checks passed"


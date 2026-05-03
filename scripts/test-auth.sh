#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mkdir -p .build
BUILD_DIR="$ROOT_DIR/.build"
FIXTURE_AUTH_DIR="$ROOT_DIR/fixture/auth"

if command -v lsof >/dev/null 2>&1; then
  # Clean up any leftover processes that may keep ports busy (local dev convenience).
  for p in 3011 18080 18081 18082 18083 18084; do
    PIDS="$(lsof -ti tcp:"$p" 2>/dev/null || true)"
    if [[ -n "${PIDS}" ]]; then
      kill ${PIDS} 2>/dev/null || true
    fi
  done
fi

pick_port() {
  local port
  for _ in {1..50}; do
    port=$(( (RANDOM % 20000) + 20000 ))
    if command -v lsof >/dev/null 2>&1; then
      if [[ -z "$(lsof -ti tcp:"$port" 2>/dev/null || true)" ]]; then
        echo "$port"
        return 0
      fi
    else
      echo "$port"
      return 0
    fi
  done
  echo "Failed to pick a free port" >&2
  exit 1
}

FIXTURE_PORT="$(pick_port)"
FAILFAST_PORT="$(pick_port)"
BASIC_PORT="$(pick_port)"
BEARER_PORT="$(pick_port)"
CONFIG_JSON_PORT="$(pick_port)"
CONFIG_YAML_PORT="$(pick_port)"

echo "test-password-123" > "$BUILD_DIR/test-auth-password.txt"
echo "test-bearer-token-123" > "$BUILD_DIR/test-auth-token.txt"
cat > "$BUILD_DIR/test-targets-config.json" <<'EOF'
{
  "targets": [
    {
      "name": "secure_basic",
      "url": "http://localhost:FIXTURE_PORT/histogram.txt",
      "tls": {
        "insecure_skip_verify": true
      },
      "auth": {
        "type": "basic",
        "username": "prometheus_scraper",
        "password_file": "test-auth-password.txt"
      }
    },
    {
      "name": "secure_bearer",
      "url": "http://localhost:FIXTURE_PORT/histogram.txt",
      "auth": {
        "type": "bearer",
        "token_file": "test-auth-token.txt"
      }
    },
    {
      "name": "no_auth",
      "url": "http://localhost:FIXTURE_PORT/histogram.txt"
    }
  ]
}
EOF

perl -pi -e "s/FIXTURE_PORT/$FIXTURE_PORT/g" "$BUILD_DIR/test-targets-config.json"

cat > "$BUILD_DIR/test-targets-config.yaml" <<'EOF'
targets:
  - name: secure_basic
    url: http://localhost:FIXTURE_PORT/histogram.txt
    tls:
      insecure_skip_verify: true
    auth:
      type: basic
      username: prometheus_scraper
      password_file: test-auth-password.txt
  - name: secure_bearer
    url: http://localhost:FIXTURE_PORT/histogram.txt
    auth:
      type: bearer
      token_file: test-auth-token.txt
  - name: no_auth
    url: http://localhost:FIXTURE_PORT/histogram.txt
EOF

perl -pi -e "s/FIXTURE_PORT/$FIXTURE_PORT/g" "$BUILD_DIR/test-targets-config.yaml"

cleanup() {
  for f in "$BUILD_DIR/test-auth-failfast.pid" "$BUILD_DIR/test-auth-success.pid" "$BUILD_DIR/test-auth-bearer.pid" "$BUILD_DIR/test-auth-config.pid" "$BUILD_DIR/test-auth-config-yaml.pid" "$BUILD_DIR/test-auth-fixture.pid"; do
    if [[ -f "$f" ]]; then
      kill "$(cat "$f")" 2>/dev/null || true
    fi
  done
}
trap cleanup EXIT

# Start fixture server (Basic or Bearer)
(
  cd "$FIXTURE_AUTH_DIR"
  AUTH_FIXTURE_PORT="$FIXTURE_PORT" go run serve.go > "$BUILD_DIR/test-auth-fixture.log" 2>&1 & echo $! > "$BUILD_DIR/test-auth-fixture.pid"
)
sleep 1

# 1) Fail-fast: embedded credentials in target URL should be rejected
./bin/prometheus-aggregate-exporter \
  -targets="bad=http://prometheus_scraper:test-password-123@localhost:$FIXTURE_PORT/histogram.txt" \
  -server.bind=":$FAILFAST_PORT" \
  -verbose=true > "$BUILD_DIR/test-auth-failfast.log" 2>&1 & echo $! > "$BUILD_DIR/test-auth-failfast.pid"
sleep 1
curl -s "localhost:$FAILFAST_PORT/metrics" > "$BUILD_DIR/test-auth-failfast.metrics"
awk '/credentials in target URL are not allowed/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-failfast.log"

# 2) Per-target auth via -targets.config should work (basic + bearer) and allow no-auth target
./bin/prometheus-aggregate-exporter \
  -targets.config="$BUILD_DIR/test-targets-config.json" \
  -server.bind=":$CONFIG_JSON_PORT" \
  -verbose=true > "$BUILD_DIR/test-auth-config.log" 2>&1 & echo $! > "$BUILD_DIR/test-auth-config.pid"
sleep 1
curl -s "localhost:$CONFIG_JSON_PORT/metrics" > "$BUILD_DIR/test-auth-config.metrics"
awk '/^http_requests_total\{.*ae_source="secure_basic"/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-config.metrics"
awk '/^http_requests_total\{.*ae_source="secure_bearer"/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-config.metrics"
awk '/failed to fetch URL .* due to status code: 401/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-config.log"

# 3) YAML config should be accepted as well
./bin/prometheus-aggregate-exporter \
  -targets.config="$BUILD_DIR/test-targets-config.yaml" \
  -server.bind=":$CONFIG_YAML_PORT" \
  -verbose=true > "$BUILD_DIR/test-auth-config-yaml.log" 2>&1 & echo $! > "$BUILD_DIR/test-auth-config-yaml.pid"
sleep 1
curl -s "localhost:$CONFIG_YAML_PORT/metrics" > "$BUILD_DIR/test-auth-config-yaml.metrics"
awk '/^http_requests_total\{.*ae_source="secure_basic"/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-config-yaml.metrics"
awk '/^http_requests_total\{.*ae_source="secure_bearer"/ { found=1 } END { exit(found ? 0 : 1) }' "$BUILD_DIR/test-auth-config-yaml.metrics"

echo "OK: auth integration checks passed"


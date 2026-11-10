#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ROS2_DIR="$ROOT_DIR/scripts/ros2"
SIMA_ADDR="127.0.0.1:18081"

cleanup() {
  pkill -f "cmd/sima-ros2" >/dev/null 2>&1 || true
  docker compose -f "$ROS2_DIR/docker-compose.yml" down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

cd "$ROS2_DIR"
docker compose up -d --build

for i in {1..30}; do
  if curl -fsS http://127.0.0.1:8090/v1/telemetry/AUV-01 >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

cd "$ROOT_DIR"
go run ./cmd/sima-ros2 -http "$SIMA_ADDR" -bridge http://127.0.0.1:8090 >/tmp/sima-ros2.log 2>&1 &

for i in {1..30}; do
  if curl -fsS "http://$SIMA_ADDR/api/state" >/tmp/sima-ros2-state.json 2>/dev/null; then
    break
  fi
  sleep 1
done

python3 - <<'PY'
import json
from pathlib import Path
state = json.loads(Path('/tmp/sima-ros2-state.json').read_text())
assert len(state['vehicles']) >= 3, 'expected 3 vehicles'
ids = [v['snapshot']['id'] for v in state['vehicles']]
assert ids[:3] == ['AUV-01','AUV-02','AUV-03'], ids
print('PASS telemetry ingestion and identity')
PY

curl -fsS -X POST "http://$SIMA_ADDR/api/link" -H 'Content-Type: application/json' -d '{"connected":false}' >/dev/null || true

echo "PASS ros2 integration smoke (dashboard + telemetry). Logs: /tmp/sima-ros2.log"

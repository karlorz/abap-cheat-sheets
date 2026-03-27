#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-status}"
HOST_ALIAS="${HOST_ALIAS:-${2:-msi-1}}"
LOCAL_PORT="${LOCAL_PORT:-${3:-11433}}"
REMOTE_PORT="${REMOTE_PORT:-1433}"
PID_FILE="${PID_FILE:-files/dashboard/output/ptd_sql_tunnel.pid}"
LOG_FILE="${LOG_FILE:-files/dashboard/output/ptd_sql_tunnel.log}"

mkdir -p "$(dirname "$PID_FILE")"

is_listening() {
  lsof -iTCP:"$LOCAL_PORT" -sTCP:LISTEN >/dev/null 2>&1
}

find_tunnel_pids() {
  pgrep -f "ssh .*${LOCAL_PORT}:localhost:${REMOTE_PORT} ${HOST_ALIAS}" || true
}

case "$ACTION" in
  start)
    if is_listening; then
      printf 'Tunnel already listening on localhost:%s\n' "$LOCAL_PORT"
      exit 0
    fi

    : >"$LOG_FILE"
    if ssh -f -n -o ExitOnForwardFailure=yes -N -L "${LOCAL_PORT}:localhost:${REMOTE_PORT}" "$HOST_ALIAS" >"$LOG_FILE" 2>&1; then
      sleep 1
    else
      rm -f "$PID_FILE"
      printf 'Tunnel failed to start. Check %s\n' "$LOG_FILE" >&2
      exit 1
    fi

    if is_listening; then
      find_tunnel_pids | head -n 1 >"$PID_FILE"
      printf 'Tunnel started: localhost:%s -> %s:localhost:%s\n' "$LOCAL_PORT" "$HOST_ALIAS" "$REMOTE_PORT"
      exit 0
    fi

    rm -f "$PID_FILE"
    printf 'Tunnel failed to start. Check %s\n' "$LOG_FILE" >&2
    exit 1
    ;;

  stop)
    TUNNEL_PIDS="$(find_tunnel_pids)"
    if [[ -n "$TUNNEL_PIDS" ]]; then
      while IFS= read -r pid; do
        [[ -n "$pid" ]] || continue
        kill "$pid" >/dev/null 2>&1 || true
      done <<< "$TUNNEL_PIDS"
      rm -f "$PID_FILE"
      printf 'Tunnel stopped for localhost:%s\n' "$LOCAL_PORT"
      exit 0
    fi

    rm -f "$PID_FILE"
    printf 'No running tunnel found for localhost:%s\n' "$LOCAL_PORT"
    exit 0
    ;;

  status)
    if is_listening; then
      printf 'Tunnel listening on localhost:%s\n' "$LOCAL_PORT"
    else
      printf 'Tunnel not listening on localhost:%s\n' "$LOCAL_PORT"
    fi

    TUNNEL_PIDS="$(find_tunnel_pids)"
    if [[ -n "$TUNNEL_PIDS" ]]; then
      printf 'Active tunnel pids:\n%s\n' "$TUNNEL_PIDS"
    elif [[ -f "$PID_FILE" ]]; then
      printf 'Pid file: %s\n' "$PID_FILE"
      printf 'Recorded pid: %s\n' "$(cat "$PID_FILE")"
    fi
    ;;

  *)
    printf 'Usage: %s {start|stop|status} [host_alias] [local_port]\n' "$0" >&2
    exit 1
    ;;
esac

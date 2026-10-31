#!/usr/bin/env bash
# /usr/bin/qtun-routing — apply iptables + auto-revert saat app mati
#
# Commands:
#   start            apply rules (butuh app running)
#   stop             hapus rules
#   check            one-shot: hapus rules jika app mati
#   watch [SEC]      daemon loop: apply saat app up, revert saat app down
#   status           tampilkan state
#
# Compatibel bash & busybox-ash (POSIX).
set -u

# ---- Konfigurasi ----
CHAIN="${QTUN_CHAIN:-QTUN}"
REDIRECT_PORT="${QTUN_REDIRECT_PORT:-7892}"
API_PORT="${QTUN_API_PORT:-9090}"
LAN_IF="${QTUN_LAN_IF:-auto}"
APP_NAME="${QTUN_APP_NAME:-sockshttp}"
PIDFILE="${QTUN_PIDFILE:-/var/run/sockshttp.pid}"
CONF="${QTUN_CONF:-/etc/config/sockshttp.yaml}"
LOG_TAG="qtun-routing"

log() { logger -t "$LOG_TAG" "$*" 2>/dev/null; echo "[$LOG_TAG] $*"; }

app_running() {
    if [ -f "$PIDFILE" ]; then
        pid=$(cat "$PIDFILE" 2>/dev/null)
        if [ -n "${pid:-}" ] && kill -0 "$pid" 2>/dev/null; then
            return 0
        fi
    fi
    pidof "$APP_NAME" >/dev/null 2>&1 && return 0
    return 1
}

chain_exists() { iptables -t nat -L "$CHAIN" -n >/dev/null 2>&1; }

detect_lan_if() {
    case "$LAN_IF" in
        auto|"")
            if ip link show br-lan >/dev/null 2>&1; then
                LAN_IF="br-lan"
            else
                LAN_IF=$(ip -o link show up 2>/dev/null \
                    | awk -F': ' '$2!="lo" && $2!~"^wlan"{print $2; exit}')
                [ -z "${LAN_IF:-}" ] && LAN_IF="eth0"
            fi
            ;;
    esac
}

server_ip() {
    [ -f "$CONF" ] || return
    srv=$(awk '/^server:/ {print $2; exit}' "$CONF" 2>/dev/null | tr -d "\"'")
    [ -z "$srv" ] && return
    if echo "$srv" | grep -qE '^[0-9.]+$'; then
        echo "$srv"; return
    fi
    nslookup "$srv" 2>/dev/null | awk '
        /^Address: /        {print $2; exit}
        /^Address [0-9]+:/ {print $3; exit}
    '
}

apply_rules() {
    detect_lan_if

    # Bersihkan / buat chain
    iptables -t nat -N "$CHAIN" 2>/dev/null \
        || iptables -t nat -F "$CHAIN"

    # Bypass jaringan privat
    for net in 0.0.0.0/8 10.0.0.0/8 127.0.0.0/8 169.254.0.0/16 \
               172.16.0.0/12 192.168.0.0/16 224.0.0.0/4 240.0.0.0/4; do
        iptables -t nat -A "$CHAIN" -d "$net" -j RETURN
    done

    # Bypass IP server VPN
    srv_ip=$(server_ip)
    if [ -n "${srv_ip:-}" ]; then
        iptables -t nat -A "$CHAIN" -d "$srv_ip" -j RETURN
        log "bypass server $srv_ip"
    fi

    # Bypass API port
    iptables -t nat -A "$CHAIN" -p tcp --dport "$API_PORT" -j RETURN

    # Redirect TCP ke listener
    iptables -t nat -A "$CHAIN" -p tcp \
        -j REDIRECT --to-ports "$REDIRECT_PORT"

    # Hook ke OUTPUT & PREROUTING (idempotent)
    iptables -t nat -C OUTPUT -p tcp -j "$CHAIN" 2>/dev/null \
        || iptables -t nat -A OUTPUT -p tcp -j "$CHAIN"
    iptables -t nat -C PREROUTING -i "$LAN_IF" -p tcp -j "$CHAIN" 2>/dev/null \
        || iptables -t nat -A PREROUTING -i "$LAN_IF" -p tcp -j "$CHAIN"

    log "routing applied (lan=$LAN_IF → :$REDIRECT_PORT)"
}

remove_rules() {
    detect_lan_if
    iptables -t nat -D OUTPUT -p tcp -j "$CHAIN" 2>/dev/null
    iptables -t nat -D PREROUTING -i "$LAN_IF" -p tcp -j "$CHAIN" 2>/dev/null
    iptables -t nat -F "$CHAIN" 2>/dev/null
    iptables -t nat -X "$CHAIN" 2>/dev/null
    log "routing removed"
}

# -------------------------------------------------------------
case "${1:-}" in
    start)
        if ! app_running; then
            log "ERROR: $APP_NAME tidak berjalan, skip apply"
            exit 1
        fi
        apply_rules
        ;;
    stop)
        remove_rules
        ;;
    check)
        if chain_exists && ! app_running; then
            log "app mati tapi chain $CHAIN masih ada → cleanup"
            remove_rules
        fi
        ;;
    watch)
        interval="${2:-5}"
        log "watcher start (interval ${interval}s) pid=$$"
        while true; do
            if app_running; then
                chain_exists || apply_rules
            else
                if chain_exists; then
                    log "app $APP_NAME mati → restore routing"
                    remove_rules
                fi
            fi
            sleep "$interval"
        done
        ;;
    status)
        if chain_exists; then echo "chain: PRESENT"; else echo "chain: ABSENT"; fi
        if app_running; then echo "app: RUNNING";  else echo "app: STOPPED"; fi
        ;;
    *)
        cat <<EOF
Usage: $0 {start|stop|check|watch [SEC]|status}
Env overrides: QTUN_CHAIN QTUN_REDIRECT_PORT QTUN_API_PORT QTUN_LAN_IF
               QTUN_APP_NAME QTUN_PIDFILE QTUN_CONF
EOF
        exit 1
        ;;
esac
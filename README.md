# SocksHTTP-Go (ARM64)

SSH tunnel → transparent proxy untuk OpenWrt / Linux ARM64.

## Struktur
- `main.go` — app
- `sockshttp.yaml` — config
- `scripts/init.d/sockshttp` — OpenWrt procd
- `scripts/action/routing.sh` — iptables + watchdog

## Local build
```bash
make tidy
make arm64
ls -lh dist/sockshttp-linux-arm64
```

## CI (GitHub Actions)
Push ke `main` → artifact `sockshttp-linux-arm64.tar.gz` di tab **Actions**.

## Deploy ke router ARM64
```sh
tar xzf sockshttp-linux-arm64.tar.gz -C /tmp
mv /tmp/sockshttp-linux-arm64  /usr/bin/sockshttp
mv /tmp/qtun-routing           /usr/bin/qtun-routing
mv /tmp/init.d-sockshttp       /etc/init.d/sockshttp
mv /tmp/sockshttp.yaml         /etc/config/sockshttp.yaml
chmod +x /usr/bin/sockshttp /usr/bin/qtun-routing /etc/init.d/sockshttp

/etc/init.d/sockshttp enable
/etc/init.d/sockshttp start
```

## Cek status
```sh
/usr/bin/qtun-routing status
iptables -t nat -L QTUN -n -v
curl http://127.0.0.1:9090/api/status
```

## Matikan & restore routing
```sh
/etc/init.d/sockshttp stop   # watchdog di-kill, rules dihapus
```

## Catatan `bash` di OpenWrt
`routing.sh` pakai `#!/usr/bin/env bash` tapi ditulis POSIX-compatible.
Jika `bash` tidak tersedia: `opkg install bash`, atau ganti shebang ke
`#!/bin/sh` (sudah kompatibel busybox-ash).
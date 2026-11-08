<div align="center">
  <h1>⚡ SocksHTTP-Go (ARM64 / Linux)</h1>
  <p><b>SSH Tunnel • Transparent Proxy • Fake-IP DNS • Bandwidth Limiter</b></p>

  [![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go)](https://go.dev/)
  [![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20OpenWrt-lightgrey?style=for-the-badge&logo=linux)](https://openwrt.org/)
  [![License](https://img.shields.io/badge/License-MIT-blue?style=for-the-badge)](LICENSE)
</div>

---

**SocksHTTP-Go** adalah aplikasi *tunneling* super ringan yang dirancang khusus untuk router **OpenWrt / Linux ARM64**. Aplikasi ini membungkus koneksi SSH menjadi *Transparent Proxy* (iptables REDIRECT) dan *SOCKS5 Proxy*, dilengkapi dengan sistem **Fake-IP DNS**, **Routing per-Device**, **Web Dashboard Modern**, dan **Bandwidth Limiter per-IP**.

---

## ✨ Fitur Utama

- 🚀 **Transparent Proxy (REDIRECT)** — Otomatis membelokkan lalu lintas TCP dari perangkat di jaringan lokal (LAN) tanpa perlu setting proxy manual di HP/TV.
- 🔀 **SOCKS5 Proxy** — Tersedia di port `7776` untuk klien yang ingin menggunakan proxy secara manual.
- 🌐 **Fake-IP DNS** — Resolusi DNS super cepat tanpa *leak*. Domain diubah menjadi IP `198.18.x.x` dan diterjemahkan kembali saat koneksi keluar.
- ⏱ **Bandwidth Limiter per-IP** — Batasi kecepatan Upload & Download per perangkat (dalam Kbps). Mendukung *Live-Apply* (berlaku instan meski koneksi sedang berjalan).
- 🎨 **Web Dashboard Keren** — UI modern (Glassmorphism) untuk memantau status, melihat koneksi aktif (Live Speed), mengatur proxy group, melihat log, dan mengelola limiter.
- 🛡 **Rule-Based Routing** — Bebaskan traffic tertentu (Bypass LAN) dan arahkan device tertentu ke grup proxy yang berbeda (`SRC-IP-CIDR`).
- 🐶 **Auto-Reconnect & Watchdog** — Sistem *health-check* otomatis yang memaksa reconnect jika tunnel SSH terputus.

---

## 🏗️ Arsitektur Alur Data

```text
[ Device LAN ] ──(TCP)──▶ [ iptables REDIRECT ] ──▶ [ :7893 Router ] ──▶ [ SSH Tunnel ] ──▶ [ VPS/Server ]
                                │
                        [ Fake-IP DNS ]
                       (Resolusi Domain)
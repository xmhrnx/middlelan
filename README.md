# MiddleMan

**See every connection. Know every device.**

MiddleMan is a cross-platform network monitor built with [Giode](https://github.com/xmhrnx/giode)
(Go + Gio, GPU-rendered, no webview). It watches every app's network connections, shows where
each one connects (host, owner, country, map), finds the devices on your local network, and
includes a full toolbox of network diagnostics.

Platforms: **Windows, macOS, Linux, Android** (one codebase).

## Features

### Live monitoring
- **Per-app traffic**: live download/upload rates, today's totals and session totals for every
  program, plus a 7-day history.
- **Where apps connect**: every remote endpoint per app, with the **domain name the app looked
  up** (from the Windows DNS cache, packet capture as administrator on Linux/macOS, or the
  Android VPN; reverse DNS otherwise), owning company/network (Google, AWS, Cloudflare…),
  country and city. Destinations are remembered across sessions.
- **Connections view**: every open socket on the computer, filterable by internet, local and
  listening, with search by app, address, host or port.
- **World map**: a dot-matrix map with arcs from your location to every destination, plus
  breakdowns by country and network.
- **Listening ports** per app, flagged when reachable from the network.
- **Alerts** when a program connects to the network for the first time.
- **History**: hourly usage for 14 days and daily for 90, with week-over-week comparison;
  click any hour or day to see which apps used the network then.
- **Export** usage and destinations as CSV, or everything as JSON.

### Security
- **Threat alerts**: destinations are checked offline against free blocklists (abuse.ch Feodo
  and URLhaus, Spamhaus DROP, FireHOL level 1), refreshed daily.
- **Firewall, off by default**: block a program, or an address, with a click (Windows Firewall,
  nftables on Linux, pf on macOS, the VPN on Android). It never changes your system's firewall
  defaults or turns the system firewall on or off; it only adds the rules you create, and
  turning it off, or the next start after a crash, removes every rule it added.
- **Ask before new programs connect**: optional, while the firewall is on: Allow or Block when
  a program first uses the network.
- **On-device VPN on Android**: relays traffic on the phone itself (no server) to show every
  app's connections and domain names, and enforce firewall rules.

### Runs quietly in the background
- Closes to the system tray / macOS menu bar and keeps monitoring. The tray menu is a mini panel:
  live speeds, today's total and the busiest apps, plus pause and quit.
- **Android home screen widget** with live speeds, today's usage and the busiest app.
- Optional **start in the tray at login** (Windows Run key, macOS LaunchAgent, Linux autostart).
- Right-click (long-press on Android) an app, connection, destination or device for its actions:
  copy the address or name, ping, traceroute, WHOIS, scan ports, wake, or block it.
- Keyboard shortcuts: Ctrl/⌘+1–6 switch pages, Ctrl/⌘+F search, Ctrl/⌘+P pause, Ctrl/⌘+, settings, F5 refresh.
- Sampling every 2 s while the window is shown and every 6 s while hidden. When idle it draws
  about one frame per sample, and per-socket counters come straight from the kernel, so it
  doesn't poll processes.
- Android: an optional foreground service keeps monitoring in the background.

### Devices (LAN scanner)
- Finds devices by ICMP/TCP probes and the ARP table, then names them with reverse DNS,
  NetBIOS and mDNS. Each device gets its MAC maker (54k-entry OUI database, offline) and a
  device type (router, phone, computer, TV, printer, camera…).
- Remembers devices, lets you name them and mark them as known, and **alerts when a new device
  joins**. It can rescan in the background every 5–60 minutes.
- **Presence timeline** per device: when it joined and left the network.
- **Whole-network traffic** from the router's UPnP counters, every device included.

### Tools
Speed test with history, optional scheduled tests and an **ISP report card** graded against
your plan · Wi-Fi analyzer (signal, nearby networks, channel congestion, weak security) ·
Internet uptime monitor (outage log + notifications) · Router security check ·
UPnP & Bonjour service discovery · Ping · Traceroute · Port scanner · DNS lookup · WHOIS ·
TLS certificate inspector · Wake on LAN · Network info (interfaces, gateway, DNS, public IP,
ISP) · Subnet calculator.

### Design
Dark and light themes in graphite and champagne gold, the Inter typeface, and a custom title bar on desktops (traffic lights on
macOS) instead of the OS frame. Layouts are responsive: a sidebar on desktops and a bottom
navigation bar on phones.
In English, Español, Deutsch, Français, Português and فارسی (right to left), following the system language or the one chosen in Settings → Appearance (translations live in `locales/`).

## How it sees connections

| Platform | Connections and owners | Per-app bytes |
| --- | --- | --- |
| Linux | netlink `sock_diag` (falls back to `/proc/net`), owners from `/proc/*/fd` | per-socket `tcp_info` counters, no root needed |
| Windows | `GetExtendedTcpTable` / `GetExtendedUdpTable` | per-connection extended stats (needs **Run as administrator**, offered in-app) |
| macOS | `lsof` | `nettop` per-process counters |
| Android | Android 10+ hides other apps' sockets; MiddleMan's on-device VPN (Security) shows them | `NetworkStatsManager` per-app usage (grant **Usage access**, offered in-app), split by destination with the VPN |

Without administrator rights, desktops hide the program names of other users' and system
sockets; MiddleMan labels those sockets *hidden* and offers to restart elevated.

**Updates:** desktop builds check GitHub Releases daily (can be turned off). With a signed
update feed (`giode update-key`, then `[update]` in giode.toml) they also install updates.

**Privacy:** usage history stays on the device (90 days). Locating remote IPs uses
[geojs.io](https://www.geojs.io/) and can be switched off in Settings.

## Building

You need Go 1.26+ and the Giode CLI:

```sh
go install github.com/xmhrnx/giode-cli/cmd/giode@latest
giode doctor            # what this computer still needs per platform
giode run               # run on this computer
giode run android       # build, install and launch on a connected phone
giode build linux windows deb appimage windows-installer
giode build android aab # needs the Android SDK, NDK and a JDK
giode build macos dmg   # on a Mac with Xcode
```

`giode ci` already wrote `.github/workflows/release.yml`. Pushing a tag such as `v1.0.0`
builds and signs every platform and attaches the packages to a GitHub release.

## Project layout

```
main.go, tray.go, keepalive.go   wiring: window, tray, background services
assets/                          icons (app, adaptive Android layers, tray), fonts, map data, UI icons
  src/make_icons.py              regenerates every icon from SVG
android/java/                    Java helpers: usage stats, VPN service, Wi-Fi, widget, permission prompt
android/aar/                     extra manifest entries and resources, packed by build-aar.sh into android/libs/
locales/                         translations (English text is the key)
internal/monitor                 connection sources per OS, collector, usage store (hourly and daily)
internal/geo                     reverse DNS, IP geolocation, owner names (cached, batched)
internal/dnslog                  domain names apps look up (DNS cache / packet capture)
internal/nettools                ping, traceroute, port scan, DNS, WHOIS, LAN scan, OUI, SSDP, mDNS, speed test…
internal/inventory               known devices, presence timeline, background scanner
internal/firewall                block rules: Windows Firewall, nftables, pf, Android VPN
internal/threat, internal/alerts blocklists and the alerts they raise
internal/vpn                     Android on-device VPN (gVisor netstack)
internal/wifi                    Wi-Fi link, scans, channel congestion
internal/router                  whole-network traffic over UPnP IGD
internal/speedlog                speed test history and ISP report card
internal/export, internal/updates  CSV/JSON export; update checks and signed self-update
internal/settings                preferences
ui/                              Giode UI: shell, screens, components (one per file)
```

## Icon

The emblem is the man in the middle: a gold figure between two linked devices, on graphite, the
point every packet passes through. On Android it ships as an adaptive icon with a full-bleed
background layer and the emblem inside the 66 dp safe zone, so launchers never draw a white
frame around it. There is also a monochrome layer for Android 13 themed icons.

## Licenses

Code: Apache-2.0 (see `LICENSE`). Inter font: SIL OFL (`assets/fonts/LICENSE-Inter.txt`).
UI icons: Lucide, ISC (`assets/icons-LICENSE.txt`). Map: Natural Earth (public domain).
MAC vendors: IEEE OUI registry via `oui-data`.

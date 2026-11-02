# MiddleLAN

**See every connection. Know every device.**

MiddleLAN is a cross-platform network monitor built with [Giode](https://github.com/mehran9675/giode)
(Go + Gio, GPU-rendered, no webview). It watches every app's network connections, shows where
each one connects (host, owner, country, map), finds the devices on your local network, and
includes a full toolbox of network diagnostics.

Platforms: **Windows, macOS, Linux, Android** (one codebase).

## Features

### Live monitoring
- **Per-app traffic**: live download/upload rates, today's totals and session totals for every
  program, plus a 7-day history.
- **Where apps connect**: every remote endpoint per app, with reverse-DNS host name, owning
  company/network (Google, AWS, Cloudflare…), country and city. Destinations are remembered
  across sessions.
- **Connections view**: every open socket on the computer, filterable by internet, local and
  listening, with search by app, address, host or port.
- **World map**: a dot-matrix map with arcs from your location to every destination, plus
  breakdowns by country and network.
- **Listening ports** per app, flagged when reachable from the network.
- **Alerts** when a program connects to the network for the first time.

### Runs quietly in the background
- Closes to the system tray / macOS menu bar and keeps monitoring. The tray menu can pause it
  and shows live rates in the tooltip.
- Optional **start hidden at login** (Windows Run key, macOS LaunchAgent, Linux autostart).
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

### Tools
Speed test · Internet uptime monitor (outage log + notifications) · Router security check ·
UPnP & Bonjour service discovery · Ping · Traceroute · Port scanner · DNS lookup · WHOIS ·
TLS certificate inspector · Wake on LAN · Network info (interfaces, gateway, DNS, public IP,
ISP) · Subnet calculator.

### Design
Dark and light themes, the Inter typeface, and a custom title bar on desktops (traffic lights on
macOS) instead of the OS frame. Layouts are responsive: a sidebar on desktops and a bottom
navigation bar on phones.

## How it sees connections

| Platform | Connections and owners | Per-app bytes |
| --- | --- | --- |
| Linux | netlink `sock_diag` (falls back to `/proc/net`), owners from `/proc/*/fd` | per-socket `tcp_info` counters, no root needed |
| Windows | `GetExtendedTcpTable` / `GetExtendedUdpTable` | per-connection extended stats (needs **Run as administrator**, offered in-app) |
| macOS | `lsof` | `nettop` per-process counters |
| Android | Android 10+ hides other apps' sockets from apps | `NetworkStatsManager` per-app usage (grant **Usage access**, offered in-app) |

Without administrator rights, desktops hide the program names of other users' and system
sockets; MiddleLAN labels those sockets *hidden* and offers to restart elevated.

**Privacy:** usage history stays on the device (90 days). Locating remote IPs uses
[geojs.io](https://www.geojs.io/) and can be switched off in Settings.

## Building

You need Go 1.26+ and the Giode CLI:

```sh
go install github.com/mehran9675/giode-cli/cmd/giode@latest
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
android/java/                    NetStats.java: per-app usage on Android
internal/monitor                 connection sources per OS, collector, usage store
internal/geo                     reverse DNS, IP geolocation, owner names (cached, batched)
internal/nettools                ping, traceroute, port scan, DNS, WHOIS, LAN scan, OUI, SSDP, mDNS, speed test…
internal/inventory               known devices, new-device detection, background scanner
internal/settings, autostart     preferences, start at login
ui/                              Giode UI: shell, screens, components (one per file)
```

## Icon

The emblem is an "M" whose strokes meet in a glowing node in the middle: the man in the middle
that every packet passes through. On Android it ships as an adaptive icon with a full-bleed
background layer and the emblem inside the 66 dp safe zone, so launchers never draw a white
frame around it. There is also a monochrome layer for Android 13 themed icons.

## Licenses

Code: Apache-2.0 (see `LICENSE`). Inter font: SIL OFL (`assets/fonts/LICENSE-Inter.txt`).
UI icons: Lucide, ISC (`assets/icons-LICENSE.txt`). Map: Natural Earth (public domain).
MAC vendors: IEEE OUI registry via `oui-data`.

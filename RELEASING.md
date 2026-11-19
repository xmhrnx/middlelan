# Releasing MiddleMan

Pushing a tag like `v1.0.0` (or "Run workflow" in the Actions tab) runs
`.github/workflows/release.yml`. The tag sets the version, and a four-part tag
(`v1.0.0.7`) also sets the build number; otherwise the build number is the
run number. The workflow runs `go test ./...`, builds every package and, for a
tag, attaches them to the GitHub release.

The workflow is written by the Giode CLI from `giode.toml`. After changing
`[ci]` or a store section, run `giode ci -f`, then redo the three hand edits
listed in `[ci]` in `giode.toml`:

1. `go install github.com/xmhrnx/giode-cli/cmd/giode@v0.1.35-alpha.1` instead of `@latest`;
2. `go-version-file: go.mod` instead of `go-version: stable`;
3. in "Build Windows Android", build the APK first, then the Play bundle
   (`aab`) with `tags = "playstore"` added to `giode.toml` for that build only
   (see [Google Play](#google-play)).

`.github/workflows/android-vpn-check.yml` is separate: it runs the app in an
Android emulator and turns the on-device VPN on. Keep it.

| Where | What the release gets | Needs |
| --- | --- | --- |
| GitHub, Linux | `.deb` and `.rpm`, x64 | nothing |
| GitHub, Windows | `MiddleMan-<version>-amd64-setup.exe` | nothing; a code signing certificate stops SmartScreen warnings |
| GitHub, macOS | a universal `.dmg` | a Developer ID certificate and notarization, or macOS refuses to open it |
| GitHub, Android | `.apk` (with ad blocking) | the release keystore, or it's signed with a debug key |
| Microsoft Store | `MiddleMan-<version>-amd64.msix` to upload in Partner Center | a Partner Center account and the reserved name |
| Google Play | `.aab` (no ad blocking) to upload in Play Console | a Play Console account, the release keystore and the VpnService declaration |
| Mac App Store | nothing: **not offered**, see below | |
| Apple App Store (iOS) | nothing: MiddleMan doesn't target iOS | |

There is no Mac App Store package. The App Sandbox every store app runs in
breaks what MiddleMan is for:

- **Connections:** on macOS MiddleMan reads every app's sockets with `lsof`
  and per-app bytes with `nettop`. In the sandbox, those tools (which inherit
  the app's sandbox) can't inspect other processes, so the app would show
  little beyond its own connections.
- **Firewall and domain names:** blocking uses `pf` and domain names come
  from packet capture (`/dev/bpf`); both need root, which MiddleMan gets with
  the administrator prompt ("Run as administrator"). App Review guideline
  2.4.5 forbids Mac App Store apps from asking for root.

The notarized `.dmg` has none of these limits. Keep it the only macOS
download unless the macOS side is rewritten around a Network Extension (a
separate, entitlement-gated project).

iOS is not a target (`app.platforms`), and couldn't be one without a rewrite:
iOS shows no app another app's connections or lets it scan for them.

## Repository secrets

Set these in GitHub under Settings → Secrets and variables → Actions. Each
`*_BASE64` secret is the file encoded with `base64 -i file | pbcopy` on macOS,
or `base64 -w0 file` on Linux. Without a secret, that step is skipped (or
builds unsigned) and the rest of the release still builds.

| Secret | What it is |
| --- | --- |
| `ANDROID_KEYSTORE_BASE64`, `GIODE_KEYSTORE_PASS` | The Android upload keystore (`release.keystore`) and its password: signs the `.apk` and `.aab` |
| `MACOS_CERT_BASE64`, `MACOS_CERT_PASS` | The "Developer ID Application" certificate as a `.p12`, and its password: signs the `.dmg` |
| `GIODE_NOTARY_ID`, `GIODE_NOTARY_TEAM_ID`, `GIODE_NOTARY_PASS` | Your Apple ID, team ID and an app-specific password: notarizes the `.dmg` |
| `WINDOWS_CERT_BASE64`, `GIODE_WINDOWS_CERT_PASS` | Optional. An Authenticode `.pfx` and its password, for the setup program. Also set `certificate = "codesign.pfx"` under `[windows.signing]`. Leave both unset while the `.msix` goes to the Store: `-release` would sign the MSIX too, with a subject that doesn't match the Store publisher. |

`release.keystore` (and `*.keystore`, `*.jks`) is in `.gitignore`. `giode.toml`
sets `keystore = "release.keystore"` under `[android]` even though the file
only exists in CI: `giode build -release` fails without `android.keystore`,
so signing with `GIODE_KEYSTORE_PASS` needs it.

## Google Play

### Set up once

1. Open a Google Play Console developer account.
2. Create the upload keystore with `giode keystore` (it writes
   `release.keystore`; keep a backup and the password somewhere safe). Add it
   and its password as `ANDROID_KEYSTORE_BASE64` and `GIODE_KEYSTORE_PASS`.
3. In Play Console, create the app "MiddleMan" (app, free). Package name:
   `com.middlelan.app` (`app.id`); it can never change.
4. Publish a release with a tag. Upload `MiddleMan.aab` from the release to an
   internal testing track first. Accept **Play App Signing**: Google keeps the
   app signing key and `release.keystore` becomes the upload key. (The `.apk`
   on GitHub is signed with the upload key, so Play and GitHub installs can't
   update each other.)
5. Raise `build` (or use a four-part tag) for every upload: Play rejects a
   version code it has seen.

### Target SDK

`target_sdk = 36`: Google Play requires Android 16 (API 36) for new apps and
updates since 31 Aug 2026, and the CLI's default (35) is rejected. Android 16
no longer calls `onBackPressed` for apps targeting it, which is how Gio's
activity handles Back, so `[android]` sets
`application_attributes = { enableOnBackInvokedCallback = "false" }` (opting
out of predictive back). **Before each Play release, test the Back gesture on
an Android 16 device or emulator:** it must go back inside MiddleMan (close a
dialog, leave an app's detail page) rather than close the app.

### Policy declarations (App content)

MiddleMan uses sensitive Android APIs. Answer these in Play Console → Policy →
App content:

- **VpnService declaration (required).** MiddleMan's VPN is a local,
  on-device VPN for network monitoring and a per-app firewall (the
  "device security: firewall" and "network related tools" uses). Suggested
  answer: "MiddleMan's core function is showing which apps use the network and
  where they connect, and letting the user block them. Android 10+ hides other
  apps' sockets, so MiddleMan runs a local VpnService (like NetGuard and
  PCAPdroid): a userspace TCP/IP stack on the phone ends each connection and
  re-makes it from MiddleMan's own sockets, straight to the original
  destination. There is no remote VPN server and traffic never leaves the
  device through a tunnel, so there is no tunnel endpoint to encrypt to; the
  apps' own encryption (TLS) is untouched, MiddleMan never decrypts, stores or
  modifies traffic contents, and doesn't redirect traffic or touch ads. The
  user turns the VPN on in Security, after an in-app explanation and Android's
  VPN consent dialog." Record a short video: Security → On-device VPN →
  consent → the Connections list filling in.
- **Ad blocking is not in the Play build.** Play's VpnService declaration says
  any manipulation of ads that can impact monetization is rejected, and the
  Device and Network Abuse policy doesn't allow blocking other apps' ads. The
  Play bundle is built with the `playstore` Go build tag (`adblock_off.go`),
  which leaves the ad and tracker blocking card and filter out; the GitHub APK
  keeps it. Never upload the GitHub `.apk` to Play.
- **Foreground services.** Declare two types:
  - `specialUse` for `MiddleVpnService` (the on-device VPN; the manifest's
    subtype text says so);
  - `dataSync` for giode's `KeepAliveService`, which "Keep monitoring in the
    background" starts (off by default). Describe it as keeping usage
    sampling and alerts running while the app is in the background. Risk:
    `dataSync` is meant for transfers, Google may question it, and Android 15+
    stops a `dataSync` service after 6 hours a day. The type is fixed by the
    Giode CLI (v0.1.35), not `giode.toml`; if Play rejects it, the option has
    to go from the Play build or the CLI must allow `specialUse` there.
- **Usage access** (`PACKAGE_USAGE_STATS`) needs no form; the in-app button
  opens Settings for the user to grant it.
- **Location** (`ACCESS_FINE_LOCATION`, foreground only) is for Wi-Fi names and
  scans, which Android only shows to apps with location; no background
  location, so no location declaration.
- **Package visibility:** no `QUERY_ALL_PACKAGES`; a `<queries>` launcher
  intent names apps, so no declaration.
- **Data safety:** see below. **Ads:** no. **Target audience:** 18+ (a
  technical tool; not designed for children). **Government / financial /
  health:** no.

### Store listing (ready to paste)

- **App name:** MiddleMan
- **Short description (80 max):** See which apps use your network, where they connect, and every device on Wi-Fi.
  (79 characters)
- **Full description:**

  > MiddleMan shows what your phone does on the network: which apps use it, how
  > much, and where each one connects.
  >
  > • Per-app traffic: live speeds, today's totals and a 7-day history (Usage access).
  > • Where apps connect: every destination with the domain name the app looked up, the owner (Google, AWS, Cloudflare…), country and city, on a world map.
  > • Firewall: block an app or an address with a tap. Optionally ask before a new app connects.
  > • Threat alerts: destinations are checked offline against free blocklists (abuse.ch, Spamhaus DROP, FireHOL).
  > • Devices: finds the devices on your Wi-Fi, names them, shows their maker and type, and tells you when a new one joins.
  > • Tools: speed test with an ISP report card, Wi-Fi analyzer, uptime monitor, router check, ping, traceroute, port scanner, DNS lookup, WHOIS, TLS certificate inspector, Wake on LAN, subnet calculator.
  > • Home screen widget, dark and light themes, six languages.
  >
  > How MiddleMan uses the VPN: Android hides other apps' connections, so
  > MiddleMan can run a local VPN (VpnService), only when you turn it on. It
  > relays traffic on the phone itself, straight to where each app was going,
  > so MiddleMan can show which app connects where, capture domain names and
  > apply your firewall rules. There is no VPN server, nothing is sent to us,
  > and MiddleMan never decrypts or stores what apps send.
  >
  > No account, no ads, no tracking. Locating server addresses uses geojs.io
  > and can be turned off. Open source (Apache-2.0).

- **Category:** Tools. **Tags:** network monitor, firewall, Wi-Fi.
- **Contact:** the GitHub issues page; **website:** https://xmhrnx.github.io/middlelan/
- **Privacy policy:** https://xmhrnx.github.io/middlelan/#privacy
- **Content rating (IARC):** Utility; no violence, sexuality, language,
  gambling, user interaction or sharing of location. Expect "Everyone" / PEGI 3.

### Data safety answers

- **Does the app collect or share user data?** Yes, because geojs.io and
  ipinfo.io receive IP addresses when online lookups are on (a default that
  can be switched off). Answer:
  - **Web browsing history:** the addresses of the servers apps connect to
    are sent to geojs.io for geolocation. Declare it **shared** with a third
    party (not collected by the developer), purpose **App functionality**,
    **optional** (Settings → online lookups), not linked to an identity.
  - Everything else (usage history, destinations, devices, Wi-Fi names,
    location) stays on the device: **not collected**.
- **Encrypted in transit:** yes (all of those requests are HTTPS).
- **Deletion:** no account and nothing held by the developer; uninstalling
  removes all data.
- If Google pushes back on "shared", the conservative alternative is to keep it:
  over-declaring is allowed, under-declaring isn't.

### Screenshots

Phone (at least 2, 1080×1920 or 1080×2400): Overview with live speeds;
Connections (apps and destinations); the world map; Devices; Security with the
on-device VPN card (shows the VPN disclosure); Tools / speed test. Take them
from the Play build (no ad-blocking card). A 1024×500 feature graphic: the icon
on graphite with "See every connection. Know every device."

## Microsoft Store

1. Use the Partner Center account of EasySend and AnyFile.
2. Reserve the name "MiddleMan" (if taken, reserve a variant and set the
   display name the same way AnyFile does).
3. On Product identity, check the values match `[windows.msix]`:
   - **Package/Identity/Name** → `identity_name` (`MehranZahir.MiddleMan` is
     the expected value; correct it if Partner Center shows another).
   - **Package/Identity/Publisher** → `publisher`
     (`CN=B40E8F57-A7E0-40BB-8E74-5D051D0B578B`).
   - **Package/Properties/PublisherDisplayName** → `publisher_display_name`
     (`Mehran Zahiroddini`).
4. Commit, tag, and upload `MiddleMan-<version>-amd64.msix` from the release
   in a Partner Center submission. The Store signs it, so it needs no
   certificate here.
5. **runFullTrust:** MiddleMan is a full-trust desktop app. Explain: "MiddleMan
   is a network monitor: it reads the system's TCP/UDP connection tables and
   per-connection statistics, and adds the user's outbound block rules to
   Windows Defender Firewall."
6. Listing: same name, short description and full description as Google Play
   (drop the VPN paragraph; say instead that per-app byte counts and the
   firewall need "Run as administrator"). Category: **Utilities & tools**,
   subcategory none. Privacy policy URL as above. Age rating questionnaire: no
   objectionable content (IARC 3+). Screenshots at 1920×1080 or larger:
   Overview, Connections, map, Devices, Security (firewall), Tools.

**Known limits of the Store version:**

- **Run as administrator.** Per-app byte counts, other users' and system
  programs' names, and the firewall need administrator rights; MiddleMan
  restarts itself elevated through UAC (`ShellExecute runas`). MSIX packages
  can only run elevated when the manifest declares the restricted
  `allowElevation` capability, which Giode CLI v0.1.35 doesn't write. Test
  "Run as administrator" in the installed Store package. If it fails, the
  Store version runs as a standard user (connections of your own programs,
  no firewall) and the notice should point to the setup program on GitHub.
- **Updates:** a Store copy doesn't check GitHub for updates (the Store
  updates it); the setting is hidden there.
- **Start at login** uses the package's startup task (`startup_task = true`);
  Windows lists it under Settings → Apps → Startup.

## macOS (GitHub only)

1. Join the Apple Developer Program.
2. Create a "Developer ID Application" certificate, export it as a `.p12`, and
   add `MACOS_CERT_BASE64` and `MACOS_CERT_PASS`.
3. Create an app-specific password for your Apple ID and add
   `GIODE_NOTARY_ID`, `GIODE_NOTARY_TEAM_ID` and `GIODE_NOTARY_PASS`.
4. Publish a release: it gets a signed, notarized
   `MiddleMan-<version>-universal.dmg`.

macOS 15 asks before MiddleMan reaches the local network (the device scan);
the prompt shows `[macos.usage] local_network`. `ITSAppUsesNonExemptEncryption`
and `[macos.store]` are left out on purpose: there is no store build.

## Linux

The `.deb` and `.rpm` are attached to the release, and the website's download
button points at the release. They list the system libraries they need, so
`apt` and `dnf` install those too.

## Before each release

- `gofmt -l .`, `go vet ./...`, `go test ./...`, and
  `go vet -tags playstore .` (the Play build).
- `giode build linux msix` and `giode manifest android` locally.
- On Android 16: the Back gesture, the VPN consent, and that the Play build
  has no ad-blocking card.
- If the privacy behavior changed (a new service, a new permission), update
  the website's `#privacy` section and the Data safety answers above.

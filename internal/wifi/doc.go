// Package wifi reports the Wi-Fi link MiddleLAN's device is connected to
// and the networks around it: signal, channel, band, security and link
// rates, plus helpers that flag weak security and summarize channel
// congestion.
//
// Each platform uses what it offers without extra privileges: netsh on
// Windows, nmcli or iw on Linux, system_profiler (and the legacy airport
// tool where it still exists) on macOS, and WifiManager through a small
// Java helper on Android. All parsing lives in pure functions so it can be
// tested with captured output on any OS.
package wifi

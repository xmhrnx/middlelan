//go:build mcpdebug

package main

// debugMCP turns on giode's automation server (127.0.0.1:7600), for the
// emulator test in .github/workflows/android-vpn-check.yml. Release builds
// never have this tag.
const debugMCP = true

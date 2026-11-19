//go:build !playstore

package main

// adBlockBuilt includes ad and tracker blocking (Android's on-device VPN).
// The Google Play build leaves it out: Play doesn't allow apps that block
// other apps' ads (see RELEASING.md); the playstore tag builds it.
const adBlockBuilt = true

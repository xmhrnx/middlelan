//go:build playstore

package main

// adBlockBuilt is false in the Google Play build: Play doesn't allow apps
// that block other apps' ads (see RELEASING.md).
const adBlockBuilt = false

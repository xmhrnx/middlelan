//go:build ios

package ui

// isIOS: iOS keeps apps from seeing other apps' connections, running
// system commands and sending multicast (without an entitlement Apple
// grants on request), so the pages and tools that need those are left out.
const isIOS = true

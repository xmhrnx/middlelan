//go:build !android

package monitor

// UsageAccess reports whether per-app usage statistics are allowed. Only
// Android asks the user; elsewhere it's always true.
func UsageAccess() bool { return true }

// OpenUsageAccessSettings shows Android's Usage Access screen; elsewhere
// it does nothing.
func OpenUsageAccessSettings() error { return nil }

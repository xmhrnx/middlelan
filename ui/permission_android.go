//go:build android

package ui

import "github.com/mehran9675/giode/android"

// requestLocationPermission asks for location access, which Android
// requires before telling apps Wi-Fi network names.
func requestLocationPermission() error {
	return android.CallVoid("com/middlelan/app/PermissionActivity", "requestLocation", "(Landroid/content/Context;)V", android.Context())
}

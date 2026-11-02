//go:build android

package ui

import "github.com/mehran9675/giode/android"

const widgetClass = "com/middlelan/app/UsageWidget"

// pushWidget sends the latest figures to the home screen widget, when the
// user has placed one.
func pushWidget(down, up, today, top string) error {
	placed, err := android.CallBool(widgetClass, "placed", "(Landroid/content/Context;)Z", android.Context())
	if err != nil || !placed {
		return err
	}
	return android.CallVoid(widgetClass, "update",
		"(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)V",
		android.Context(), down, up, today, top)
}

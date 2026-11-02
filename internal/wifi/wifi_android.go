//go:build android

package wifi

import (
	"context"

	"github.com/mehran9675/giode/android"
)

// The Java side is android/java/com/middlelan/app/WifiInfo.java.
const wifiInfoClass = "com/middlelan/app/WifiInfo"

func current(context.Context) (Link, error) {
	s, err := android.CallString(wifiInfoClass, "current", "(Landroid/content/Context;)Ljava/lang/String;", android.Context())
	if err != nil {
		return Link{}, err
	}
	return parseAndroidCurrent(s)
}

func scan(context.Context) ([]Network, error) {
	s, err := android.CallString(wifiInfoClass, "scan", "(Landroid/content/Context;)Ljava/lang/String;", android.Context())
	if err != nil {
		return nil, err
	}
	return parseAndroidScan(s)
}

package com.middlelan.app;

import android.Manifest;
import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.os.Build;
import android.os.Bundle;

/**
 * A transparent activity that asks for location permission, which Android
 * requires before it tells apps Wi-Fi network names. Started from Go,
 * which has no activity of its own to ask from.
 */
public class PermissionActivity extends Activity {
    private static final int REQUEST = 1;

    /** Starts the request. Called from Go. */
    public static void requestLocation(Context ctx) {
        Intent i = new Intent(ctx, PermissionActivity.class);
        i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        ctx.startActivity(i);
    }

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        if (Build.VERSION.SDK_INT < 23) {
            finish();
            return;
        }
        requestPermissions(new String[] {
            Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION,
        }, REQUEST);
    }

    @Override
    public void onRequestPermissionsResult(int code, String[] perms, int[] results) {
        finish();
    }
}

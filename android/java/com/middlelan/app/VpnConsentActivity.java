package com.middlelan.app;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.net.VpnService;
import android.os.Bundle;

/**
 * A transparent activity that shows Android's VPN consent dialog. The
 * dialog has to be started for a result from an activity: it learns
 * which app is asking from the caller, and started any other way it sees
 * none, closes at once and grants nothing. Go, which has no activity of
 * its own, starts this one instead.
 */
public class VpnConsentActivity extends Activity {
    private static final int REQUEST = 1;

    /** Shows the consent dialog. Called from VpnBridge.prepare. */
    static void show(Context ctx) {
        Intent i = new Intent(ctx, VpnConsentActivity.class);
        i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        ctx.startActivity(i);
    }

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        if (state != null) return; // recreated while the dialog is up
        Intent consent;
        try {
            consent = VpnService.prepare(this);
        } catch (Exception e) {
            consent = null;
        }
        if (consent == null) { // already allowed
            finish();
            return;
        }
        try {
            startActivityForResult(consent, REQUEST);
        } catch (Exception e) {
            finish();
        }
    }

    @Override
    protected void onActivityResult(int request, int result, Intent data) {
        // Go polls VpnBridge.isPrepared and starts the service once allowed.
        finish();
    }
}

package com.middlelan.app;

import android.Manifest;
import android.content.Context;
import android.content.pm.PackageManager;
import android.location.LocationManager;
import android.net.wifi.ScanResult;
import android.net.wifi.SupplicantState;
import android.net.wifi.WifiManager;
import android.os.Build;

import java.util.List;

/**
 * Wi-Fi link and scan details for MiddleMan, called from Go
 * (internal/wifi/wifi_android.go) through giode's android package.
 * Results are tab-separated strings parsed by internal/wifi/parse_wifiinfo.go;
 * failures are returned as "ERR:" and a reason ("off", "nowifi",
 * "location", "permission").
 *
 * Network names, BSSIDs and scan results need ACCESS_FINE_LOCATION with
 * location services on; everything needs ACCESS_WIFI_STATE.
 */
public final class WifiInfo {
    private WifiInfo() {}

    private static WifiManager wifi(Context ctx) {
        return (WifiManager) ctx.getApplicationContext().getSystemService(Context.WIFI_SERVICE);
    }

    private static String clean(String s) {
        if (s == null) return "";
        return s.replace('\t', ' ').replace('\n', ' ').replace('\r', ' ');
    }

    private static boolean granted(Context ctx, String perm) {
        if (Build.VERSION.SDK_INT < 23) return true;
        return ctx.checkSelfPermission(perm) == PackageManager.PERMISSION_GRANTED;
    }

    /** Whether the app may read scan results (location permission and location services on). */
    private static boolean canSeeScans(Context ctx) {
        boolean fine = granted(ctx, Manifest.permission.ACCESS_FINE_LOCATION);
        boolean coarse = granted(ctx, Manifest.permission.ACCESS_COARSE_LOCATION);
        if (Build.VERSION.SDK_INT >= 29 ? !fine : !(fine || coarse)) return false;
        if (Build.VERSION.SDK_INT >= 28) {
            LocationManager lm = (LocationManager) ctx.getSystemService(Context.LOCATION_SERVICE);
            if (lm != null && !lm.isLocationEnabled()) return false;
        }
        return true;
    }

    /**
     * The connected network as
     * "connected\tssid\tbssid\trssi\tfrequency\tlinkSpeed\ttxSpeed\trxSpeed\tsecurityType\tcapabilities",
     * where connected is "1" or "0", speeds are Mbps (tx/rx 0 before
     * Android 10), securityType is WifiInfo.getCurrentSecurityType() on
     * Android 12+ (else empty) and capabilities is the matching scan
     * result's capabilities string (empty when unavailable).
     */
    public static String current(Context ctx) {
        WifiManager wm = wifi(ctx);
        if (wm == null) return "ERR:nowifi";
        android.net.wifi.WifiInfo info;
        try {
            if (!wm.isWifiEnabled()) return "ERR:off";
            info = wm.getConnectionInfo();
        } catch (SecurityException e) {
            return "ERR:permission";
        }
        if (info == null) return "0\t\t\t\t\t\t\t\t\t";
        boolean connected = info.getSupplicantState() == SupplicantState.COMPLETED
                || info.getNetworkId() != -1;
        String ssid = info.getSSID();
        if (ssid != null && ssid.length() >= 2 && ssid.startsWith("\"") && ssid.endsWith("\"")) {
            ssid = ssid.substring(1, ssid.length() - 1);
        }
        String bssid = info.getBSSID();
        int freq = Build.VERSION.SDK_INT >= 21 ? info.getFrequency() : 0;
        int tx = 0, rx = 0;
        if (Build.VERSION.SDK_INT >= 29) {
            tx = info.getTxLinkSpeedMbps();
            rx = info.getRxLinkSpeedMbps();
        }
        String secType = "";
        if (Build.VERSION.SDK_INT >= 31) {
            secType = Integer.toString(info.getCurrentSecurityType());
        }
        String caps = "";
        if (bssid != null && canSeeScans(ctx)) {
            try {
                List<ScanResult> results = wm.getScanResults();
                if (results != null) {
                    for (ScanResult r : results) {
                        if (bssid.equalsIgnoreCase(r.BSSID)) {
                            caps = r.capabilities;
                            break;
                        }
                    }
                }
            } catch (SecurityException e) {
                // Without location permission the security stays unknown.
            }
        }
        return (connected ? "1" : "0") + '\t' + clean(ssid) + '\t' + clean(bssid) + '\t'
                + info.getRssi() + '\t' + freq + '\t' + info.getLinkSpeed() + '\t'
                + tx + '\t' + rx + '\t' + secType + '\t' + clean(caps);
    }

    /**
     * The latest scan results, one network per line:
     * "ssid\tbssid\tlevel\tfrequency\tcapabilities". Also asks for a new
     * scan (Android throttles these), whose results show up on a later call.
     */
    public static String scan(Context ctx) {
        WifiManager wm = wifi(ctx);
        if (wm == null) return "ERR:nowifi";
        List<ScanResult> results;
        try {
            if (!wm.isWifiEnabled()) return "ERR:off";
            if (!canSeeScans(ctx)) return "ERR:location";
            try {
                wm.startScan();
            } catch (SecurityException e) {
                // CHANGE_WIFI_STATE missing: use the cached results.
            }
            results = wm.getScanResults();
        } catch (SecurityException e) {
            return "ERR:location";
        }
        StringBuilder sb = new StringBuilder();
        if (results == null) return "";
        for (ScanResult r : results) {
            sb.append(clean(r.SSID)).append('\t').append(clean(r.BSSID)).append('\t')
              .append(r.level).append('\t').append(r.frequency).append('\t')
              .append(clean(r.capabilities)).append('\n');
        }
        return sb.toString();
    }
}

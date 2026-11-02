package com.middlelan.app;

import android.app.AppOpsManager;
import android.app.usage.NetworkStats;
import android.app.usage.NetworkStatsManager;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.net.ConnectivityManager;
import android.net.TrafficStats;
import android.os.Build;
import android.os.Process;
import android.provider.Settings;

import java.util.HashMap;
import java.util.Map;

/**
 * Per-app network usage for MiddleLAN, called from Go through giode's
 * android package. Android doesn't let apps see other apps' sockets, so
 * traffic per app comes from NetworkStatsManager, which needs the user to
 * grant Usage Access.
 */
public final class NetStats {
    private NetStats() {}

    /** Whether the user granted Usage Access to this app. */
    public static boolean hasUsageAccess(Context ctx) {
        AppOpsManager ops = (AppOpsManager) ctx.getSystemService(Context.APP_OPS_SERVICE);
        if (ops == null) return false;
        int mode;
        if (Build.VERSION.SDK_INT >= 29) {
            mode = ops.unsafeCheckOpNoThrow(AppOpsManager.OPSTR_GET_USAGE_STATS, Process.myUid(), ctx.getPackageName());
        } else {
            mode = ops.checkOpNoThrow(AppOpsManager.OPSTR_GET_USAGE_STATS, Process.myUid(), ctx.getPackageName());
        }
        return mode == AppOpsManager.MODE_ALLOWED;
    }

    /** Opens the system screen where the user grants Usage Access. */
    public static void openUsageAccessSettings(Context ctx) {
        Intent i = new Intent(Settings.ACTION_USAGE_ACCESS_SETTINGS);
        i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        ctx.startActivity(i);
    }

    /** The device's total received and sent bytes since boot, as "rx,tx". */
    public static String totals() {
        return TrafficStats.getTotalRxBytes() + "," + TrafficStats.getTotalTxBytes();
    }

    /**
     * Bytes per app between two times (milliseconds since the epoch) over
     * Wi-Fi and mobile data, one app per line: "uid\tlabel\tpackage\trx\ttx".
     * Returns "ERR:" and a reason when it can't.
     */
    public static String appUsage(Context ctx, long start, long end) {
        if (Build.VERSION.SDK_INT < 23) return "ERR:Android 6 or later is needed";
        NetworkStatsManager nsm = (NetworkStatsManager) ctx.getSystemService(Context.NETWORK_STATS_SERVICE);
        if (nsm == null) return "ERR:no network stats service";
        Map<Integer, long[]> byUid = new HashMap<>();
        int[] types = {ConnectivityManager.TYPE_WIFI, ConnectivityManager.TYPE_MOBILE};
        for (int type : types) {
            NetworkStats stats = null;
            try {
                stats = nsm.querySummary(type, null, start, end);
                NetworkStats.Bucket b = new NetworkStats.Bucket();
                while (stats.hasNextBucket()) {
                    stats.getNextBucket(b);
                    long[] v = byUid.get(b.getUid());
                    if (v == null) {
                        v = new long[2];
                        byUid.put(b.getUid(), v);
                    }
                    v[0] += b.getRxBytes();
                    v[1] += b.getTxBytes();
                }
            } catch (SecurityException e) {
                return "ERR:permission";
            } catch (Exception e) {
                // A network type the device lacks (no SIM): skip it.
            } finally {
                if (stats != null) stats.close();
            }
        }
        PackageManager pm = ctx.getPackageManager();
        StringBuilder sb = new StringBuilder();
        for (Map.Entry<Integer, long[]> e : byUid.entrySet()) {
            int uid = e.getKey();
            String label = labelFor(pm, uid);
            String pkg = "";
            String[] pkgs = pm.getPackagesForUid(uid);
            if (pkgs != null && pkgs.length > 0) pkg = pkgs[0];
            sb.append(uid).append('\t').append(label.replace('\t', ' ').replace('\n', ' '))
              .append('\t').append(pkg).append('\t')
              .append(e.getValue()[0]).append('\t').append(e.getValue()[1]).append('\n');
        }
        return sb.toString();
    }

    private static String labelFor(PackageManager pm, int uid) {
        switch (uid) {
            case -4: return "Removed apps";
            case -5: return "Tethering & hotspot";
            case 0: return "Android OS (root)";
            case 1000: return "Android System";
            case 1013: return "Media server";
            case 1051: return "DNS resolver";
        }
        String[] pkgs = pm.getPackagesForUid(uid);
        if (pkgs == null || pkgs.length == 0) return "uid " + uid;
        try {
            ApplicationInfo ai = pm.getApplicationInfo(pkgs[0], 0);
            CharSequence l = pm.getApplicationLabel(ai);
            if (pkgs.length > 1) return l + " (+" + (pkgs.length - 1) + ")";
            return l.toString();
        } catch (PackageManager.NameNotFoundException ex) {
            return pkgs[0];
        }
    }
}

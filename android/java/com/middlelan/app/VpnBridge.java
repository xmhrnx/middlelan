package com.middlelan.app;

import android.content.Context;
import android.content.Intent;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.VpnService;
import android.os.Build;

import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.util.ArrayList;
import java.util.List;

/**
 * The on-device VPN's bridge between Go (internal/vpn) and Android. Go
 * calls the static helpers through giode's android package; the service
 * calls into Go through the native methods, which internal/vpn exports
 * as Java_com_middlelan_app_VpnBridge_*.
 */
public final class VpnBridge {
    private VpnBridge() {}

    /** The virtual resolver the tunnel advertises; never an upstream. */
    static final String VIRTUAL_DNS = "10.111.0.2";

    private static volatile Context appContext;

    static void remember(Context ctx) {
        if (ctx == null) return;
        Context app = ctx.getApplicationContext();
        appContext = app != null ? app : ctx;
    }

    /** Starts Go's engine on the tunnel descriptor, which Go then owns. */
    static native boolean nativeStart(int fd, String dns);

    /** Stops Go's engine; it closes the tunnel descriptor. */
    static native void nativeStop();

    /** Gives Go new upstream DNS servers (comma separated). */
    static native void nativeSetDns(String dns);

    /** Whether the user already allowed this app to run a VPN. */
    public static boolean isPrepared(Context ctx) {
        remember(ctx);
        try {
            return VpnService.prepare(ctx) == null;
        } catch (Exception e) {
            return false;
        }
    }

    /**
     * Returns true when VPN consent is already granted; otherwise shows
     * the system consent dialog and returns false.
     */
    public static boolean prepare(Context ctx) {
        remember(ctx);
        Intent consent;
        try {
            consent = VpnService.prepare(ctx);
        } catch (Exception e) {
            return false;
        }
        if (consent == null) return true;
        consent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            ctx.startActivity(consent);
        } catch (Exception e) {
            // No activity to show it (very old or locked-down devices).
        }
        return false;
    }

    /** Asks MiddleVpnService to establish the tunnel. */
    public static boolean start(Context ctx) {
        remember(ctx);
        Intent i = new Intent(ctx, MiddleVpnService.class);
        i.setAction(MiddleVpnService.ACTION_START);
        try {
            if (Build.VERSION.SDK_INT >= 26) {
                ctx.startForegroundService(i);
            } else {
                ctx.startService(i);
            }
            return true;
        } catch (Exception e) {
            return false;
        }
    }

    /** Asks MiddleVpnService to tear the tunnel down. */
    public static void stop(Context ctx) {
        remember(ctx);
        MiddleVpnService.requestStop();
    }

    /**
     * The uid owning a connection seen in the tunnel (proto 6 for TCP, 17
     * for UDP; src is the app's side), or -1. Needs Android 10, where only
     * the active VPN app may ask.
     */
    public static int ownerUid(int proto, String srcIp, int srcPort, String dstIp, int dstPort) {
        if (Build.VERSION.SDK_INT < 29) return -1;
        Context ctx = appContext;
        if (ctx == null) return -1;
        ConnectivityManager cm = (ConnectivityManager) ctx.getSystemService(Context.CONNECTIVITY_SERVICE);
        if (cm == null) return -1;
        try {
            InetSocketAddress local = new InetSocketAddress(InetAddress.getByName(srcIp), srcPort);
            InetSocketAddress remote = new InetSocketAddress(InetAddress.getByName(dstIp), dstPort);
            return cm.getConnectionOwnerUid(proto, local, remote);
        } catch (Exception e) {
            return -1;
        }
    }

    /** An app's name and package for a uid, as "label\tpackage". */
    public static String appLabel(Context ctx, int uid) {
        remember(ctx);
        PackageManager pm = ctx.getPackageManager();
        String pkg = "";
        String[] pkgs = null;
        try {
            pkgs = pm.getPackagesForUid(uid);
        } catch (Exception e) {
            // Unknown uid.
        }
        if (pkgs != null && pkgs.length > 0) pkg = pkgs[0];
        String label = labelFor(pm, uid, pkgs);
        return clean(label) + "\t" + clean(pkg);
    }

    private static String clean(String s) {
        return s == null ? "" : s.replace('\t', ' ').replace('\n', ' ');
    }

    private static String labelFor(PackageManager pm, int uid, String[] pkgs) {
        switch (uid) {
            case 0: return "Android OS (root)";
            case 1000: return "Android System";
            case 1013: return "Media server";
            case 1051: return "DNS resolver";
        }
        if (pkgs == null || pkgs.length == 0) return "uid " + uid;
        try {
            ApplicationInfo ai = pm.getApplicationInfo(pkgs[0], 0);
            CharSequence l = pm.getApplicationLabel(ai);
            if (pkgs.length > 1) return l + " (+" + (pkgs.length - 1) + ")";
            return l.toString();
        } catch (Exception e) {
            return pkgs[0];
        }
    }

    /**
     * The DNS servers of the underlying (non-VPN) network, comma
     * separated, or "" when unknown.
     */
    public static String dnsServers(Context ctx) {
        remember(ctx);
        ConnectivityManager cm = (ConnectivityManager) ctx.getSystemService(Context.CONNECTIVITY_SERVICE);
        if (cm == null) return "";
        List<String> out = new ArrayList<String>();
        try {
            Network active = cm.getActiveNetwork();
            if (active != null && !isVpn(cm, active)) {
                addDns(cm, active, out);
            }
            if (out.isEmpty()) {
                for (Network n : cm.getAllNetworks()) {
                    if (isVpn(cm, n)) continue;
                    NetworkCapabilities caps = cm.getNetworkCapabilities(n);
                    if (caps == null || !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)) continue;
                    addDns(cm, n, out);
                }
            }
        } catch (Exception e) {
            // Leave what was found.
        }
        StringBuilder sb = new StringBuilder();
        for (String s : out) {
            if (sb.length() > 0) sb.append(',');
            sb.append(s);
        }
        return sb.toString();
    }

    private static boolean isVpn(ConnectivityManager cm, Network n) {
        NetworkCapabilities caps = cm.getNetworkCapabilities(n);
        return caps != null && caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN);
    }

    private static void addDns(ConnectivityManager cm, Network n, List<String> out) {
        LinkProperties lp = cm.getLinkProperties(n);
        if (lp == null) return;
        for (InetAddress a : lp.getDnsServers()) {
            String s = a.getHostAddress();
            if (s == null || s.equals(VIRTUAL_DNS) || out.contains(s)) continue;
            out.add(s);
        }
    }
}

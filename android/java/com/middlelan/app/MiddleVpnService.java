package com.middlelan.app;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.content.pm.ServiceInfo;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;
import android.net.VpnService;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.os.ParcelFileDescriptor;

/**
 * MiddleMan's on-device VPN. It routes every app's traffic into a local
 * tunnel whose packets Go (internal/vpn) terminates and re-sends from
 * MiddleMan's own sockets, so each connection can be attributed to its
 * app and blocked. Nothing goes to a remote server. MiddleMan itself is
 * left out of the tunnel, so Go's sockets reach the network directly.
 */
public class MiddleVpnService extends VpnService {
    static final String ACTION_START = "com.middlelan.app.vpn.START";
    static final String ACTION_STOP = "com.middlelan.app.vpn.STOP";

    private static final String CHANNEL = "middlelan_vpn";
    private static final int NOTIFICATION_ID = 0x4d4c;

    private static volatile MiddleVpnService instance;

    private boolean running;
    private boolean foreground;
    private ConnectivityManager.NetworkCallback networkCallback;

    /** Stops the running service, if any, on the main thread. */
    static void requestStop() {
        final MiddleVpnService s = instance;
        if (s == null) return;
        new Handler(Looper.getMainLooper()).post(new Runnable() {
            @Override
            public void run() {
                s.shutdown();
            }
        });
    }

    @Override
    public void onCreate() {
        super.onCreate();
        instance = this;
        VpnBridge.remember(this);
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        String action = intent != null ? intent.getAction() : null;
        if (ACTION_STOP.equals(action)) {
            shutdown();
            return START_NOT_STICKY;
        }
        // ACTION_START from the app, or the system starting an always-on
        // VPN (action android.net.VpnService, or a null intent).
        instance = this;
        goForeground();
        if (!running && !establish()) {
            shutdown();
        }
        return START_NOT_STICKY;
    }

    private boolean establish() {
        if (!loadGo()) return false;
        String dns = VpnBridge.dnsServers(this);
        Builder b = new Builder();
        b.setSession("MiddleMan");
        b.addAddress("10.111.0.1", 32);
        b.addAddress("fd00:111::1", 128);
        b.addRoute("0.0.0.0", 0);
        b.addRoute("::", 0);
        b.addDnsServer(VpnBridge.VIRTUAL_DNS);
        b.setMtu(1500);
        b.setBlocking(true);
        try {
            b.addDisallowedApplication(getPackageName());
        } catch (PackageManager.NameNotFoundException e) {
            // Our own package always exists.
        }
        if (Build.VERSION.SDK_INT >= 29) {
            b.setMetered(false);
        }
        PendingIntent open = openAppIntent();
        if (open != null) b.setConfigureIntent(open);

        ParcelFileDescriptor pfd;
        try {
            pfd = b.establish();
        } catch (Exception e) {
            return false;
        }
        if (pfd == null) return false; // consent was revoked
        int fd = pfd.detachFd();
        boolean ok;
        try {
            ok = VpnBridge.nativeStart(fd, dns);
        } catch (Throwable t) {
            // Go isn't loaded; the descriptor is ours to close.
            try {
                ParcelFileDescriptor.adoptFd(fd).close();
            } catch (Exception e) {
                // Nothing more to do.
            }
            return false;
        }
        if (!ok) return false; // Go closed the descriptor
        running = true;
        watchNetwork();
        return true;
    }

    /**
     * Makes sure the Go library is loaded and running: the system may
     * start an always-on VPN before the app's activity ever ran.
     */
    private boolean loadGo() {
        try {
            Class<?> gio = Class.forName("org.gioui.Gio");
            gio.getMethod("init", Context.class).invoke(null, getApplicationContext());
            return true;
        } catch (Throwable t) {
            try {
                System.loadLibrary("gio");
                return true;
            } catch (Throwable t2) {
                return false;
            }
        }
    }

    /** Hands Go the new DNS servers whenever the underlying network changes. */
    private void watchNetwork() {
        if (Build.VERSION.SDK_INT < 24 || networkCallback != null) return;
        final ConnectivityManager cm = (ConnectivityManager) getSystemService(Context.CONNECTIVITY_SERVICE);
        if (cm == null) return;
        networkCallback = new ConnectivityManager.NetworkCallback() {
            @Override
            public void onAvailable(Network network) {
                pushDns();
            }

            @Override
            public void onLinkPropertiesChanged(Network network, LinkProperties lp) {
                pushDns();
            }
        };
        try {
            // This app is outside the tunnel, so its default network is
            // the underlying one.
            cm.registerDefaultNetworkCallback(networkCallback);
        } catch (Exception e) {
            networkCallback = null;
        }
    }

    private void pushDns() {
        if (!running) return;
        String dns = VpnBridge.dnsServers(this);
        if (dns.length() == 0) return;
        try {
            VpnBridge.nativeSetDns(dns);
        } catch (Throwable t) {
            // Go went away; nothing to update.
        }
    }

    private void unwatchNetwork() {
        if (networkCallback == null) return;
        ConnectivityManager cm = (ConnectivityManager) getSystemService(Context.CONNECTIVITY_SERVICE);
        if (cm != null) {
            try {
                cm.unregisterNetworkCallback(networkCallback);
            } catch (Exception e) {
                // Already unregistered.
            }
        }
        networkCallback = null;
    }

    /** Stops Go, the notification and the service. Safe to call twice. */
    void shutdown() {
        unwatchNetwork();
        if (running) {
            running = false;
            try {
                VpnBridge.nativeStop();
            } catch (Throwable t) {
                // Go isn't loaded.
            }
        }
        if (foreground) {
            foreground = false;
            if (Build.VERSION.SDK_INT >= 24) {
                stopForeground(STOP_FOREGROUND_REMOVE);
            } else {
                stopForeground(true);
            }
        }
        if (instance == this) instance = null;
        stopSelf();
    }

    @Override
    public void onRevoke() {
        // Another VPN took over or the user turned this one off.
        shutdown();
        super.onRevoke();
    }

    @Override
    public void onDestroy() {
        shutdown();
        super.onDestroy();
    }

    private void goForeground() {
        if (foreground) return;
        Notification n = buildNotification();
        try {
            if (Build.VERSION.SDK_INT >= 34) {
                startForeground(NOTIFICATION_ID, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE);
            } else {
                startForeground(NOTIFICATION_ID, n);
            }
            foreground = true;
        } catch (Exception e) {
            // Not allowed to go foreground now; the VPN still works.
        }
    }

    private Notification buildNotification() {
        Notification.Builder nb;
        if (Build.VERSION.SDK_INT >= 26) {
            NotificationManager nm = (NotificationManager) getSystemService(Context.NOTIFICATION_SERVICE);
            if (nm != null && nm.getNotificationChannel(CHANNEL) == null) {
                NotificationChannel ch = new NotificationChannel(CHANNEL, "Network monitor (VPN)", NotificationManager.IMPORTANCE_LOW);
                ch.setDescription("Shown while MiddleMan watches this device's connections through its local VPN.");
                ch.setShowBadge(false);
                nm.createNotificationChannel(ch);
            }
            nb = new Notification.Builder(this, CHANNEL);
        } else {
            nb = new Notification.Builder(this);
            nb.setPriority(Notification.PRIORITY_LOW);
        }
        int icon = getApplicationInfo().icon;
        if (icon == 0) icon = android.R.drawable.stat_sys_warning;
        nb.setSmallIcon(icon)
          .setContentTitle("MiddleMan is monitoring connections")
          .setContentText("Traffic stays on this device; tap to open MiddleMan.")
          .setOngoing(true)
          .setShowWhen(false);
        PendingIntent open = openAppIntent();
        if (open != null) nb.setContentIntent(open);
        if (Build.VERSION.SDK_INT >= 31) {
            nb.setForegroundServiceBehavior(Notification.FOREGROUND_SERVICE_IMMEDIATE);
        }
        return nb.build();
    }

    private PendingIntent openAppIntent() {
        Intent launch = getPackageManager().getLaunchIntentForPackage(getPackageName());
        if (launch == null) return null;
        launch.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_RESET_TASK_IF_NEEDED);
        int flags = PendingIntent.FLAG_UPDATE_CURRENT;
        if (Build.VERSION.SDK_INT >= 23) flags |= PendingIntent.FLAG_IMMUTABLE;
        return PendingIntent.getActivity(this, 0, launch, flags);
    }
}

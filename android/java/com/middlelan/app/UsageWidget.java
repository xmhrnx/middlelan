package com.middlelan.app;

import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.os.Build;
import android.widget.RemoteViews;

/**
 * MiddleMan's home screen widget: live speeds, today's usage and the
 * busiest app. Go pushes new values with {@link #update}; the widget keeps
 * the last ones in SharedPreferences so it redraws after a reboot.
 * Its layout and provider info come from android/libs/middlelan-widget.aar.
 */
public class UsageWidget extends AppWidgetProvider {
    private static final String PREFS = "middlelan_widget";

    @Override
    public void onUpdate(Context ctx, AppWidgetManager mgr, int[] ids) {
        render(ctx, mgr, ids);
    }

    /** Saves new values and redraws every MiddleMan widget. Called from Go. */
    public static void update(Context ctx, String down, String up, String today, String top) {
        SharedPreferences.Editor e = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit();
        e.putString("down", down).putString("up", up).putString("today", today).putString("top", top).apply();
        AppWidgetManager mgr = AppWidgetManager.getInstance(ctx);
        int[] ids = mgr.getAppWidgetIds(new ComponentName(ctx, UsageWidget.class));
        if (ids != null && ids.length > 0) render(ctx, mgr, ids);
    }

    /** Whether the user has placed a widget, so Go can skip updates otherwise. */
    public static boolean placed(Context ctx) {
        int[] ids = AppWidgetManager.getInstance(ctx).getAppWidgetIds(new ComponentName(ctx, UsageWidget.class));
        return ids != null && ids.length > 0;
    }

    private static void render(Context ctx, AppWidgetManager mgr, int[] ids) {
        String pkg = ctx.getPackageName();
        int layout = ctx.getResources().getIdentifier("middlelan_widget", "layout", pkg);
        if (layout == 0) return;
        SharedPreferences p = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        RemoteViews v = new RemoteViews(pkg, layout);
        set(ctx, v, "middlelan_widget_down", "↓ " + p.getString("down", "—"));
        set(ctx, v, "middlelan_widget_up", "↑ " + p.getString("up", "—"));
        set(ctx, v, "middlelan_widget_today", "Today " + p.getString("today", "—"));
        set(ctx, v, "middlelan_widget_top", p.getString("top", ""));
        Intent open = ctx.getPackageManager().getLaunchIntentForPackage(pkg);
        int root = ctx.getResources().getIdentifier("middlelan_widget_root", "id", pkg);
        if (open != null && root != 0) {
            int flags = PendingIntent.FLAG_UPDATE_CURRENT;
            if (Build.VERSION.SDK_INT >= 23) flags |= PendingIntent.FLAG_IMMUTABLE;
            v.setOnClickPendingIntent(root, PendingIntent.getActivity(ctx, 0, open, flags));
        }
        for (int id : ids) mgr.updateAppWidget(id, v);
    }

    private static void set(Context ctx, RemoteViews v, String name, String text) {
        int id = ctx.getResources().getIdentifier(name, "id", ctx.getPackageName());
        if (id != 0) v.setTextViewText(id, text);
    }
}

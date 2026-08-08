package com.cjxpj.nebula;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.os.PowerManager;
import android.util.Log;

/**
 * 前台服务 —— 保持进程在后台存活
 *
 * 策略：
 *   - 前台 Service + 常驻通知（Android 要求）
 *   - WakeLock 防止 CPU 休眠
 *   - START_STICKY 被系统杀掉后自动重启
 */
public class NebulaService extends Service {

    private static final String TAG = "NebulaService";
    private static final String CHANNEL_ID = "nebula_keepalive";
    private static final String CHANNEL_NAME = "Nebula 服务";
    private static final int NOTIFICATION_ID = 9527;

    private PowerManager.WakeLock mWakeLock;
    private final Handler mHandler = new Handler(Looper.getMainLooper());
    private static final long WAKELOCK_REFRESH_MS = 9 * 60 * 1000L; // 9分钟续期一次

    private final Runnable mWakeLockRefresher = new Runnable() {
        @Override
        public void run() {
            acquireWakeLock();
            mHandler.postDelayed(this, WAKELOCK_REFRESH_MS);
        }
    };

    @Override
    public void onCreate() {
        super.onCreate();
        Log.i(TAG, "保活服务创建");

        // 创建通知渠道
        createNotificationChannel();

        // 获取 WakeLock（PARTIAL_WAKE_LOCK：保持 CPU 运行，屏幕和键盘可关闭）
        PowerManager pm = (PowerManager) getSystemService(Context.POWER_SERVICE);
        if (pm != null) {
            mWakeLock = pm.newWakeLock(
                    PowerManager.PARTIAL_WAKE_LOCK,
                    "Nebula:KeepAlive"
            );
            mWakeLock.setReferenceCounted(false);
        }
        acquireWakeLock();
        mHandler.postDelayed(mWakeLockRefresher, WAKELOCK_REFRESH_MS);
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        Log.i(TAG, "前台服务启动");

        // 构建通知（点击回到 MainActivity）
        Intent notifyIntent = new Intent(this, MainActivity.class);
        notifyIntent.setFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP);
        PendingIntent pendingIntent = PendingIntent.getActivity(
                this, 0, notifyIntent,
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT
        );

        Notification notification = new Notification.Builder(this, CHANNEL_ID)
                .setContentTitle("Nebula")
                .setContentText("管理面板运行中")
                .setSmallIcon(android.R.drawable.ic_menu_manage)
                .setContentIntent(pendingIntent)
                .setOngoing(true)
                .build();

        startForeground(NOTIFICATION_ID, notification);

        return START_STICKY;  // 被系统杀掉后自动重启
    }

    @Override
    public void onDestroy() {
        Log.w(TAG, "保活服务被销毁");
        mHandler.removeCallbacks(mWakeLockRefresher);
        releaseWakeLock();
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    // ---- WakeLock ----

    private void acquireWakeLock() {
        if (mWakeLock != null && !mWakeLock.isHeld()) {
            mWakeLock.acquire(10 * 60 * 1000L);  // 10分钟后自动释放，避免长期持有
            Log.d(TAG, "WakeLock 已获取");
        }
    }

    private void releaseWakeLock() {
        if (mWakeLock != null && mWakeLock.isHeld()) {
            mWakeLock.release();
            Log.d(TAG, "WakeLock 已释放");
        }
    }

    // ---- 通知渠道 ----

    private void createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            NotificationChannel channel = new NotificationChannel(
                    CHANNEL_ID,
                    CHANNEL_NAME,
                    NotificationManager.IMPORTANCE_LOW  // 无声音、无震动
            );
            channel.setDescription("Nebula 后台运行状态");
            channel.setShowBadge(false);

            NotificationManager nm = (NotificationManager) getSystemService(Context.NOTIFICATION_SERVICE);
            if (nm != null) {
                nm.createNotificationChannel(channel);
            }
        }
    }
}

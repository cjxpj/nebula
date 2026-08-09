package com.cjxpj.nebula;

import android.Manifest;
import android.annotation.SuppressLint;
import android.app.Activity;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.BatteryManager;
import android.os.Build;
import android.os.Bundle;
import android.os.Environment;
import android.os.Handler;
import android.os.Looper;
import android.os.PowerManager;
import android.provider.Settings;
import android.util.Log;
import android.view.KeyEvent;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;

import org.json.JSONObject;

import java.lang.reflect.Method;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

import dalvik.system.DexClassLoader;
import rikka.shizuku.Shizuku;

/**
 * 主 Activity —— WebView 显示 Go HTTP 服务的管理面板
 *
 * 流程：
 *   1. 请求必需权限（管理所有文件 + 通知 + 电池优化）
 *   2. RunNebula() → 等待 HTTP
 *   3. startKeepAliveService() → getOpuiUrl() → WebView 加载
 */
public class MainActivity extends Activity {

    private static final String TAG = "MainActivity";
    private static final String HTTP_HOST = "http://127.0.0.1:8080";
    private static final int REQ_NOTIFICATION = 101;

    // 类加载时自动加载 .so，确保 JNI 方法可用
    static {
        try {
            System.loadLibrary("nebula_arm64");
            Log.i(TAG, "libnebula_arm64.so 加载成功");
        } catch (UnsatisfiedLinkError e) {
            Log.e(TAG, "加载 .so 失败: " + e.getMessage());
        }
    }

    private WebView mWebView;
    private final Handler mHandler = new Handler(Looper.getMainLooper());
    private volatile boolean mServiceStarted = false;  // 防重复启动
    private boolean mStoragePermissionRequested = false; // 防止设置页无限循环

    // DexClassLoader 缓存：避免每次 DEX 执行都重新加载
    private final ConcurrentHashMap<String, DexClassLoader> mDexLoaders = new ConcurrentHashMap<>();

    // 通知相关
    private static final String NOTIFY_CHANNEL_ID = "nebula_notify";
    private static final String NOTIFY_CHANNEL_NAME = "Nebula 通知";
    private final AtomicInteger mNotifyIdCounter = new AtomicInteger(1000);

    // Shizuku 监听器
    private final Shizuku.OnBinderReceivedListener mBinderReceivedListener = () -> {
        Log.i(TAG, "Shizuku 服务已连接");
    };
    private final Shizuku.OnBinderDeadListener mBinderDeadListener = () -> {
        Log.i(TAG, "Shizuku 服务已断开");
    };

    // ---------- JNI 原生方法 ----------

    // 对应 Go: Java_com_cjxpj_nebula_MainActivity_RunNebula
    private native void RunNebula();

    // 对应 Go: Java_com_cjxpj_nebula_MainActivity_getOpuiUrl
    // 返回 opui 访问路径，如 "/nebula"
    private native String getOpuiUrl();

    // 手机端专属：设备信息、电量、注册
    private native void setDeviceInfo(String json);
    private native void updateBatteryStatus(int level, boolean charging);
    private native void registerDevice();

    @SuppressLint("SetJavaScriptEnabled")
    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // 创建 WebView（先显示加载提示）
        mWebView = new WebView(this);
        setContentView(mWebView);

        // 注册 Shizuku 服务监听
        Shizuku.addBinderReceivedListener(mBinderReceivedListener);
        Shizuku.addBinderDeadListener(mBinderDeadListener);

        WebSettings settings = mWebView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setCacheMode(WebSettings.LOAD_NO_CACHE);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_ALWAYS_ALLOW);

        mWebView.setWebViewClient(new WebViewClient() {
            @Override
            public void onReceivedError(WebView view, WebResourceRequest request,
                                        WebResourceError error) {
                Log.e(TAG, "WebView 加载失败: " + error.getErrorCode() + " " + error.getDescription());
                showError("页面加载失败 (" + error.getErrorCode() + ")");
            }
        });
        mWebView.setWebChromeClient(new WebChromeClient());

        // 显示加载中页面
        mWebView.loadData(
                "<html><body style='display:flex;align-items:center;justify-content:center;height:100vh;font-family:sans-serif;background:#1a1a2e;color:#eee'><div style='text-align:center'><h2>Nebula</h2><p>正在启动服务...</p></div></body></html>",
                "text/html", "UTF-8");

        // 先请求必需权限，再后台启动
        checkPermissionsAndStart();
    }

    // ---------- 权限请求 ----------

    /**
     * 检查并请求所需权限：管理所有文件 + 通知 + 电池优化
     */
    private void checkPermissionsAndStart() {
        if (mServiceStarted) return;

        // Android 11+：访问 Documents/NebulaData 需要管理所有文件权限
        if (Build.VERSION.SDK_INT >= 30) {
            if (!Environment.isExternalStorageManager()) {
                if (!mStoragePermissionRequested) {
                    mStoragePermissionRequested = true;
                    Intent intent = new Intent(Settings.ACTION_MANAGE_APP_ALL_FILES_ACCESS_PERMISSION);
                    intent.setData(Uri.parse("package:" + getPackageName()));
                    startActivity(intent);
                    return;
                }
                showError("需要「所有文件访问」权限才能运行，请在设置中开启后重新打开应用");
                return;
            }
        }

        // Android 13+：前台服务必须显示通知，需运行时权限
        if (Build.VERSION.SDK_INT >= 33) {
            if (checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS)
                    != PackageManager.PERMISSION_GRANTED) {
                requestPermissions(
                        new String[]{Manifest.permission.POST_NOTIFICATIONS},
                        REQ_NOTIFICATION);
                return; // 回调 onRequestPermissionsResult 后继续
            }
        }

        // 权限已就绪，启动服务
        mServiceStarted = true;
        new Thread(this::startAndLoad, "nebula-boot").start();
    }

    @Override
    public void onRequestPermissionsResult(int requestCode,
                                           String[] permissions,
                                           int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode == REQ_NOTIFICATION && !mServiceStarted) {
            // 无论通知权限是否被授予，都继续启动（前台服务会自动适配）
            mServiceStarted = true;
            new Thread(this::startAndLoad, "nebula-boot").start();
        }
    }

    /**
     * 请求忽略电池优化（用户需手动确认）
     */
    private void requestBatteryOptimizationExemption() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            PowerManager pm = (PowerManager) getSystemService(POWER_SERVICE);
            if (pm != null && !pm.isIgnoringBatteryOptimizations(getPackageName())) {
                Intent intent = new Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS);
                intent.setData(Uri.parse("package:" + getPackageName()));
                startActivity(intent);
                Log.i(TAG, "已请求电池优化白名单");
            }
        }
    }

    /**
     * 后台线程：启动服务 → 等待就绪 → 加载 opui
     */
    private void startAndLoad() {
        // 1) 启动 Go HTTP 服务
        try {
            Log.i(TAG, "正在启动 Nebula 服务...");
            RunNebula();
        } catch (Exception e) {
            Log.e(TAG, "RunNebula 异常", e);
            showError("原生服务启动失败: " + e.getMessage());
            return;
        }

        // 2) 等待 HTTP 服务就绪
        if (!waitForHttpReady(30)) {
            showError("HTTP 服务启动超时");
            return;
        }

        // 3) 启动前台保活服务 + 请求电池优化 + 采集设备信息并注册
        startKeepAliveService();
        mHandler.post(() -> {
            requestBatteryOptimizationExemption();
            collectAndSendDeviceInfo();
            registerDevice();
        });

        // 4) 从 so 直接获取 opui 路径
        String opuiUrl;
        try {
            opuiUrl = getOpuiUrl();
            if (opuiUrl == null || opuiUrl.isEmpty()) {
                opuiUrl = "/nebula"; // 回退默认值
            }
        } catch (Exception e) {
            Log.e(TAG, "getOpuiUrl 异常", e);
            opuiUrl = "/nebula";
        }

        String url = HTTP_HOST + opuiUrl;
        Log.i(TAG, "加载 opui: " + url);

        // 5) 加载到 WebView（先清除 loadData 历史，防止返回键变白）
        mHandler.post(() -> {
            mWebView.clearHistory();
            mWebView.loadUrl(url);
        });
    }

    /**
     * 轮询 HTTP 服务直到就绪
     */
    private boolean waitForHttpReady(int timeoutSeconds) {
        long deadline = System.currentTimeMillis() + timeoutSeconds * 1000L;
        while (System.currentTimeMillis() < deadline) {
            try {
                java.net.URL url = new java.net.URL(HTTP_HOST + "/");
                java.net.HttpURLConnection conn = (java.net.HttpURLConnection) url.openConnection();
                conn.setConnectTimeout(2000);
                conn.setReadTimeout(2000);
                conn.setRequestMethod("GET");
                int code = conn.getResponseCode();
                conn.disconnect();
                if (code > 0) {
                    Log.i(TAG, "HTTP 服务就绪 (HTTP " + code + ")");
                    return true;
                }
            } catch (Exception ignored) {
            }
            try { Thread.sleep(1000); } catch (InterruptedException ignored) {}
        }
        return false;
    }

    private void showError(String msg) {
        Log.e(TAG, msg);
        String safeMsg = msg.replace("&", "&amp;").replace("<", "&lt;")
                .replace(">", "&gt;").replace("\"", "&quot;");
        String html = "<html><body style='display:flex;align-items:center;justify-content:center;height:100vh;font-family:sans-serif;background:#1a1a2e;color:#e74c3c'><div style='text-align:center;padding:20px'><h2>错误</h2><p>" + safeMsg + "</p></div></body></html>";
        mHandler.post(() -> mWebView.loadData(html, "text/html", "UTF-8"));
    }

    // ---------- 手机端专属功能 ----------

    /**
     * 采集设备信息并传递给 Go 层
     */
    private void collectAndSendDeviceInfo() {
        try {
            JSONObject json = new JSONObject();
            json.put("brand", Build.BRAND);
            json.put("model", Build.MODEL);
            json.put("manufacturer", Build.MANUFACTURER);
            json.put("device", Build.DEVICE);
            json.put("product", Build.PRODUCT);
            json.put("sdk", Build.VERSION.SDK_INT);
            json.put("release", Build.VERSION.RELEASE);
            json.put("fingerprint", Build.FINGERPRINT);
            json.put("hardware", Build.HARDWARE);
            String info = json.toString();
            Log.i(TAG, "设备信息: " + info);
            setDeviceInfo(info);
        } catch (Exception e) {
            Log.e(TAG, "采集设备信息失败", e);
        }
    }

    /**
     * 电量变化广播接收器
     */
    private final BroadcastReceiver mBatteryReceiver = new BroadcastReceiver() {
        @Override
        public void onReceive(Context context, Intent intent) {
            int level = intent.getIntExtra(BatteryManager.EXTRA_LEVEL, -1);
            int scale = intent.getIntExtra(BatteryManager.EXTRA_SCALE, -1);

            // 优先用 getIntProperty 获取精确电量（与系统状态栏一致）
            BatteryManager bm = (BatteryManager) getSystemService(BATTERY_SERVICE);
            int pct = bm != null ? bm.getIntProperty(BatteryManager.BATTERY_PROPERTY_CAPACITY) : -1;
            if (pct < 0) {
                pct = scale > 0 ? (level * 100 + scale / 2) / scale : -1;
            }
            int status = intent.getIntExtra(BatteryManager.EXTRA_STATUS, -1);
            boolean charging = status == BatteryManager.BATTERY_STATUS_CHARGING
                            || status == BatteryManager.BATTERY_STATUS_FULL;
            updateBatteryStatus(pct, charging);
        }
    };

    /**
     * 动态加载并执行 DEX 文件
     *
     * @param dexPath   DEX 文件路径
     * @param className 要加载的类全名
     * @param methodName 要调用的方法名
     * @param args      方法参数（可选）
     */
    private Object executeDex(String dexPath, String className, String methodName, Object... args) {
        Log.i(TAG, "执行 DEX: " + dexPath + " -> " + className + "." + methodName);
        try {
            DexClassLoader loader = mDexLoaders.computeIfAbsent(dexPath, path -> new DexClassLoader(
                    path,
                    getCacheDir().getAbsolutePath(),
                    null,
                    getClassLoader()
            ));
            Class<?> clazz = loader.loadClass(className);
            Object instance = clazz.getDeclaredConstructor().newInstance();
            Class<?>[] paramTypes = new Class<?>[args.length];
            for (int i = 0; i < args.length; i++) {
                paramTypes[i] = args[i] != null ? args[i].getClass() : Object.class;
            }
            Method method = clazz.getMethod(methodName, paramTypes);
            return method.invoke(instance, args);
        } catch (Exception e) {
            Log.e(TAG, "DEX 执行失败", e);
            return null;
        }
    }

    /**
     * JNI 回调桥接：Go 侧 $执行DEX$ 词库函数通过此方法调用 Java 层。
     * 参数 argsJson 为 JSON 数组字符串，如 "[\"arg1\", 123]"。
     */
    @SuppressWarnings("unused")
    private String executeDexBridge(String dexPath, String className,
                                    String methodName, String argsJson) {
        // 解析参数
        Object[] args = new Object[0];
        if (argsJson != null && !argsJson.isEmpty()) {
            try {
                org.json.JSONArray arr = new org.json.JSONArray(argsJson);
                args = new Object[arr.length()];
                for (int i = 0; i < arr.length(); i++) {
                    args[i] = arr.get(i);
                }
            } catch (Exception e) {
                Log.e(TAG, "DEX 参数解析失败", e);
                return "参数解析失败: " + e.getMessage();
            }
        }

        Object result = executeDex(dexPath, className, methodName, args);
        return result != null ? result.toString() : "null";
    }

    /**
     * JNI 回调桥接：Go 侧 $发送通知$ 词库函数通过此方法发送系统通知。
     */
    @SuppressWarnings("unused")
    private void sendNotificationBridge(String title, String content) {
        Log.i(TAG, "发送通知: " + title + " - " + content);

        NotificationManager nm = (NotificationManager) getSystemService(Context.NOTIFICATION_SERVICE);
        if (nm == null) return;

        // 创建通知渠道（已存在则无操作）
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            NotificationChannel channel = new NotificationChannel(
                    NOTIFY_CHANNEL_ID,
                    NOTIFY_CHANNEL_NAME,
                    NotificationManager.IMPORTANCE_DEFAULT
            );
            nm.createNotificationChannel(channel);
        }

        // 点击通知回到 MainActivity
        Intent intent = new Intent(this, MainActivity.class);
        intent.setFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP);
        PendingIntent pendingIntent = PendingIntent.getActivity(
                this, 0, intent,
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT
        );

        Notification notification = new Notification.Builder(this, NOTIFY_CHANNEL_ID)
                .setContentTitle(title)
                .setContentText(content)
                .setSmallIcon(android.R.drawable.ic_menu_manage)
                .setContentIntent(pendingIntent)
                .setAutoCancel(true)
                .build();

        int notifyId = mNotifyIdCounter.incrementAndGet();
        nm.notify(notifyId, notification);
    }

    /**
     * JNI 回调桥接：Go 侧 $Shizuku检查$ 查询 Shizuku 服务状态。
     * 返回 JSON：{"available":bool,"granted":bool,"version":int}
     */
    @SuppressWarnings("unused")
    private String checkShizukuBridge() {
        try {
            boolean available = Shizuku.pingBinder();
            boolean granted = false;
            int version = 0;

            if (available) {
                granted = Shizuku.checkSelfPermission() == PackageManager.PERMISSION_GRANTED;
                version = Shizuku.getVersion();
            }

            return "{\"available\":" + available
                    + ",\"granted\":" + granted
                    + ",\"version\":" + version + "}";
        } catch (Exception e) {
            Log.e(TAG, "Shizuku 状态检查失败", e);
            return "{\"available\":false,\"granted\":false,\"version\":0}";
        }
    }

    /**
     * JNI 回调桥接：Go 侧 $Shizuku执行$ 使用 Shizuku 提权执行 Shell 命令。
     * 返回命令的 stdout+stderr，末尾附加 [exit=退出码]。
     */
    @SuppressWarnings("unused")
    private String executeShizukuBridge(String command) {
        // 检查 Shizuku 服务是否运行
        if (!Shizuku.pingBinder()) {
            return "错误: Shizuku 服务未运行，请先启动 Shizuku App";
        }

        // 检查是否已授权
        if (Shizuku.checkSelfPermission() != PackageManager.PERMISSION_GRANTED) {
            return "错误: 未获得 Shizuku 授权，请在 Shizuku App 中授权 Nebula";
        }

        Log.i(TAG, "Shizuku 执行: " + command);

        Process p = null;
        Thread t1 = null;
        Thread t2 = null;
        try {
            // 使用 Shizuku 提权创建进程（以 shell UID 运行）
            p = Shizuku.newProcess(
                    new String[]{"sh", "-c", command}, null, null);
            if (p == null) {
                return "错误: Shizuku 创建进程失败，请检查 Shizuku 服务状态";
            }

            // 双线程分别读取 stdout 和 stderr，防止管道阻塞导致死锁
            java.io.ByteArrayOutputStream out = new java.io.ByteArrayOutputStream();
            t1 = new Thread(() -> {
                try {
                    byte[] buf = new byte[4096];
                    int n;
                    java.io.InputStream is = p.getInputStream();
                    while ((n = is.read(buf)) != -1) {
                        out.write(buf, 0, n);
                    }
                } catch (Exception ignored) {}
            }, "shizuku-stdout");
            t2 = new Thread(() -> {
                try {
                    byte[] buf = new byte[4096];
                    int n;
                    java.io.InputStream es = p.getErrorStream();
                    while ((n = es.read(buf)) != -1) {
                        out.write(buf, 0, n);
                    }
                } catch (Exception ignored) {}
            }, "shizuku-stderr");

            t1.start();
            t2.start();

            try {
                t1.join();
                t2.join();
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt(); // 恢复中断标志
                t1.interrupt();
                t2.interrupt();
                return "错误: Shizuku 执行被中断";
            }

            p.waitFor();
            String output = out.toString("UTF-8").trim();
            int exitCode = p.exitValue();

            if (exitCode == 0) {
                return output.isEmpty() ? "[ok]" : output;
            }
            return (output.isEmpty() ? "" : output + "\n") + "[exit=" + exitCode + "]";
        } catch (Exception e) {
            Log.e(TAG, "Shizuku 执行失败", e);
            return "Shizuku 执行异常: " + e.getMessage();
        } finally {
            if (p != null) {
                // 确保进程不会泄漏：终止未完成的读线程并销毁进程
                if (t1 != null && t1.isAlive()) t1.interrupt();
                if (t2 != null && t2.isAlive()) t2.interrupt();
                p.destroy();
            }
        }
    }

    /**
     * 启动前台保活 Service
     */
    private void startKeepAliveService() {
        Intent intent = new Intent(this, NebulaService.class);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(intent);
        } else {
            startService(intent);
        }
        Log.i(TAG, "保活服务已启动");
    }

    // ---------- 生命周期 ----------

    @Override
    protected void onResume() {
        super.onResume();
        if (mWebView != null) mWebView.onResume();
        // 从设置页返回时重新检查权限（MANAGE_EXTERNAL_STORAGE 等）
        if (!mServiceStarted) checkPermissionsAndStart();
        // 注册电量监听
        IntentFilter filter = new IntentFilter(Intent.ACTION_BATTERY_CHANGED);
        registerReceiver(mBatteryReceiver, filter);
    }

    @Override
    protected void onPause() {
        if (mWebView != null) mWebView.onPause();
        // 取消电量监听
        try { unregisterReceiver(mBatteryReceiver); } catch (Exception ignored) {}
        super.onPause();
    }

    @Override
    protected void onDestroy() {
        // 注销 Shizuku 监听
        Shizuku.removeBinderReceivedListener(mBinderReceivedListener);
        Shizuku.removeBinderDeadListener(mBinderDeadListener);
        if (mWebView != null) {
            mWebView.destroy();
        }
        super.onDestroy();
    }

    @Override
    public boolean onKeyDown(int keyCode, KeyEvent event) {
        if (keyCode == KeyEvent.KEYCODE_BACK && mWebView != null && mWebView.canGoBack()) {
            mWebView.goBack();
            return true;
        }
        // 已到 WebView 根页面：退到后台而非关闭（保活服务继续运行）
        if (keyCode == KeyEvent.KEYCODE_BACK) {
            moveTaskToBack(true);
            return true;
        }
        return super.onKeyDown(keyCode, event);
    }
}

package com.cjxpj.nebula;

import android.Manifest;
import android.annotation.SuppressLint;
import android.app.Activity;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.PowerManager;
import android.provider.Settings;
import android.util.Log;
import android.view.KeyEvent;
import android.webkit.WebChromeClient;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;

/**
 * 主 Activity —— WebView 显示 Go HTTP 服务的管理面板
 *
 * 流程：
 *   1. 请求必需权限（通知 + 电池优化）
 *   2. setDataDir() → RunNebula() → 等待 HTTP
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

    // ---------- JNI 原生方法 ----------
    // 传递应用内部存储目录给 Go 侧（解决 Android 11+ 存储权限问题）
    private native void setDataDir(String dir);

    // 对应 Go: Java_com_cjxpj_nebula_MainActivity_RunNebula
    private native void RunNebula();

    // 对应 Go: Java_com_cjxpj_nebula_MainActivity_getOpuiUrl
    // 返回 opui 访问路径，如 "/nebula"
    private native String getOpuiUrl();

    @SuppressLint("SetJavaScriptEnabled")
    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // 创建 WebView（先显示加载提示）
        mWebView = new WebView(this);
        setContentView(mWebView);

        WebSettings settings = mWebView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setCacheMode(WebSettings.LOAD_NO_CACHE);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_ALWAYS_ALLOW);

        mWebView.setWebViewClient(new WebViewClient() {
            @Override
            public void onReceivedError(WebView view, int errorCode,
                                        String description, String failingUrl) {
                Log.e(TAG, "WebView 加载失败: " + errorCode + " " + description);
                showError("页面加载失败 (" + errorCode + ")");
            }
        });
        mWebView.setWebChromeClient(new WebChromeClient());

        // 显示加载中页面
        mWebView.loadData(
                "<html><body style='display:flex;align-items:center;justify-content:center;height:100vh;font-family:sans-serif;background:#1a1a2e;color:#eee'><div style='text-align:center'><h2>Nebula</h2><p>正在启动服务...</p></div></body></html>",
                "text/html", "UTF-8");

        // 先请求必需权限，再后台启动
        requestRequiredPermissions();
    }

    // ---------- 权限请求 ----------

    /**
     * Android 13+ 通知权限 + 电池优化白名单
     */
    private void requestRequiredPermissions() {
        // 防重复启动
        if (mServiceStarted) return;

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
        // 0) 传递内部存储路径给 Go（避免 Android 11+ 存储权限问题）
        String dataDir = getFilesDir().getParentFile().getAbsolutePath();
        Log.i(TAG, "设置数据目录: " + dataDir);
        setDataDir(dataDir);

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

        // 3) 启动前台保活服务 + 请求电池优化
        startKeepAliveService();
        mHandler.post(this::requestBatteryOptimizationExemption);

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
    }

    @Override
    protected void onPause() {
        if (mWebView != null) mWebView.onPause();
        super.onPause();
    }

    @Override
    protected void onDestroy() {
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

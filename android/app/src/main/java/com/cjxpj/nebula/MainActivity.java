package com.cjxpj.nebula;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
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
 *   1. setDataDir() 传递应用内部存储路径给 Go 侧
 *   2. RunNebula() 启动 Go HTTP 服务（loadConfig 写入配置文件）
 *   3. 轮询等待 HTTP 服务就绪
 *   4. getOpuiUrl() 从 Go 侧获取面板路径
 *   5. WebView 加载
 */
public class MainActivity extends Activity {

    private static final String TAG = "MainActivity";
    private static final String HTTP_HOST = "http://127.0.0.1:8080";

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

        mWebView.setWebViewClient(new WebViewClient());
        mWebView.setWebChromeClient(new WebChromeClient());

        // 显示加载中页面
        mWebView.loadData(
                "<html><body style='display:flex;align-items:center;justify-content:center;height:100vh;font-family:sans-serif;background:#1a1a2e;color:#eee'><div style='text-align:center'><h2>Nebula</h2><p>正在启动服务...</p></div></body></html>",
                "text/html", "UTF-8");

        // 后台启动 Go HTTP 服务
        new Thread(this::startAndLoad, "nebula-boot").start();
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

        // 3) 从 so 直接获取 opui 路径
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

        // 4) 加载到 WebView
        mHandler.post(() -> mWebView.loadUrl(url));
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
        String html = "<html><body style='display:flex;align-items:center;justify-content:center;height:100vh;font-family:sans-serif;background:#1a1a2e;color:#e74c3c'><div style='text-align:center;padding:20px'><h2>错误</h2><p>" + msg + "</p></div></body></html>";
        mHandler.post(() -> mWebView.loadData(html, "text/html", "UTF-8"));
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
        return super.onKeyDown(keyCode, event);
    }
}

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

import java.io.BufferedReader;
import java.io.File;
import java.io.FileInputStream;
import java.io.InputStreamReader;

/**
 * 主 Activity —— 加载 .so 启动 Go HTTP 服务，然后用 WebView 显示 opui 管理面板
 *
 * 流程：
 *   1. 后台线程加载 .so 并调用 RunNebula() 启动 Go HTTP 服务
 *   2. 轮询等待 HTTP 服务就绪
 *   3. 读取 system.ini 获取 opui 访问路径
 *   4. WebView 加载 opui 管理面板
 */
public class MainActivity extends Activity {

    private static final String TAG = "MainActivity";
    private static final String HTTP_HOST = "http://127.0.0.1:8080";

    private WebView mWebView;
    private final Handler mHandler = new Handler(Looper.getMainLooper());

    // ---------- JNI 原生方法 ----------
    // 对应 Go 导出: Java_com_cjxpj_nebula_MainActivity_RunNebula
    // 内部调用 dic.Start()，启动 HTTP 服务并阻塞在事件循环
    private native void RunNebula();

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
        // 1) 启动 Go HTTP 服务（阻塞式，RunNebula 内部 go dic.Start() + 事件循环）
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
            showError("HTTP 服务启动超时，请检查 .so 是否正确部署");
            return;
        }

        // 3) 读取 opui 路径
        String opuiPath = readOpuiPath();
        String url = HTTP_HOST + opuiPath + "/index.html";
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
                // 服务尚未就绪
            }
            try { Thread.sleep(1000); } catch (InterruptedException ignored) {}
        }
        return false;
    }

    /**
     * 从 system.ini 解析 opui 访问路径
     *
     * INI 格式：
     *   [管理面板]
     *   启用 = true
     *   访问路径 = nebula
     */
    private String readOpuiPath() {
        File iniFile = new File(getFilesDir().getParentFile(), "private/system/system.ini");
        if (!iniFile.exists()) {
            Log.w(TAG, "system.ini 不存在于 " + iniFile.getAbsolutePath());
            return "/nebula";
        }

        try (BufferedReader reader = new BufferedReader(
                new InputStreamReader(new FileInputStream(iniFile)))) {
            boolean inSection = false;
            String line;
            while ((line = reader.readLine()) != null) {
                line = line.trim();
                if (line.isEmpty() || line.startsWith("#") || line.startsWith(";")) continue;

                if (line.startsWith("[")) {
                    inSection = line.equals("[管理面板]");
                    continue;
                }

                if (inSection && line.contains("=")) {
                    int idx = line.indexOf('=');
                    String key = line.substring(0, idx).trim();
                    String value = line.substring(idx + 1).trim();
                    if ("访问路径".equals(key)) {
                        Log.d(TAG, "opui 路径: /" + value);
                        return "/" + value;
                    }
                }
            }
        } catch (Exception e) {
            Log.e(TAG, "读取 system.ini 失败", e);
        }
        return "/nebula";
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

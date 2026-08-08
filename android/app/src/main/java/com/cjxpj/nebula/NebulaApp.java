package com.cjxpj.nebula;

import android.app.Application;
import android.util.Log;

/**
 * Application 类 —— 应用初始化
 */
public class NebulaApp extends Application {

    private static final String TAG = "NebulaApp";

    @Override
    public void onCreate() {
        super.onCreate();
        Log.i(TAG, "Nebula 应用启动");
    }
}

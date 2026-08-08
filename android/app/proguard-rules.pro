# Nebula ProGuard 规则

# 保留 JNI 方法
-keepclasseswithmembernames class * {
    native <methods>;
}

# 保留 WebView 相关
-keep class * extends android.webkit.WebViewClient
-keep class * extends android.webkit.WebChromeClient

# 保留 Native 方法所在的类
-keep class com.cjxpj.nebula.MainActivity {
    native <methods>;
}

//go:build so && (arm64 || arm)

package main

//#include <jni.h>
//#include <stdlib.h>
//#include <string.h>
//
//static jstring newJString(JNIEnv* env, const char* str) {
//    return (*env)->NewStringUTF(env, str);
//}
//static const char* getChars(JNIEnv* env, jstring str) {
//    return (*env)->GetStringUTFChars(env, str, NULL);
//}
//static void releaseChars(JNIEnv* env, jstring str, const char* chars) {
//    (*env)->ReleaseStringUTFChars(env, str, chars);
//}
//
//// ---- JVM 缓存（供 Go 协程回调 Java） ----
//static JavaVM* gJVM = NULL;
//static jobject gActivity = NULL;
//
//static void cacheJVM(JNIEnv* env, jobject activity) {
//    if (gJVM == NULL) {
//        (*env)->GetJavaVM(env, &gJVM);
//        gActivity = (*env)->NewGlobalRef(env, activity);
//    }
//}
//
//// 从任意线程回调 Java 的 executeDexBridge，返回 malloc 的字符串（Go 侧负责 free）
//static char* callExecuteDex(const char* dexPath, const char* className,
//                            const char* methodName, const char* argsJson) {
//    if (gJVM == NULL) return NULL;
//
//    JNIEnv* env;
//    jint ret = (*gJVM)->GetEnv(gJVM, (void**)&env, JNI_VERSION_1_6);
//    int attached = 0;
//    if (ret == JNI_EDETACHED) {
//        (*gJVM)->AttachCurrentThread(gJVM, &env, NULL);
//        attached = 1;
//    }
//
//    jclass cls = (*env)->GetObjectClass(env, gActivity);
//    jmethodID mid = (*env)->GetMethodID(env, cls, "executeDexBridge",
//        "(Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;");
//    if (mid == NULL) {
//        if (attached) (*gJVM)->DetachCurrentThread(gJVM);
//        return NULL;
//    }
//
//    jstring jDex   = (*env)->NewStringUTF(env, dexPath);
//    jstring jClass = (*env)->NewStringUTF(env, className);
//    jstring jMeth  = (*env)->NewStringUTF(env, methodName);
//    jstring jArgs  = (*env)->NewStringUTF(env, argsJson);
//
//    jobject result = (*env)->CallObjectMethod(env, gActivity, mid, jDex, jClass, jMeth, jArgs);
//
//    char* copy = NULL;
//    if (result != NULL) {
//        const char* chars = (*env)->GetStringUTFChars(env, (jstring)result, NULL);
//        copy = (char*)malloc(strlen(chars) + 1);
//        strcpy(copy, chars);
//        (*env)->ReleaseStringUTFChars(env, (jstring)result, chars);
//    }
//
//    (*env)->DeleteLocalRef(env, jDex);
//    (*env)->DeleteLocalRef(env, jClass);
//    (*env)->DeleteLocalRef(env, jMeth);
//    (*env)->DeleteLocalRef(env, jArgs);
//
//    if (attached) (*gJVM)->DetachCurrentThread(gJVM);
//    return copy;
//}
//
//// 从任意线程回调 Java 的 sendNotificationBridge，显示系统通知
//static int callSendNotification(const char* title, const char* content) {
//    if (gJVM == NULL) return -1;
//
//    JNIEnv* env;
//    jint ret = (*gJVM)->GetEnv(gJVM, (void**)&env, JNI_VERSION_1_6);
//    int attached = 0;
//    if (ret == JNI_EDETACHED) {
//        (*gJVM)->AttachCurrentThread(gJVM, &env, NULL);
//        attached = 1;
//    }
//
//    jclass cls = (*env)->GetObjectClass(env, gActivity);
//    jmethodID mid = (*env)->GetMethodID(env, cls, "sendNotificationBridge",
//        "(Ljava/lang/String;Ljava/lang/String;)V");
//    if (mid == NULL) {
//        if (attached) (*gJVM)->DetachCurrentThread(gJVM);
//        return -1;
//    }
//
//    jstring jTitle   = (*env)->NewStringUTF(env, title);
//    jstring jContent = (*env)->NewStringUTF(env, content);
//
//    (*env)->CallVoidMethod(env, gActivity, mid, jTitle, jContent);
//
//    (*env)->DeleteLocalRef(env, jTitle);
//    (*env)->DeleteLocalRef(env, jContent);
//
//    if (attached) (*gJVM)->DetachCurrentThread(gJVM);
//    return 0;
//}
//
//// 从任意线程回调 Java 的 checkShizukuBridge，返回 Shizuku 状态 JSON（malloc 的字符串）
//static char* callCheckShizuku() {
//    if (gJVM == NULL) return NULL;
//
//    JNIEnv* env;
//    jint ret = (*gJVM)->GetEnv(gJVM, (void**)&env, JNI_VERSION_1_6);
//    int attached = 0;
//    if (ret == JNI_EDETACHED) {
//        (*gJVM)->AttachCurrentThread(gJVM, &env, NULL);
//        attached = 1;
//    }
//
//    jclass cls = (*env)->GetObjectClass(env, gActivity);
//    jmethodID mid = (*env)->GetMethodID(env, cls, "checkShizukuBridge",
//        "()Ljava/lang/String;");
//    if (mid == NULL) {
//        if (attached) (*gJVM)->DetachCurrentThread(gJVM);
//        return NULL;
//    }
//
//    jobject result = (*env)->CallObjectMethod(env, gActivity, mid);
//
//    char* copy = NULL;
//    if (result != NULL) {
//        const char* chars = (*env)->GetStringUTFChars(env, (jstring)result, NULL);
//        copy = (char*)malloc(strlen(chars) + 1);
//        strcpy(copy, chars);
//        (*env)->ReleaseStringUTFChars(env, (jstring)result, chars);
//    }
//
//    if (attached) (*gJVM)->DetachCurrentThread(gJVM);
//    return copy;
//}
//
//// 从任意线程回调 Java 的 executeShizukuBridge，使用 Shizuku 提权执行命令（malloc 的字符串）
//static char* callExecuteShizuku(const char* command) {
//    if (gJVM == NULL) return NULL;
//
//    JNIEnv* env;
//    jint ret = (*gJVM)->GetEnv(gJVM, (void**)&env, JNI_VERSION_1_6);
//    int attached = 0;
//    if (ret == JNI_EDETACHED) {
//        (*gJVM)->AttachCurrentThread(gJVM, &env, NULL);
//        attached = 1;
//    }
//
//    jclass cls = (*env)->GetObjectClass(env, gActivity);
//    jmethodID mid = (*env)->GetMethodID(env, cls, "executeShizukuBridge",
//        "(Ljava/lang/String;)Ljava/lang/String;");
//    if (mid == NULL) {
//        if (attached) (*gJVM)->DetachCurrentThread(gJVM);
//        return NULL;
//    }
//
//    jstring jCommand = (*env)->NewStringUTF(env, command);
//
//    jobject result = (*env)->CallObjectMethod(env, gActivity, mid, jCommand);
//
//    char* copy = NULL;
//    if (result != NULL) {
//        const char* chars = (*env)->GetStringUTFChars(env, (jstring)result, NULL);
//        copy = (char*)malloc(strlen(chars) + 1);
//        strcpy(copy, chars);
//        (*env)->ReleaseStringUTFChars(env, (jstring)result, chars);
//    }
//
//    (*env)->DeleteLocalRef(env, jCommand);
//
//    if (attached) (*gJVM)->DetachCurrentThread(gJVM);
//    return copy;
//}
import "C"
import (
	"fmt"
	"log"
	"unsafe"

	"github.com/cjxpj/nebula/dic"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/mobile"
)

func init() {
	// 只在编译 so 时注册手机端专属函数，Windows/Linux 端不受影响
	funcs.Registers(
		dto.RegisterDicFunc{Name: "设备信息", L: "0", Fn: funcs.DicDeviceInfo},
		dto.RegisterDicFunc{Name: "设备电量", L: "0", Fn: funcs.DicBattery},
		dto.RegisterDicFunc{Name: "发送通知", L: "2", Fn: funcs.DicSendNotification},
		dto.RegisterDicFunc{Name: "执行DEX", L: "3|4", Fn: funcs.DicExecuteDex},
		dto.RegisterDicFunc{Name: "Shizuku检查", L: "0", Fn: funcs.DicShizukuCheck},
		dto.RegisterDicFunc{Name: "Shizuku执行", L: "1", Fn: funcs.DicShizukuExec},
	)

	// 设置回调桥接
	funcs.DexCallback = dexCallback
	mobile.SendNotificationFunc = sendNotificationCallback
	funcs.ShizukuCheckCallback = shizukuCheckCallback
	funcs.ShizukuExecCallback = shizukuExecCallback
}

// dexCallback 从 Go 协程回调 Java executeDexBridge
func dexCallback(dexPath, className, methodName, argsJson string) (string, error) {
	cDexPath := C.CString(dexPath)
	cClassName := C.CString(className)
	cMethodName := C.CString(methodName)
	cArgsJson := C.CString(argsJson)
	defer C.free(unsafe.Pointer(cDexPath))
	defer C.free(unsafe.Pointer(cClassName))
	defer C.free(unsafe.Pointer(cMethodName))
	defer C.free(unsafe.Pointer(cArgsJson))

	result := C.callExecuteDex(cDexPath, cClassName, cMethodName, cArgsJson)
	if result == nil {
		return "", fmt.Errorf("DEX 执行失败")
	}
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result), nil
}

// sendNotificationCallback 从 Go 协程回调 Java sendNotificationBridge，发送系统通知
func sendNotificationCallback(title, content string) error {
	cTitle := C.CString(title)
	cContent := C.CString(content)
	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cContent))

	ret := C.callSendNotification(cTitle, cContent)
	if ret != 0 {
		return fmt.Errorf("发送通知失败")
	}
	log.Printf("[nebula] 通知已发送: %s - %s", title, content)
	return nil
}

// shizukuCheckCallback 从 Go 协程回调 Java checkShizukuBridge，返回 Shizuku 状态 JSON
func shizukuCheckCallback() (string, error) {
	result := C.callCheckShizuku()
	if result == nil {
		return "", fmt.Errorf("Shizuku 状态检查失败")
	}
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result), nil
}

// shizukuExecCallback 从 Go 协程回调 Java executeShizukuBridge，使用 Shizuku 提权执行命令
func shizukuExecCallback(command string) (string, error) {
	cCommand := C.CString(command)
	defer C.free(unsafe.Pointer(cCommand))

	result := C.callExecuteShizuku(cCommand)
	if result == nil {
		return "", fmt.Errorf("Shizuku 执行失败")
	}
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result), nil
}

// setDeviceInfo 接收 Java 侧采集的设备信息 JSON
//
//export Java_com_cjxpj_nebula_MainActivity_setDeviceInfo
func Java_com_cjxpj_nebula_MainActivity_setDeviceInfo(env *C.JNIEnv, obj C.jobject, info C.jstring) {
	C.cacheJVM(env, obj) // 首次调用即缓存 JVM
	chars := C.getChars(env, info)
	mobile.SetDeviceInfo(C.GoString(chars))
	C.releaseChars(env, info, chars)
	log.Println("[nebula] 设备信息:", mobile.GetDeviceInfo())
}

// updateBatteryStatus 接收 Java 侧电量回调
//
//export Java_com_cjxpj_nebula_MainActivity_updateBatteryStatus
func Java_com_cjxpj_nebula_MainActivity_updateBatteryStatus(env *C.JNIEnv, obj C.jobject, level C.jint, charging C.jboolean) {
	mobile.UpdateBattery(int(level), charging == C.JNI_TRUE)
}

// registerDevice 手机端专属注册（Java 在服务启动后调用）
//
//export Java_com_cjxpj_nebula_MainActivity_registerDevice
func Java_com_cjxpj_nebula_MainActivity_registerDevice(env *C.JNIEnv, obj C.jobject) {
	log.Printf("[nebula] 设备注册 - info: %s, battery: %d%%, charging: %v",
		mobile.GetDeviceInfo(), mobile.GetBatteryLevel(), mobile.IsBatteryCharging())
	// TODO: 实现具体注册逻辑（如调用注册 API、绑定设备等）
}

//export Java_com_cjxpj_nebula_MainActivity_RunNebula
func Java_com_cjxpj_nebula_MainActivity_RunNebula(env *C.JNIEnv, obj C.jobject) {
	startupResult := dic.Start()
	if startupResult != "" {
		mobile.SetStartupUrl(startupResult)
	}
}

//export Java_com_cjxpj_nebula_MainActivity_getOpuiUrl
func Java_com_cjxpj_nebula_MainActivity_getOpuiUrl(env *C.JNIEnv, obj C.jobject) C.jstring {
	url := mobile.GetStartupUrl() // 优先使用词库自定义启动页
	if url == "" && dto.ServerConfig.OPUI != nil {
		url = dto.ServerConfig.OPUI.Addr
	}
	cUrl := C.CString(url)
	defer C.free(unsafe.Pointer(cUrl))
	return C.newJString(env, cUrl)
}

func main() {}

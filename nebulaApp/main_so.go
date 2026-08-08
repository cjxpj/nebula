//go:build so && (arm64 || arm)

package main

//#include <jni.h>
//#include <stdlib.h>
//
//// C helpers for JNI operations
//static jstring newJString(JNIEnv* env, const char* str) {
//    return (*env)->NewStringUTF(env, str);
//}
//static const char* getChars(JNIEnv* env, jstring str) {
//    return (*env)->GetStringUTFChars(env, str, NULL);
//}
//static void releaseChars(JNIEnv* env, jstring str, const char* chars) {
//    (*env)->ReleaseStringUTFChars(env, str, chars);
//}
import "C"
import (
	"unsafe"

	"github.com/cjxpj/nebula/dic"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

//export Java_com_cjxpj_nebula_MainActivity_setDataDir
func Java_com_cjxpj_nebula_MainActivity_setDataDir(env *C.JNIEnv, obj C.jobject, dir C.jstring) {
	chars := C.getChars(env, dir)
	cDir := C.GoString(chars)
	C.releaseChars(env, dir, chars)
	utils.SetAppDataDir(cDir)
}

//export Java_com_cjxpj_nebula_MainActivity_RunNebula
func Java_com_cjxpj_nebula_MainActivity_RunNebula(env *C.JNIEnv, obj C.jobject) {
	dic.Start()
}

//export Java_com_cjxpj_nebula_MainActivity_getOpuiUrl
func Java_com_cjxpj_nebula_MainActivity_getOpuiUrl(env *C.JNIEnv, obj C.jobject) C.jstring {
	url := ""
	if dto.ServerConfig.OPUI != nil {
		url = dto.ServerConfig.OPUI.Addr
	}
	cUrl := C.CString(url)
	defer C.free(unsafe.Pointer(cUrl))
	return C.newJString(env, cUrl)
}

func main() {}

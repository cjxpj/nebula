//go:build so && (arm64 || arm)

package main

//#include <jni.h>
import "C"
import "github.com/cjxpj/nebula/dic"

// Java_com_cjxpj_nebula_rn_11_RunNebula
// Java_com_cjxpj_nebula_MainActivity_RunNebula

//export Java_com_cjxpj_nebula_MainActivity_RunNebula
func Java_com_cjxpj_nebula_MainActivity_RunNebula(env *C.JNIEnv, obj C.jobject) {
	dic.Start()
}

func main() {}

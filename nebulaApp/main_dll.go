//go:generate goversioninfo
//go:build dll

package main

/*
#include <stdlib.h>

char* RunN_C(char* text, char* trigger, char* path);

void FreeString(char* s);
*/
import "C"
import (
	"cjxpj/nebula/dic"
	"cjxpj/nebula/juice"
)

//export RunJuice
func RunJuice() {
	juice.Run()
}

//export RunN
func RunN(text, trigger, path *C.char) *C.char {
	goFile := C.GoString(text)
	goTrigger := C.GoString(trigger)
	goPath := C.GoString(path)
	result := dic.NewDic(goFile, goTrigger, goPath).Run()
	cResult := C.CString(result)
	return cResult
}

func main() {}

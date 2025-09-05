package main

import "C"
import "github.com/cjxpj/nebula/dic"

//export NebulaRun
func NebulaRun() {
	dic.Start()
}

func main() {}

//go:build linux

package main

import "cjxpj/nebula/dic"

func main() {
	dic.Start()
	select {}
}

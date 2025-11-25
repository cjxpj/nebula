//go:build linux

package main

import "github.com/cjxpj/nebula/dic"

func main() {
	dic.Start()
	select {}
}

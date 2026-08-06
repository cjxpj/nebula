package main

import (
	"github.com/cjxpj/nebula/dic"
	"golang.org/x/mobile/app"
)

func main() {
	app.Main(func(a app.App) {
		go dic.Start()

		// 保持应用存活，处理生命周期事件
		for range a.Events() {
		}
	})
}

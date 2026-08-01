//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/cjxpj/nebula/dic"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("收到退出信号，准备关闭...")
		cancel()
	}()

	funcs.Register("打开浏览器", "1", func(d *dto.DicInputs) (any, error) {
		err := openBrowser(d.Inputs.String(1))
		return "", err
	})

	dic.Start()
	<-ctx.Done()
	fmt.Println("主程序退出")
}

// openBrowser 用系统默认浏览器打开指定 URL
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // Linux 及其他
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}

//go:build !js

package dic

import (
	"bufio"
	"errors"
	"io"

	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// 终端.监听执行
func cmdListenRun(d *dto.DicInputs) (any, error) {
	cmd := getCmdConfig(d.Inputs.Get(1))
	if cmd == nil {
		return "", errors.New("参数1终端数据错误")
	}

	// 注册动作：输出文本
	stdout, _ := cmd.Cmd.StdoutPipe()
	// 注册动作：错误文本
	stderr, _ := cmd.Cmd.StderrPipe()

	if err := cmd.Cmd.Start(); err != nil {
		debugLog.Error(err)
		return "", err
	}

	// 注册动作：断开连接
	dicpath := d.Inputs.String(2)
	cmdfileTool := utils.NewFileQueue(dicpath)
	if !cmdfileTool.FileExists() {
		return "", errors.New("请创建词库监听文件")
	}

	// 监听输出
	go func() {
		multi := io.MultiReader(stdout, stderr)
		scanner := bufio.NewScanner(multi)
		for scanner.Scan() {
			raw := scanner.Bytes()
			line, _ := utils.DecodeType(cmd.Decoder, raw)

			// 重新读取
			cmdfile, _ := cmdfileTool.ReadFromFile()
			dd := dic_dto.NewDic(dicpath, cmdfile)
			dd.SetFunc("断开连接", dto.DicFunc{
				L: "0",
				Fn: func(d *dto.DicInputs) (any, error) {
					cmd.Cmd.Process.Kill()
					return "", nil
				}})
			dd.SetFunc("输入文本", dto.DicFunc{
				L: "1",
				Fn: func(d *dto.DicInputs) (any, error) {
					text := d.Inputs.String(1)
					_, err := cmd.Stdin.Write([]byte(text))
					return "", err
				}})

			if res := dic_api.Api.DicRun(dd, line); res != "" {
				debugLog.Infof("%v", res)
			}
		}
		if err := scanner.Err(); err != nil {
			debugLog.Infof("终端断开: %v", err)
		}
	}()
	return "", nil
}

// getCmdConfig 从参数中提取终端配置，兼容直接传入 *CmdConfig 或面对像 Class 实例。
func getCmdConfig(v any) *funcs.CmdConfig {
	switch t := v.(type) {
	case *funcs.CmdConfig:
		return t
	case *dto.DicClass:
		if raw := t.LocalValue.Get("_终端_"); raw != nil {
			if cmd, ok := raw.(*funcs.CmdConfig); ok {
				return cmd
			}
		}
	}
	return nil
}

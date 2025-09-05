package funcs

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"

	"github.com/cjxpj/nebula/utils"
)

// 终端结构
type CmdConfig struct {
	Cmd *exec.Cmd
	// 解码器
	Decoder string
}

// 终端输入
func (f *DicFunc) RunCommandInput() (string, error) {
	if f.Len != 0 {
		return "", errors.New("参数错误")
	}
	var inputText string
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		inputText = scanner.Text()
	}

	if err := scanner.Err(); err != nil {
		inputText = "error"
	}

	return inputText, nil
}

// 新建终端
func (f *DicFunc) RunCommandNew() (any, error) {
	if f.Len == 0 {
		return "", errors.New("缺少参数")
	}

	var cmd *exec.Cmd

	if f.Len > 1 {
		var args []string
		for _, arg := range f.Inputs.List[2:] {
			if strArg, ok := arg.(string); ok {
				args = append(args, strArg)
			}
		}
		cmd = exec.Command(f.Inputs.String(1), args...)
	} else {
		cmd = exec.Command(f.Inputs.String(1))
	}
	cmdConfig := &CmdConfig{
		Cmd:     cmd,
		Decoder: "utf-8",
	}
	return cmdConfig, nil
}

// 终端解码器
func (f *DicFunc) RunCommandDecoder() (string, error) {
	if !f.Inputs.LenOk(2) {
		return "", errors.New("参数错误")
	}

	cmd, ok := f.Inputs.Get(1).(*CmdConfig)
	if !ok {
		return "", errors.New("传入参数错误")
	}

	cmd.Decoder = f.Inputs.String(2)
	return "", nil
}

// 终端变量
func (f *DicFunc) RunCommandVar() (string, error) {
	if !f.Inputs.LenOk(2) {
		return "", errors.New("参数错误")
	}

	cmd, ok := f.Inputs.Get(1).(*CmdConfig)
	if !ok {
		return "", errors.New("传入参数错误")
	}

	cmd.Cmd.Env = append(cmd.Cmd.Env, f.Inputs.String(2))

	return "", nil
}

// 断开终端
func (f *DicFunc) RunCommandClose() (string, error) {
	if !f.Inputs.LenOk(1) {
		return "", errors.New("参数错误")
	}

	cmd, ok := f.Inputs.Get(1).(*CmdConfig)
	if !ok {
		return "", errors.New("传入参数错误")
	}

	if err := cmd.Cmd.Process.Kill(); err != nil {
		return "false", nil
	}
	return "true", nil
}

// 终端输入
func (f *DicFunc) RunCommandInputText() (string, error) {
	if !f.Inputs.LenOk(2) {
		return "", errors.New("参数错误")
	}

	cmd, ok := f.Inputs.Get(1).(*CmdConfig)
	if !ok {
		return "", errors.New("传入参数错误")
	}

	// 检测启动
	if err := cmd.Cmd.Start(); err != nil {
		return "未启动", nil
	}

	if stdin, err := cmd.Cmd.StdinPipe(); err != nil {
		stdin.Write([]byte(f.Inputs.String(2)))
	}
	return "", nil
}

// 执行终端
func (f *DicFunc) RunCommand() (string, error) {
	if !f.Inputs.LenOk(1) {
		return "", errors.New("参数错误")
	}

	cmd, ok := f.Inputs.Get(1).(*CmdConfig)
	if !ok {
		return "", errors.New("传入参数错误")
	}

	var out bytes.Buffer
	cmd.Cmd.Stdout = &out
	cmd.Cmd.Stderr = &out

	err := cmd.Cmd.Run()
	rawBytes := out.Bytes()

	// 解码
	str, _ := utils.DecodeType(cmd.Decoder, rawBytes)

	if err != nil {
		return err.Error(), nil
	}
	return str, nil
}

package funcs

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"

	"github.com/cjxpj/nebula/debugLog"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// 终端结构
type CmdConfig struct {
	Cmd *exec.Cmd
	// 写入端
	Stdin io.WriteCloser
	// 解码器
	Decoder string
}

// 终端输入
func runCommandInput(d *dto.DicInputs) (any, error) {
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
func runCommandNew(d *dto.DicInputs) (any, error) {
	var cmd *exec.Cmd

	if d.Inputs.Len() > 1 {
		var args []string
		for _, arg := range d.Inputs.List[2:] {
			if strArg, ok := arg.(string); ok {
				args = append(args, strArg)
			}
		}
		cmd = exec.Command(d.Inputs.String(1), args...)
	} else {
		cmd = exec.Command(d.Inputs.String(1))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	cmdConfig := &CmdConfig{
		Cmd:     cmd,
		Stdin:   stdin,
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

// 终端执行目录
func runCommandDir(d *dto.DicInputs) (any, error) {
	if !d.Inputs.LenOk(2) {
		return "", errors.New("参数错误")
	}

	cmd, ok := d.Inputs.Get(1).(*CmdConfig)
	if !ok {
		return "", errors.New("传入参数错误")
	}

	cmd.Cmd.Dir = d.Inputs.String(2)
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
	if cmd.Cmd.Process == nil {
		return "", errors.New("未启动终端")
	}

	_, err := cmd.Stdin.Write([]byte(f.Inputs.String(2)))
	if err != nil {
		return "", err
	}
	return "", nil
}

// 执行终端
func runCommand(d *dto.DicInputs) (any, error) {
	cmd, ok := d.Inputs.Get(1).(*CmdConfig)
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

// 异步执行终端
func runCommandAsync(d *dto.DicInputs) (any, error) {
	cmd, ok := d.Inputs.Get(1).(*CmdConfig)
	if !ok {
		return "", errors.New("传入参数错误")
	}

	go func() {
		if err := cmd.Cmd.Run(); err != nil {
			debugLog.Info(err)
		}
	}()
	return "", nil
}

package dic

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
)

// 执行词库
func runDic(d *dto.DicInputs) (any, error) {
	data := d.Inputs.String(1)
	if data == "" {
		return "", nil
	}
	dicPath := "执行"

	// 触发
	chufa := d.Inputs.StringDefault(2, "Main")

	// 执行模式
	dicType := d.Inputs.StringDefault(3, "独立")

	calldicrun := NewDic(dicPath, data).
		SetGlobal_v(d.V.G)
	calldicrun.MyFunc = d.Dic.MyFunc
	calldicrun.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2..") {
			return "", errors.New("调用参数错误")
		}
		go func() {
			sleepTime := inputs.Int(1)
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			rMsg := calldicrun.RunPrivate(inputs.StringAfter(2))
			if rMsg != "" {
				fmt.Println(rMsg)
			}
		}()
		return "", nil
	})
	calldicrun.ClassText = d.Dic.LocalClass
	calldicrun.Val.P.Set("_词库路径_", dicPath)

	switch dicType {
	case "继承":
		fv := dto.NewVal()
		fv.Reset(d.V.P.GetAll())
		fv.Set("_词库路径_", dicPath)
		calldicrun.Set_v(fv)
		calldicrun.FuncText = d.Dic.LocalFunc
	case "继承函数":
		calldicrun.FuncText = d.Dic.LocalFunc
	case "互通":
		d.V.P.Set("_词库路径_", dicPath)
		calldicrun.Set_v(d.V.P)
		calldicrun.FuncText = d.Dic.LocalFunc
	}

	DicRes := calldicrun.Run(chufa)
	return DicRes, nil
}

// 执行词库文件
func runDicFile(d *dto.DicInputs) (any, error) {
	dicPath := d.Inputs.String(1)
	data, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return "", nil
	}

	// 触发
	chufa := d.Inputs.StringDefault(2, "Main")

	// 执行模式
	dicType := d.Inputs.StringDefault(3, "独立")

	calldicrun := NewDic(dicPath, data).
		SetGlobal_v(d.V.G)
	calldicrun.MyFunc = d.Dic.MyFunc
	calldicrun.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2..") {
			return "", errors.New("调用参数错误")
		}
		go func() {
			sleepTime := inputs.Int(1)
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			rMsg := calldicrun.RunPrivate(inputs.StringAfter(2))
			if rMsg != "" {
				fmt.Println(rMsg)
			}
		}()
		return "", nil
	})
	calldicrun.ClassText = d.Dic.LocalClass
	calldicrun.Val.P.Set("_词库路径_", dicPath)

	switch dicType {
	case "继承":
		fv := dto.NewVal()
		fv.Reset(d.V.P.GetAll())
		fv.Set("_词库路径_", dicPath)
		calldicrun.Set_v(fv)
		calldicrun.FuncText = d.Dic.LocalFunc
	case "继承函数":
		calldicrun.FuncText = d.Dic.LocalFunc
	case "互通":
		d.V.P.Set("_词库路径_", dicPath)
		calldicrun.Set_v(d.V.P)
		calldicrun.FuncText = d.Dic.LocalFunc
	}

	DicRes := calldicrun.Run(chufa)
	return DicRes, nil
}

// 回调词库
func callDic(d *dto.DicInputs) (any, error) {
	var triggerParts []string
	for _, part := range d.Inputs.List[1:] {
		if strPart, ok := part.(string); ok {
			triggerParts = append(triggerParts, strPart)
		}
	}
	trigger := strings.Join(triggerParts, " ")

	// 判断是否在整合包中执行
	if classN, ok := d.V.P.Get("Class").(string); ok {
		classData := d.Dic.LocalClass[classN]
		if classData != nil {
			GetDic, GetDicTrigger, _, _ := run.RunFor(classData.LocalStatic, trigger, 0)
			funcV := dto.NewVal()
			funcV.Reset(d.V.P.GetAll())
			funcV.Set("触发词", trigger)
			funcV.Set("触发", GetDicTrigger)
			RunDics := NewRunDicEntry().
				SetGlobal_v(d.V.G).
				Set_v(funcV).
				SetDic_v(d.Dic)
			RunDic := RunDics.Run(GetDic)
			return RunDic, nil
		}
	}
	GetDic, GetDicTrigger, _, _ := run.RunFor(d.Dic.LocalStatic, trigger, 0)
	funcV := dto.NewVal()
	funcV.Reset(d.V.P.GetAll())
	funcV.Set("触发词", trigger)
	funcV.Set("触发", GetDicTrigger)
	RunDics := NewRunDicEntry().
		SetGlobal_v(d.V.G).
		Set_v(funcV).
		SetDic_v(d.Dic)
	RunDic := RunDics.Run(GetDic)
	return RunDic, nil
}

// 执行网页词库
func runWebDic(d *dto.DicInputs) (any, error) {
	data := d.Inputs.String(1)
	if data == "" {
		return "", nil
	}
	dicPath := "执行"
	webdic := NewWebDic(dicPath, data).
		SetGlobal_v(d.V.G)
	webdic.MyFunc = d.Dic.MyFunc
	webdicRes := webdic.Run()
	return webdicRes, nil
}

// 执行网页词库文件
func runWebDicFile(d *dto.DicInputs) (any, error) {
	dicPath := d.Inputs.String(1)
	data, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return "", nil
	}
	webdic := NewWebDic(dicPath, data).
		SetGlobal_v(d.V.G)
	webdic.MyFunc = d.Dic.MyFunc
	webdicRes := webdic.Run()
	return webdicRes, nil
}

// 终端.监听执行
func cmdListenRun(d *dto.DicInputs) (any, error) {
	cmd, ok := d.Inputs.Get(1).(*funcs.CmdConfig)
	if !ok {
		return "", errors.New("参数1终端数据错误")
	}

	// 注册动作：输出文本
	stdout, _ := cmd.Cmd.StdoutPipe()
	// 注册动作：错误文本
	stderr, _ := cmd.Cmd.StderrPipe()

	if err := cmd.Cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// 注册动作：断开连接
	dicpath := d.Inputs.String(2)
	cmdfile, err := utils.NewFileQueue(dicpath).ReadFromFile()
	if err != nil {
		fmt.Println("读取文件时出错，请手动创建词库监听文件:", err)
		return "", err
	}
	dd := NewDic(dicpath, cmdfile)

	dd.SetFunc("断开连接", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		cmd.Cmd.Process.Kill()
		return "", nil
	})

	dd.SetFunc("输入文本", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk(1) {
			return "参数错误", nil
		}
		text := inputs.String(1)
		_, err := cmd.Stdin.Write([]byte(text))
		return "", err
	})

	// 监听输出
	go func() {
		multi := io.MultiReader(stdout, stderr)
		scanner := bufio.NewScanner(multi)
		for scanner.Scan() {
			raw := scanner.Bytes()
			line, _ := utils.DecodeType(cmd.Decoder, raw)
			if res := dd.Run(line); res != "" {
				fmt.Println(res)
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Println("终端断开:", err)
		}
	}()
	return "", nil
}

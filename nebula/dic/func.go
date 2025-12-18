package dic

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"runtime"
	"strings"

	"github.com/cjxpj/nebula/count"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"

	_ "github.com/go-sql-driver/mysql"
)

// 函数跟变量
func (d *DicFunc) Runs(text string) string {
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		input := utils.NewDicInputs()
		input.SetString(valStr)
		resAny, err := d.Funcs(input)
		if err != nil {
			log.Printf("[%s]%s：%v", d.Val.Get("_词库路径_"), valStr[0], err)
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		return "", false
	}, func(s string) (string, bool) {
		return utils.AnyIsString(d.Val.Text(s)), false
	})
	return output
}

// 执行，函数跟变量
func (d *DicFunc) RunsAny(text string) any {
	// 拦截外部赋予值
	var resA any
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		input := utils.NewDicInputs()
		input.SetString(valStr)
		resAny, err := d.Funcs(input)
		if err != nil {
			fmt.Println(err)
		}
		if resAny == nil {
			return "", false
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		resA = resAny
		return "", true
	}, func(s string) (string, bool) {
		resAny := d.Val.Text(s)
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		resA = resAny
		return "", true
	})
	if resA != nil {
		return resA
	}
	return output
}

// 赋予值执行，函数跟变量
func (d *DicFunc) RunsVal(text string, setVal string) (string, bool) {
	// 拦截外部赋予值
	strNo := false
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		input := utils.NewDicInputs()
		input.SetString(valStr)
		resAny, err := d.Funcs(input)
		if err != nil {
			fmt.Println(err)
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		d.Val.P.Set(setVal, resAny)
		strNo = true
		return "", false
	}, func(s string) (string, bool) {
		return utils.AnyIsString(d.Val.Text(s)), false
	})
	return output, strNo
}

// 纯函数
func (d *DicFunc) Run(text string) string {
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		input := utils.NewDicInputs()
		input.SetString(valStr)
		resAny, err := d.Funcs(input)
		if err != nil {
			log.Printf("[%s]%s：%v", d.Val.Get("_词库路径_"), valStr[0], err)
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		return "", false
	}, func(s string) (string, bool) {
		return s, false
	})
	return output
}

func (d *DicFunc) Funcs(dic_i *utils.DicInputs) (any, error) {
	if dic_i.LenOk(-1) {
		return "$$", nil
	}

	// 面对象
	if className := dic_i.String(0); dic_i.LenOk("2..") && len(className) > 1 && (className[0] == '.' || className[0] == '%') {
		classType := className[0]
		className := className[1:]

		var isV bool
		if classType == '%' {
			isV = true
		}
		if className == "自己" {
			className = d.Val.P.Get("Class").(string)
		}
		classData := d.Dic.LocalClass[className]
		if classData == nil {
			return "", errors.New("非整合包")
		}

		if isV {
			if dic_i.LenOk(3) {
				resVT := utils.AnyIsString(d.Val.Text(count.RunCountText(d.Val, dic_i.String(1))))
				resVTs := utils.AnyIsString(d.Val.Text(count.RunCountText(d.Val, dic_i.String(2))))
				classData.LocalValue.Set(resVT, resVTs)
				return "", nil
			}
			if dic_i.LenOk(2) {
				resVT := utils.AnyIsString(d.Val.Text(count.RunCountText(d.Val, dic_i.String(1))))
				resV, _ := classData.LocalValue.Get(resVT).(string)
				return resV, nil
			}
			return "未知整合包变量方法", nil
		}

		if dic_i.LenOk(0, 1) {
			return "未知整合包方法", nil
		}

		TStr := strings.Join(dic_i.StringAfterList(1), " ")
		// 整合包局部函数
		if str, Tstr, _, regex := run.RunFor(classData.LocalFunc, TStr, 0); regex != nil {
			funcv := dto.NewVal()
			funcv.Set("触发", Tstr)
			funcv.Set("触发词", TStr)
			funcv.Set("Class", className)
			dto.ValRunTrigger(TStr, Tstr, d.Val.NewDicVal(funcv), d.Val)
			RunDic := NewRunDicEntry().
				CloseTrigger().
				SetGlobal_v(d.Val.G).
				Set_v(funcv).
				SetDic_v(d.Dic.Clone())
			RunDic.ClearDicFuncs()
			resRunDic := RunDic.Run(str)
			return resRunDic, nil
		}
	} else {
		text := strings.Join(dic_i.StringList(), " ")
		// 局部函数
		if str, Tstr, _, regex, tparts := run.RunFors(d.Dic.LocalFunc, text, 0); regex != nil {
			funcv := dto.NewVal()
			give, ok := d.Val.P.Get("_继承_").(string)
			if ok && give != "" {
				for _, v := range strings.Split(give, ",") {
					set, ok := d.Val.P.Get(v).(string)
					if ok {
						funcv.Set(v, set)
					}
				}
				d.Val.P.Set("_继承_", "")
			}
			funcv.Set("触发", Tstr)
			funcv.Set("触发词", text)
			dto.ValRunTrigger(text, Tstr, d.Val.NewDicVal(funcv), d.Val)
			RunDic := NewRunDicEntry().
				CloseTrigger().
				SetGlobal_v(d.Val.G).
				Set_v(funcv).
				SetDic_v(d.Dic.Clone())
			RunDic.ClearDicFuncs()

			resRunDic := RunDic.Run(str)
			if tparts != "" {
				subParts := strings.SplitSeq(tparts, ",")
				for setv := range subParts {
					getv := RunDic.Val.P.Get(setv)
					d.Val.P.Set(setv, getv)
				}
			}
			return resRunDic, nil
		}
	}

	lines := make([]any, dic_i.Len()+1)
	inputs := utils.NewDicInputs()
	inputs.Set(make([]any, dic_i.Len()+1))

	for i, line := range dic_i.List {
		lines[i] = line
		inputs.List[i] = d.Val.Text(count.RunCountText(d.Val, line))
	}

	f := &funcs.DicFunc{
		Len:       dic_i.Len(),
		InputData: dic_i.List,
		Inputs:    inputs,
	}

	// 自定义函数
	if fn, ok := d.Dic.MyFunc[dic_i.String(0)]; ok {
		return fn(d.Val, inputs)
	}

	// 系统函数
	if fnInfo, ok := funcs.GetFunc(dic_i.String(0)); ok {
		if inputs.LenOk(fnInfo.L) {
			return fnInfo.Fn(dto.NewDicInputs(d.Dic, d.Val, inputs))
		}
	}

	switch dic_i.String(0) {
	case "捕获输出":
		if dic_i.LenOk(0) {
			return d.Output.Get(), nil
		}
		return "", nil

	case "拦截输出":
		if dic_i.LenOk(0) {
			res := d.Output.Get()
			d.Output.Clear()
			return res, nil
		}
		return "", nil

	case "STOP":
		if dic_i.LenOk(0) {
			utils.LogStop(d.Output.Get())
			return "", nil
		}
		return "", nil

	case "正则替换":
		if inputs.LenOk(2, 3) {
			matcheA, err := regexp.Compile(inputs.String(2))
			if err != nil {
				return "", nil
			}
			var tStr string
			if inputs.LenOk(3) {
				tStr = inputs.String(3)
				if tStr == lines[3].(string) && strings.HasPrefix(lines[3].(string), "%") && strings.HasSuffix(lines[3].(string), "%") && strings.Count(lines[3].(string), "%") == 2 {
					var regex *regexp.Regexp
					obj := d.Val.P.GetObj(lines[3].(string)[1 : len(lines[3].(string))-1])
					if t, ok := obj["type"].(string); ok && t == "函数框" {
						funcTrigger := obj["trigger"].(string)
						regex = regexp.MustCompile("^" + funcTrigger + "$")
						res := matcheA.ReplaceAllStringFunc(inputs.String(1), func(s string) string {
							matches := regex.FindStringSubmatch(s)
							if len(matches) > 0 || funcTrigger == "" {
								funcv := dto.NewVal()
								funcv.Reset(d.Val.P.GetAll())
								funcv.Set("触发", funcTrigger)
								funcv.Set("触发词", s)
								content := obj["content"].([]string)
								RunDic := NewRunDicEntry().
									SetGlobal_v(d.Val.G).
									Set_v(funcv).
									SetDic_v(d.Dic)
								return RunDic.Run(content)
							}
							return ""
						})
						return res, nil
					}
				}
			}
			replacedText := matcheA.ReplaceAllString(inputs.String(1), tStr)
			return replacedText, nil

		}
		return "", nil

	case "终端.解码器":
		return f.RunCommandDecoder()

	case "终端.变量":
		return f.RunCommandVar()

	case "终端.断开":
		return f.RunCommandClose()

	case "终端.输入":
		return f.RunCommandInputText()

	case "MD5编码":
		return f.Md5(), nil

	case "B64编码":
		return f.Base64En(), nil

	case "B64解码":
		return f.Base64De(), nil

	case "URL编码":
		return f.UrlEn(), nil

	case "URL解码":
		return f.UrlDe(), nil

	case "URL链接编码":
		return f.UrlPathEn(), nil

	case "URL链接解码":
		return f.UrlPathDe(), nil

	case "判断值":
		return f.IfNONull(), nil

	case "判断空值":
		return f.IfNull(), nil

	case "正则匹配":
		return f.RegexpMatche(), nil

	case "正则":
		return f.Regexp(), nil

	case "加密词库":
		return f.EncodeDic(), nil

	case "大写字母":
		return f.ToUpper(), nil

	case "小写字母":
		return f.ToLower(), nil

	case "ZIP压缩":
		return f.ZipFolder(), nil

	case "ZIP解压":
		return f.UnZip(), nil

	case "文件夹大小":
		return f.DirSize(), nil

	case "文件大小":
		return f.FileSize(), nil

	case "重命名":
		return f.FileRename(), nil

	case "复制粘贴":
		return f.FileCopy(), nil

	case "计算":
		return f.Count(), nil

	case "随机数":
		return f.RandNum(), nil

	case "随机文本":
		return f.RandString(), nil

	case "随机大小字母":
		return f.RandLetter(0), nil

	case "随机大写字母":
		return f.RandLetter(1), nil

	case "随机小写字母":
		return f.RandLetter(2), nil

	case "随机大小字母数字":
		return f.RandLetter(3), nil

	case "随机小写字母数字":
		return f.RandLetter(4), nil

	case "随机大写字母数字":
		return f.RandLetter(5), nil

	case "随机数字":
		return f.RandLetter(6), nil

	case "时间戳格式化时间":
		return f.TimestampFormattingTime(), nil

	case "JSON解析":
		return f.QueryJson()

	case "JSON判断":
		return f.IsJson(), nil

	case "JSON存":
		return f.JsonSet(), nil

	case "JSON存字":
		return f.JsonSetString(), nil

	case "JSON追加":
		return f.JsonAdd(), nil

	case "JSON追加字":
		return f.JsonAddString(), nil

	case "JSON删":
		return f.JsonDelete(), nil

	case "JSON存在":
		return f.JsonIsKey(), nil

	case "JSON长度":
		return f.JsonLen(), nil

	case "JSON美化":
		return f.JsonPrettyPrint()

	case "HTML解析":
		return f.HtmlParse()

	case "HTML编码":
		return f.HtmlEncode()

	case "HTML解码":
		return f.HtmlDecode()

	case "编码":
		return f.EnUtf8(), nil

	case "解码":
		return f.DeUtf8(), nil

	case "GIF拆帧":
		return f.GetGif(), nil

	case "绘图":
		return f.DrawImg(), nil

	case "排序":
		return f.Sort(), nil

	case "范围":
		return f.Range(), nil

	case "Ed25519从种子生成密钥":
		return f.Ed25519_NewKeyFromSeed()

	case "Ed25519签名":
		return f.Ed25519_Sign()

	case "Ed25519验证签名":
		return f.Ed25519_Verify()

	case "Ed25519公钥转换为Curve25519":
		return f.Ed25519_PublicKeyToCurve25519()

	case "Ed25519私钥转换为Curve25519":
		return f.Ed25519_PrivateKeyToCurve25519()

	case "Ed25519从Curve25519生成密钥":
		return f.Ed25519_NewKeyFromCurve25519()

	case "画笔.字体":
		return nil, f.DrawImgLoadFont()

	case "画笔.大小":
		return nil, f.DrawImgSetSize()

	case "绘制.文本":
		return nil, f.DrawImgText()

	case "绘制.点":
		return nil, f.DrawImgPoint()

	case "绘制.线":
		return nil, f.DrawImgLine()

	case "绘制.喷漆":
		return nil, f.DrawImgBrushLine()

	case "绘制.波浪":
		return nil, f.DrawImgWaveLine()

	case "绘制.油漆桶":
		return nil, f.DrawImgFloodFill()

	case "绘制.方形":
		return nil, f.DrawImgRectangleFill()

	case "绘制.方形描边":
		return nil, f.DrawImgRectangleStroke()

	case "绘制.椭圆":
		return nil, f.DrawImgEllipseFill()

	case "绘制.椭圆描边":
		return nil, f.DrawImgEllipse()

	case "绘制.圆形":
		return nil, f.DrawImgPieFill()

	case "绘制.圆形描边":
		return nil, f.DrawImgPie()

	case "绘制.多边形":
		return nil, f.DrawImgPolygon()

	case "绘制.多边形描边":
		return nil, f.DrawImgPolygons()

	case "绘制.图片":
		return nil, f.DrawImgPaste()

	case "画布.旋转":
		return nil, f.DrawImgRotate()

	case "画布.圆形":
		return nil, f.DrawImgRoundCorners()

	case "绘制.随机点":
		return nil, f.DrawImgRandomDots()

	case "绘制.随机线条":
		return nil, f.DrawImgRandomLines()

	case "绘制.马赛克":
		return nil, f.DrawImgMosaic()

	case "绘制.高斯模糊":
		return nil, f.DrawImgGaussianBlur()

	case "画布.马赛克":
		return nil, f.DrawImgAllMosaic()

	case "画布.灰度":
		return nil, f.DrawImgGrayscale()

	case "绘制.圆弧":
		return nil, f.DrawImgArc()

	case "图片相似度":
		return f.ImageSimilarity()

	case "GC回收":
		runtime.GC()
		return "", nil

	case "腾讯.接口":
		return f.TencentGetApi()

	case "腾讯.调用":
		return f.TencentGetApiCall()

	}

	return "$" + strings.Join(dic_i.StringList(), " ") + "$", nil
}

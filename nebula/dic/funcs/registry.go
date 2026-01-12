package funcs

import (
	"fmt"

	"github.com/cjxpj/nebula/dto"
)

type f = dto.RegisterDicFunc

func Setup() {
	if err := Registers(
		f{Name: "文本长度", L: "1", Fn: stringSliceLen},
		f{Name: "长度", L: "1", Fn: stringLen},
		f{Name: "复读", L: "1|2", Fn: repeat},
		f{Name: "去除左右", L: "1|2", Fn: removeLR},
		f{Name: "去除左", L: "1|2", Fn: removeL},
		f{Name: "去除右", L: "1|2", Fn: removeR},
		f{Name: "字符拼接", L: "2", Fn: join},
		f{Name: "查找字", L: "2", Fn: find},
		f{Name: "取中间", L: "2|3", Fn: takeTheMiddle},
		f{Name: "截取", L: "2|3", Fn: intercept},
		f{Name: "中文转拼音", L: "1", Fn: pinYin},
		f{Name: "数字格式化", L: "2|3", Fn: numberFormatting},
		f{Name: "数字转中文", L: "1", Fn: numToString},

		f{Name: "线程变量", L: "1|2", Fn: threadVar},
		f{Name: "变量", L: "1|2", Fn: localVar},
		f{Name: "存在变量", L: "1", Fn: localVarExist},
		f{Name: "全局变量", L: "1|2", Fn: globalVar},
		f{Name: "锁变量", L: "1", Fn: localVarLock},
		f{Name: "变量文本", L: "1", Fn: localVarText},
		f{Name: "延迟", L: "1", Fn: appSleep},

		f{Name: "读", L: "1|2|3", Fn: readKeyStringFile},
		f{Name: "写", L: "2|3", Fn: writeKeyStringFile},
		f{Name: "写文件", L: "1|2", Fn: writeStringFile},
		f{Name: "读文件", L: "1|2", Fn: readStringFile},
		f{Name: "文件后缀", L: "1", Fn: fileSuffix},
		f{Name: "存在文件", L: "1", Fn: fileExist},
		f{Name: "存在文件夹", L: "1", Fn: dirExist},
		f{Name: "存在文件或文件夹", L: "1", Fn: fileOrDirExist},
		f{Name: "删除文件", L: "1", Fn: deleteFile},
		f{Name: "删除文件夹", L: "1", Fn: deleteDir},
		f{Name: "下载文件", L: "2|3|4", Fn: downloadFile},

		f{Name: "邮件", L: "6", Fn: sendMail},

		f{Name: "随机文件名", L: "0|1", Fn: randomFileName},
		f{Name: "随机文件夹名", L: "0|1", Fn: randomDirName},

		f{Name: "sqlite", L: "3..", Fn: sqliteConn},
		f{Name: "mysql", L: "3..", Fn: mysqlConn},

		f{Name: "取前字符", L: "2", Fn: subStrHead},
		f{Name: "取后字符", L: "2", Fn: subStrTail},

		f{Name: "日志", L: "1|2", Fn: logfile},
		f{Name: "打印", L: "1..", Fn: print},

		f{Name: "MD转HTML", L: "1", Fn: markdownToHtml},

		f{Name: "哈基米加密", L: "1|2", Fn: hajimimanboEncrypt},
		f{Name: "哈基米解密", L: "1|2", Fn: hajimimanboDecrypt},

		f{Name: "写图片", L: "3", Fn: writeImage},
		f{Name: "读图片", L: "2", Fn: readImage},

		f{Name: "终端.创建", L: "1..", Fn: runCommandNew},
		f{Name: "终端.异步执行", L: "1", Fn: runCommandAsync},
		f{Name: "终端.执行", L: "1", Fn: runCommand},
		f{Name: "终端等待输入", L: "0", Fn: runCommandInput},

		f{Name: "文件夹列表", L: "0|1", Fn: dirList},
		f{Name: "文件列表", L: "0|1", Fn: fileList},

		f{Name: "Ed25519种子大小", L: "0", Fn: ed25519_SeedSize},
		f{Name: "Ed25519生成密钥", L: "0", Fn: ed25519_GenerateKey},

		f{Name: "画布.创建", L: "2|3", Fn: drawImgNew},
		f{Name: "主机", L: "1", Fn: host_information},

		f{Name: "画笔.获取颜色", L: "1|2|3|4", Fn: drawImgGetColor},
		f{Name: "画笔.设置颜色", L: "1|2|3|4|5", Fn: drawImgSetColor},
		f{Name: "画布.获取", L: "1|2", Fn: drawImgGet},

		f{Name: "字典.创建", L: "0|1", Fn: newMapData},
		f{Name: "字典.设置", L: "3..", Fn: setMapData},
		f{Name: "字典.获取", L: "1", Fn: getMapData},

		f{Name: "重启", L: "0", Fn: restart},

		f{Name: "读文件.随机一行", L: "1|2", Fn: readStringFileRandomLine},
		f{Name: "读文件行", L: "1|2|3|4", Fn: readStringFileLines},
		f{Name: "读文件.行数", L: "1|2", Fn: readStringFileLinesCount},

		f{Name: "替换", L: "2|3|4", Fn: replaced},

		f{Name: "AES加密", L: "3", Fn: aesEncrypt},
		f{Name: "AES解密", L: "3", Fn: aesDecrypt},

		f{Name: "分割", L: "2|3", Fn: split},
		f{Name: "字符切片", L: "1", Fn: stringSlice},

		f{Name: "sha256", L: "1", Fn: sha256Encrypt},
		f{Name: "Byte生成", L: "1", Fn: newByte},

		f{Name: "访问.新建", L: "1", Fn: newRequest},
		f{Name: "访问.切换GET", L: "1", Fn: changeRequestGet},
		f{Name: "访问.切换POST", L: "1|2", Fn: changeRequestPost},
		f{Name: "访问.启用跳转", L: "1", Fn: requestEnableRedirects},
		f{Name: "访问.禁用跳转", L: "1", Fn: requestDisableRedirects},
		f{Name: "访问.设置头部", L: "2", Fn: requestSetHeader},
		f{Name: "访问.设置超时", L: "2", Fn: requestSetTimeout},
		f{Name: "访问.POST", L: "2", Fn: requestPost},
		f{Name: "访问.POST文件", L: "3|4", Fn: requestPostFile},
		f{Name: "访问.发送", L: "1", Fn: requestSend},
		f{Name: "访问.全部内容", L: "1", Fn: requestAllContent},
		f{Name: "访问.内容", L: "1", Fn: requestContent},
		f{Name: "访问", L: "1|2", Fn: accessGet},
		f{Name: "访问POST", L: "2|3", Fn: accessPost},

		f{Name: "分割匹配", L: "3", Fn: splitMatch},
		f{Name: "正则替换", L: "2|3|4", Fn: regexReplace},
		f{Name: "正则匹配", L: "2", Fn: regexpMatche},
		f{Name: "正则", L: "2", Fn: regexpFind},

		f{Name: "随机文本", L: "1|2", Fn: randString},

		f{Name: "判断值", L: "1", Fn: ifNONull},
		f{Name: "判断空值", L: "1", Fn: ifNull},

		f{Name: "MD5编码", L: "1", Fn: enMd5},
		f{Name: "B64编码", L: "1", Fn: base64En},
		f{Name: "B64解码", L: "1", Fn: base64De},

		f{Name: "URL编码", L: "1", Fn: urlEn},
		f{Name: "URL解码", L: "1", Fn: urlDe},
		f{Name: "URL链接编码", L: "1", Fn: urlPathEn},
		f{Name: "URL链接解码", L: "1", Fn: urlPathDe},

		f{Name: "JSON存", L: "2..", Fn: jsonSet},
		f{Name: "JSON存字", L: "2..", Fn: jsonSetString},

		f{Name: "MIME类型", L: "1", Fn: getMime},
	); err != nil {
		fmt.Println(err)
	}
}

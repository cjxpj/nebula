package funcs

import (
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
)

type f = dto.RegisterDicFunc

func Setup() {
	if err := Registers(
		// ========== 字符串 ==========
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
		f{Name: "替换", L: "2|3|4", Fn: replaced},
		f{Name: "分割", L: "2|3", Fn: split},
		f{Name: "字符切片", L: "1", Fn: stringSlice},
		f{Name: "大写字母", L: "1", Fn: toUpper},
		f{Name: "小写字母", L: "1", Fn: toLower},
		f{Name: "中文转拼音", L: "1", Fn: pinYin},
		f{Name: "炫酷文字", L: "1|2", Fn: coolText},

		// ========== 数字 ==========
		f{Name: "数字格式化", L: "2|3", Fn: numberFormatting},
		f{Name: "数字转中文", L: "1", Fn: numToString},
		f{Name: "四舍五入", L: "1|2", Fn: round},
		f{Name: "计算", L: "2..", Fn: doCount},

		// ========== 随机 ==========
		f{Name: "随机文本", L: "1|2", Fn: randString},
		f{Name: "随机数", L: "2", Fn: doRandNum},
		f{Name: "随机大小字母", L: "1", Fn: randLetterUpperLower},
		f{Name: "随机大写字母", L: "1", Fn: randLetterUpper},
		f{Name: "随机小写字母", L: "1", Fn: randLetterLower},
		f{Name: "随机大小字母数字", L: "1", Fn: randLetterUpperLowerNum},
		f{Name: "随机小写字母数字", L: "1", Fn: randLetterLowerNum},
		f{Name: "随机大写字母数字", L: "1", Fn: randLetterUpperNum},
		f{Name: "随机数字", L: "1", Fn: randNumber},

		// ========== 变量 ==========
		f{Name: "线程变量", L: "1|2", Fn: threadVar},
		f{Name: "临时写", L: "2|3", Fn: tempWrite},
		f{Name: "临时读", L: "1|2", Fn: tempRead},
		f{Name: "变量", L: "1|2", Fn: localVar},
		f{Name: "存在变量", L: "1", Fn: localVarExist},
		f{Name: "全局变量", L: "1|2", Fn: globalVar},
		f{Name: "锁变量", L: "1", Fn: localVarLock},
		f{Name: "变量文本", L: "1", Fn: localVarText},
		f{Name: "字典", List: []f{
			{Name: "创建", L: "0|1", Fn: newMapData},
			{Name: "设置", L: "3..", Fn: setMapData},
			{Name: "获取", L: "1", Fn: getMapData},
		}},

		// ========== 流程控制 ==========
		f{Name: "判断值", L: "1", Fn: ifNONull},
		f{Name: "判断空值", L: "1", Fn: ifNull},
		f{Name: "延迟", L: "1", Fn: appSleep},
		f{Name: "捕获输出", L: "0", Fn: captureOutput},
		f{Name: "拦截输出", L: "0", Fn: interceptOutput},
		f{Name: "STOP", L: "0", Fn: stopProgram},
		f{Name: "重启", L: "0", Fn: restart},
		f{Name: "GC回收", L: "0", Fn: gcCollect},

		// ========== 文件操作 ==========
		f{Name: "读", L: "1|2|3", Fn: readKeyStringFile},
		f{Name: "写", L: "2|3", Fn: writeKeyStringFile},
		f{Name: "写文件", L: "1|2", Fn: writeStringFile},
		f{Name: "读文件", L: "1|2", Fn: readStringFile},
		f{Name: "读文件", List: []f{
			{Name: "随机一行", L: "1|2", Fn: readStringFileRandomLine},
			{Name: "行数", L: "1|2", Fn: readStringFileLinesCount},
		}},
		f{Name: "读文件行", L: "1|2|3|4", Fn: readStringFileLines},
		f{Name: "文件后缀", L: "1", Fn: fileSuffix},
		f{Name: "存在文件", L: "1", Fn: fileExist},
		f{Name: "存在文件夹", L: "1", Fn: dirExist},
		f{Name: "存在文件或文件夹", L: "1", Fn: fileOrDirExist},
		f{Name: "删除文件", L: "1", Fn: deleteFile},
		f{Name: "删除文件夹", L: "1", Fn: deleteDir},
		f{Name: "文件夹列表", L: "0|1", Fn: dirList},
		f{Name: "文件列表", L: "0|1", Fn: fileList},
		f{Name: "随机文件名", L: "0|1", Fn: randomFileName},
		f{Name: "随机文件夹名", L: "0|1", Fn: randomDirName},
		f{Name: "文件夹大小", L: "1", Fn: dirSize},
		f{Name: "文件大小", L: "1", Fn: fileSize},
		f{Name: "重命名", L: "2", Fn: fileRename},
		f{Name: "复制粘贴", L: "2", Fn: fileCopy},
		f{Name: "下载文件", L: "2|3|4", Fn: downloadFile},

		// ========== 日志 ==========
		f{Name: "日志", L: "1|2", Fn: logfile},
		f{Name: "打印", L: "1..", Fn: print},

		// ========== 编码/解码 ==========
		f{Name: "编码", L: "1|2", Fn: enUtf8},
		f{Name: "解码", L: "1|2", Fn: deUtf8},
		f{Name: "MD5编码", L: "1", Fn: enMd5},
		f{Name: "B64编码", L: "1", Fn: base64En},
		f{Name: "B64解码", L: "1", Fn: base64De},
		f{Name: "URL编码", L: "1", Fn: urlEn},
		f{Name: "URL解码", L: "1", Fn: urlDe},
		f{Name: "URL链接编码", L: "1", Fn: urlPathEn},
		f{Name: "URL链接解码", L: "1", Fn: urlPathDe},
		f{Name: "sha256", L: "1", Fn: sha256Encrypt},
		f{Name: "Byte生成", L: "1", Fn: newByte},
		f{Name: "Byte转String", L: "1", Fn: byteToString},
		f{Name: "MD转义", L: "1", Fn: mdEscape},
		f{Name: "MIME类型", L: "1", Fn: getMime},
		f{Name: "加密词库", L: "1", Fn: encodeDic},

		// ========== 正则 ==========
		f{Name: "分割匹配", L: "3", Fn: splitMatch},
		f{Name: "正则替换", L: "2|3|4", Fn: regexReplace},
		f{Name: "正则匹配", L: "2", Fn: regexpMatche},
		f{Name: "正则", L: "2", Fn: regexpFind},

		// ========== 加密/解密 ==========
		f{Name: "哈基米加密", L: "1|2", Fn: hajimimanboEncrypt},
		f{Name: "哈基米解密", L: "1|2", Fn: hajimimanboDecrypt},
		f{Name: "AES", List: []f{
			{Name: "CBC加密", L: "3", Fn: aesCBCEncrypt},
			{Name: "CBC解密", L: "3", Fn: aesCBCDecrypt},
			{Name: "CFB加密", L: "3", Fn: aesCFBEncrypt},
			{Name: "CFB解密", L: "3", Fn: aesCFBDecrypt},
			{Name: "GCM加密", L: "2", Fn: aesGCMEncrypt},
			{Name: "GCM解密", L: "2", Fn: aesGCMDecrypt},
			{Name: "CTR加密", L: "3", Fn: aesCTREncrypt},
			{Name: "CTR解密", L: "3", Fn: aesCTRDecrypt},
		}},

		// ========== Ed25519 ==========
		f{Name: "Ed25519种子大小", L: "0", Fn: ed25519_SeedSize},
		f{Name: "Ed25519生成密钥", L: "0", Fn: ed25519_GenerateKey},
		f{Name: "Ed25519从种子生成密钥", L: "1", Fn: ed25519NewKeyFromSeed},
		f{Name: "Ed25519签名", L: "2", Fn: ed25519Sign},
		f{Name: "Ed25519验证签名", L: "3", Fn: ed25519Verify},
		f{Name: "Ed25519公钥转换为Curve25519", L: "1", Fn: ed25519PublicKeyToCurve25519},
		f{Name: "Ed25519私钥转换为Curve25519", L: "1", Fn: ed25519PrivateKeyToCurve25519},
		f{Name: "Ed25519从Curve25519生成密钥", L: "1", Fn: ed25519NewKeyFromCurve25519},

		// ========== 网络访问 ==========
		f{Name: "访问", List: []f{
			{Name: "新建", L: "1", Fn: newRequest},
			{Name: "切换GET", L: "1", Fn: changeRequestGet},
			{Name: "切换POST", L: "1|2", Fn: changeRequestPost},
			{Name: "启用跳转", L: "1", Fn: requestEnableRedirects},
			{Name: "禁用跳转", L: "1", Fn: requestDisableRedirects},
			{Name: "设置头部", L: "2", Fn: requestSetHeader},
			{Name: "设置超时", L: "2", Fn: requestSetTimeout},
			{Name: "POST", L: "2", Fn: requestPost},
			{Name: "POST文件", L: "3|4", Fn: requestPostFile},
			{Name: "发送", L: "1", Fn: requestSend},
			{Name: "全部内容", L: "1", Fn: requestAllContent},
			{Name: "内容", L: "1", Fn: requestContent},
		}},
		f{Name: "访问", L: "1|2", Fn: accessGet},
		f{Name: "访问POST", L: "2|3", Fn: accessPost},
		f{Name: "访问转发", L: "1", Fn: requestForward},

		// ========== 终端 ==========
		f{Name: "终端", List: []f{
			{Name: "创建", L: "1..", Fn: runCommandNew},
			{Name: "Shell创建", L: "1..", Fn: runCommandShellNew},
			{Name: "异步执行", L: "1", Fn: runCommandAsync},
			{Name: "执行目录", L: "2", Fn: runCommandDir},
			{Name: "执行", L: "1", Fn: runCommand},
			{Name: "等待输入", L: "0", Fn: runCommandInput},
			{Name: "解码器", L: "2", Fn: runCommandDecoder},
			{Name: "变量", L: "2", Fn: runCommandVar},
			{Name: "断开", L: "1", Fn: runCommandClose},
			{Name: "输入", L: "2", Fn: runCommandInputText},
		}},

		// ========== 数据库 ==========
		f{Name: "mysql", List: []f{
			{Name: "新建", L: "3", Fn: mysqlNew},
			{Name: "PING", L: "1", Fn: mysqlPing},
			{Name: "执行", L: "2..", Fn: mysqlExec},
			{Name: "切换数据库", L: "2", Fn: mysqlSwitchDB},
			{Name: "写", L: "3|4", Fn: mysqlWrite},
			{Name: "读", L: "1|2|3|4", Fn: mysqlRead},
			{Name: "删除文件", L: "2", Fn: mysqlDeleteFile},
			{Name: "删除文件夹", L: "2", Fn: mysqlDeleteDir},
			{Name: "关闭", L: "1", Fn: mysqlClose},
		}},
		f{Name: "sqlite", List: []f{
			{Name: "打开", L: "1|2", Fn: sqliteOpen},
			{Name: "写", L: "3|4", Fn: sqliteWrite},
			{Name: "读", L: "1|2|3|4", Fn: sqliteRead},
			{Name: "执行", L: "2..", Fn: sqliteExec},
			{Name: "删除文件", L: "2", Fn: sqliteDeleteFile},
			{Name: "删除文件夹", L: "2", Fn: sqliteDeleteDir},
		}},
		f{Name: "读", List: []f{
			{Name: "sqlite", L: "1|2|3", Fn: readSqlite},
		}},
		f{Name: "写", List: []f{
			{Name: "sqlite", L: "2|3", Fn: writeSqlite},
		}},
		f{Name: "关闭数据库", L: "1", Fn: dbClose},
		f{Name: "db", List: []f{
			{Name: "写", L: "1|2", Fn: dbWrite},
			{Name: "读", L: "0|1|2", Fn: dbRead},
			{Name: "删除", L: "2", Fn: dbDelete},
			{Name: "删除文件", L: "1", Fn: dbDeleteFile},
			{Name: "删除文件夹", L: "1", Fn: dbDeleteDir},
		}},

		// ========== JSON ==========
		f{Name: "JSON解析", L: "1|2", Fn: queryJson},
		f{Name: "json解析", L: "1|2", Fn: queryJson},
		f{Name: "JSON判断", L: "1", Fn: isJson},
		f{Name: "JSON存", L: "2..", Fn: jsonSet},
		f{Name: "JSON存字", L: "2..", Fn: jsonSetString},
		f{Name: "JSON追加", L: "2|3", Fn: jsonAdd},
		f{Name: "JSON追加字", L: "2|3", Fn: jsonAddString},
		f{Name: "JSON删", L: "2", Fn: jsonDelete},
		f{Name: "JSON存在", L: "2", Fn: jsonIsKey},
		f{Name: "JSON长度", L: "1", Fn: jsonLen},
		f{Name: "JSON美化", L: "1|2", Fn: jsonPrettyPrint},
		f{Name: "JSON重名解析", L: "2", Fn: jsonQueryByName},
		f{Name: "json", List: []f{
			{Name: "查找文本", L: "2", Fn: jsonFindText},
			{Name: "模糊查找文本", L: "2", Fn: jsonFindTextFuzzy},
			{Name: "正则查找文本", L: "2", Fn: jsonFindTextRegex},
		}},

		// ========== HTML / Markdown ==========
		f{Name: "HTML解析", L: "1", Fn: htmlParse},
		f{Name: "HTML编码", L: "1", Fn: htmlEncode},
		f{Name: "HTML解码", L: "1", Fn: htmlDecode},
		f{Name: "MD转HTML", L: "1", Fn: markdownToHtml},

		// ========== 画布绘图 ==========
		f{Name: "绘图", L: "1", Fn: drawImg},
		f{Name: "画布", List: []f{
			{Name: "创建", L: "2|3", Fn: drawImgNew},
			{Name: "获取", L: "1|2", Fn: drawImgGet},
			{Name: "旋转", L: "2|3", Fn: drawImgRotate},
			{Name: "圆形", L: "1|2|3", Fn: drawImgRoundCorners},
			{Name: "灰度", L: "1", Fn: drawImgGrayscale},
			{Name: "马赛克", L: "1|2", Fn: drawImgAllMosaic},
		}},
		f{Name: "画笔", List: []f{
			{Name: "字体", L: "2", Fn: drawImgLoadFont},
			{Name: "大小", L: "2", Fn: drawImgSetSize},
			{Name: "获取颜色", L: "1|2|3|4", Fn: drawImgGetColor},
			{Name: "设置颜色", L: "1|2|3|4|5", Fn: drawImgSetColor},
		}},
		f{Name: "绘制", List: []f{
			{Name: "文本", L: "4|5|6|7|8", Fn: drawImgText},
			{Name: "点", L: "3|4", Fn: drawImgPoint},
			{Name: "线", L: "5|6", Fn: drawImgLine},
			{Name: "喷漆", L: "5|6|7|8|9", Fn: drawImgBrushLine},
			{Name: "波浪", L: "5|6|7|8", Fn: drawImgWaveLine},
			{Name: "油漆桶", L: "3|4", Fn: drawImgFloodFill},
			{Name: "方形", L: "5|6|7", Fn: drawImgRectangleFill},
			{Name: "方形描边", L: "5|6|7", Fn: drawImgRectangleStroke},
			{Name: "椭圆", L: "5|6", Fn: drawImgEllipseFill},
			{Name: "椭圆描边", L: "5|6", Fn: drawImgEllipse},
			{Name: "圆形", L: "3|4|5|6|7", Fn: drawImgPieFill},
			{Name: "圆形描边", L: "3|4|6|7", Fn: drawImgPie},
			{Name: "多边形", L: "4..", Fn: drawImgPolygon},
			{Name: "多边形描边", L: "3..", Fn: drawImgPolygons},
			{Name: "图片", L: "2|3|4|5|6|7|8|9", Fn: drawImgPaste},
			{Name: "圆弧", L: "6|7", Fn: drawImgArc},
			{Name: "随机点", L: "1|2", Fn: drawImgRandomDots},
			{Name: "随机线条", L: "1|2", Fn: drawImgRandomLines},
			{Name: "高斯模糊", L: "5", Fn: drawImgGaussianBlur},
			{Name: "马赛克", L: "5", Fn: drawImgMosaic},
		}},
		f{Name: "写图片", L: "2", Fn: writeImage},
		f{Name: "读图片", L: "1|2", Fn: readImage},

		// ========== 其他 ==========
		f{Name: "读配置", L: "2|3", Fn: readConfig},
		f{Name: "写配置", L: "2|3", Fn: writeConfig},
		f{Name: "GIF拆帧", L: "1", Fn: getGif},
		f{Name: "图片相似度", L: "2", Fn: imageSimilarity},
		f{Name: "排序", L: "2|3", Fn: doSort},
		f{Name: "范围", L: "2", Fn: doRange},
		f{Name: "ZIP压缩", L: "2", Fn: zipCompress},
		f{Name: "ZIP解压", L: "2", Fn: zipDecompress},
		f{Name: "邮件", List: []f{
			{Name: "创建", L: "4", Fn: emailCreate},
			{Name: "发送", L: "4", Fn: emailSend},
			{Name: "发送HTML", L: "4", Fn: emailSendHTML},
		}},
		f{Name: "主机", L: "1", Fn: host_information},
		f{Name: "时间戳格式化时间", L: "2|3", Fn: timestampFormattingTime},
		f{Name: "时间间隔", L: "1", Fn: timeSince},
		f{Name: "腾讯", List: []f{
			{Name: "接口", L: "6|7", Fn: tencentGetApi},
			{Name: "调用", L: "2", Fn: tencentGetApiCall},
		}},
		f{Name: "取前字符", L: "2", Fn: subStrHead},
		f{Name: "取后字符", L: "2", Fn: subStrTail},
	); err != nil {
		debugLog.Infof("注册函数失败：%v", err)
	}
}

package funcs

import (
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
)

type f = dto.RegisterDicFunc

// jumpAbsNoop $跳行 的注册占位实现：绝对跳行在字节码编译期（parseJumpAbs）整行拦截为 OpJumpAbs，
// 不会走到函数调用；这里仅用于让「跳行」出现在 ListFuncs 补全列表，并使行内误用静默返回空串。
func jumpAbsNoop(d *dto.DicInputs) (any, error) {
	return "", nil
}

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

		// ========== 科学计算 ==========
		f{Name: "正弦", L: "1", Fn: mathSin},
		f{Name: "sin", L: "1", Fn: mathSin},
		f{Name: "余弦", L: "1", Fn: mathCos},
		f{Name: "cos", L: "1", Fn: mathCos},
		f{Name: "正切", L: "1", Fn: mathTan},
		f{Name: "tan", L: "1", Fn: mathTan},
		f{Name: "反正弦", L: "1", Fn: mathAsin},
		f{Name: "asin", L: "1", Fn: mathAsin},
		f{Name: "反余弦", L: "1", Fn: mathAcos},
		f{Name: "acos", L: "1", Fn: mathAcos},
		f{Name: "反正切", L: "1", Fn: mathAtan},
		f{Name: "atan", L: "1", Fn: mathAtan},
		f{Name: "反正切2", L: "2", Fn: mathAtan2},
		f{Name: "atan2", L: "2", Fn: mathAtan2},
		f{Name: "幂运算", L: "2", Fn: mathPow},
		f{Name: "pow", L: "2", Fn: mathPow},
		f{Name: "指数", L: "1", Fn: mathExp},
		f{Name: "exp", L: "1", Fn: mathExp},
		f{Name: "自然对数", L: "1", Fn: mathLog},
		f{Name: "log", L: "1", Fn: mathLog},
		f{Name: "ln", L: "1", Fn: mathLog},
		f{Name: "常用对数", L: "1", Fn: mathLog10},
		f{Name: "log10", L: "1", Fn: mathLog10},
		f{Name: "二进制对数", L: "1", Fn: mathLog2},
		f{Name: "log2", L: "1", Fn: mathLog2},
		f{Name: "平方根", L: "1", Fn: mathSqrt},
		f{Name: "sqrt", L: "1", Fn: mathSqrt},
		f{Name: "立方根", L: "1", Fn: mathCbrt},
		f{Name: "cbrt", L: "1", Fn: mathCbrt},
		f{Name: "绝对值", L: "1", Fn: mathAbs},
		f{Name: "abs", L: "1", Fn: mathAbs},
		f{Name: "向上取整", L: "1", Fn: mathCeil},
		f{Name: "ceil", L: "1", Fn: mathCeil},
		f{Name: "向下取整", L: "1", Fn: mathFloor},
		f{Name: "floor", L: "1", Fn: mathFloor},

		// ========== 角度单位转换 ==========
		f{Name: "角度转弧度", L: "1", Fn: angDegToRad},
		f{Name: "deg2rad", L: "1", Fn: angDegToRad},
		f{Name: "弧度转角度", L: "1", Fn: angRadToDeg},
		f{Name: "rad2deg", L: "1", Fn: angRadToDeg},
		f{Name: "百分度转弧度", L: "1", Fn: angGonToRad},
		f{Name: "gon2rad", L: "1", Fn: angGonToRad},
		f{Name: "弧度转百分度", L: "1", Fn: angRadToGon},
		f{Name: "rad2gon", L: "1", Fn: angRadToGon},
		f{Name: "密位转弧度", L: "1", Fn: angMilToRad},
		f{Name: "mil2rad", L: "1", Fn: angMilToRad},
		f{Name: "弧度转密位", L: "1", Fn: angRadToMil},
		f{Name: "rad2mil", L: "1", Fn: angRadToMil},

		// ========== 角度制三角函数 ==========
		f{Name: "正弦度", L: "1", Fn: sinDeg},
		f{Name: "SinDeg", L: "1", Fn: sinDeg},
		f{Name: "余弦度", L: "1", Fn: cosDeg},
		f{Name: "CosDeg", L: "1", Fn: cosDeg},
		f{Name: "正切度", L: "1", Fn: tanDeg},
		f{Name: "TanDeg", L: "1", Fn: tanDeg},
		f{Name: "正弦百分度", L: "1", Fn: sinGon},
		f{Name: "SinGon", L: "1", Fn: sinGon},
		f{Name: "余弦百分度", L: "1", Fn: cosGon},
		f{Name: "CosGon", L: "1", Fn: cosGon},
		f{Name: "正切百分度", L: "1", Fn: tanGon},
		f{Name: "TanGon", L: "1", Fn: tanGon},
		f{Name: "正弦密位", L: "1", Fn: sinMil},
		f{Name: "SinMil", L: "1", Fn: sinMil},
		f{Name: "余弦密位", L: "1", Fn: cosMil},
		f{Name: "CosMil", L: "1", Fn: cosMil},
		f{Name: "正切密位", L: "1", Fn: tanMil},
		f{Name: "TanMil", L: "1", Fn: tanMil},

		// ========== 统计 ==========
		f{Name: "求和", L: "1..", Fn: statSum},
		f{Name: "sum", L: "1..", Fn: statSum},
		f{Name: "计数", L: "1..", Fn: statCount},
		f{Name: "count", L: "1..", Fn: statCount},
		f{Name: "平均值", L: "1..", Fn: statMean},
		f{Name: "mean", L: "1..", Fn: statMean},
		f{Name: "中位数", L: "1..", Fn: statMedian},
		f{Name: "median", L: "1..", Fn: statMedian},
		f{Name: "方差", L: "1..", Fn: statVariance},
		f{Name: "variance", L: "1..", Fn: statVariance},
		f{Name: "标准差", L: "1..", Fn: statStddev},
		f{Name: "stddev", L: "1..", Fn: statStddev},

		// ========== 金融 ==========
		f{Name: "净现值", L: "2..", Fn: finNPV},
		f{Name: "NPV", L: "2..", Fn: finNPV},
		f{Name: "内部收益率", L: "1..", Fn: finIRR},
		f{Name: "IRR", L: "1..", Fn: finIRR},

		// ========== 复数 ==========
		f{Name: "复数加", L: "4", Fn: complexAdd},
		f{Name: "复数减", L: "4", Fn: complexSub},
		f{Name: "复数乘", L: "4", Fn: complexMul},
		f{Name: "复数除", L: "4", Fn: complexDiv},
		f{Name: "复数模", L: "2", Fn: complexAbs},
		f{Name: "复数共轭", L: "2", Fn: complexConj},

		// ========== 矩阵 ==========
		f{Name: "矩阵加", L: "2", Fn: matrixAdd},
		f{Name: "矩阵减", L: "2", Fn: matrixSub},
		f{Name: "矩阵乘", L: "2", Fn: matrixMul},
		f{Name: "矩阵转置", L: "1", Fn: matrixTranspose},
		f{Name: "矩阵行列式", L: "1", Fn: matrixDet},
		f{Name: "矩阵逆", L: "1", Fn: matrixInverse},

		// ========== 随机 ==========
		f{Name: "设置随机种子", L: "1", Fn: randSeed},
		f{Name: "随机种子", L: "1", Fn: randSeed},
		f{Name: "seed", L: "1", Fn: randSeed},
		f{Name: "随机小数", L: "0|2", Fn: randFloat},
		f{Name: "randfloat", L: "0|2", Fn: randFloat},
		f{Name: "正态分布", L: "0|2", Fn: randNormal},
		f{Name: "normal", L: "0|2", Fn: randNormal},
		f{Name: "指数分布", L: "0|1", Fn: randExpDist},
		f{Name: "expdist", L: "0|1", Fn: randExpDist},
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
		f{Name: "创建字典", L: "0|1", Fn: newMapData},

		// ========== 流程控制 ==========
		f{Name: "判断", L: "1..", Fn: ifCondition},
		f{Name: "判断值", L: "1", Fn: ifNONull},
		f{Name: "判断空值", L: "1", Fn: ifNull},
		f{Name: "延迟", L: "1", Fn: appSleep},
		f{Name: "捕获输出", L: "0", Fn: captureOutput},
		f{Name: "拦截输出", L: "0", Fn: interceptOutput},
		f{Name: "STOP", L: "0", Fn: stopProgram},
		f{Name: "重启", L: "0", Fn: restart},
		f{Name: "GC回收", L: "0", Fn: gcCollect},
		f{Name: "跳行", L: "1", Fn: jumpAbsNoop},

		// ========== 定时任务 ==========
		f{Name: "添加定时任务", L: "1|2|3|4|5", Fn: addScheduledTaskFunc},
		f{Name: "删除定时任务", L: "1", Fn: delScheduledTaskFunc},
		f{Name: "定时任务列表", L: "0", Fn: listScheduledTaskFunc},

		// ========== 文件操作 ==========
		f{Name: "读", L: "1|2|3", Fn: readKeyStringFile},
		f{Name: "写", L: "2|3", Fn: writeKeyStringFile},
		f{Name: "写文件", L: "1|2", Fn: writeStringFile},
		f{Name: "读文件", L: "1|2", Fn: readStringFile},
		f{Name: "读文件_随机一行", L: "1|2", Fn: readStringFileRandomLine},
		f{Name: "读文件_行数", L: "1|2", Fn: readStringFileLinesCount},
		f{Name: "读文件行", L: "1|2|3|4", Fn: readStringFileLines},
		f{Name: "读文件MD5", L: "1", Fn: readFileMd5},
		f{Name: "文件后缀", L: "1", Fn: fileSuffix},
		f{Name: "存在文件", L: "1", Fn: fileExist},
		f{Name: "存在文件夹", L: "1", Fn: dirExist},
		f{Name: "存在文件或文件夹", L: "1", Fn: fileOrDirExist},
		f{Name: "设置工作目录", L: "1", Fn: setWorkDir},
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
		f{Name: "文件属性", L: "1", Fn: fileAttributeGet},
		f{Name: "设置文件属性", L: "2", Fn: fileAttributeSet},

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
		f{Name: "AES_CBC加密", L: "3", Fn: aesCBCEncrypt},
		f{Name: "AES_CBC解密", L: "3", Fn: aesCBCDecrypt},
		f{Name: "AES_CFB加密", L: "3", Fn: aesCFBEncrypt},
		f{Name: "AES_CFB解密", L: "3", Fn: aesCFBDecrypt},
		f{Name: "AES_GCM加密", L: "2", Fn: aesGCMEncrypt},
		f{Name: "AES_GCM解密", L: "2", Fn: aesGCMDecrypt},
		f{Name: "AES_CTR加密", L: "3", Fn: aesCTREncrypt},
		f{Name: "AES_CTR解密", L: "3", Fn: aesCTRDecrypt},

		// ========== RSA ==========
		f{Name: "RSA生成密钥", L: "0|1", Fn: rsaGenerateKey},
		f{Name: "RSA加密", L: "2", Fn: rsaEncrypt},
		f{Name: "RSA解密", L: "2", Fn: rsaDecrypt},

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
		f{Name: "新建访问", L: "1", Fn: newRequest},
		f{Name: "访问", L: "1|2", Fn: accessGet},
		f{Name: "访问POST", L: "2|3", Fn: accessPost},
		f{Name: "访问转发", L: "1", Fn: requestForward},

		// ========== 终端 ==========
		f{Name: "创建终端", L: "1..", Fn: runCommandNew},
		f{Name: "创建Shell终端", L: "1..", Fn: runCommandShellNew},
		f{Name: "MC终端颜色", L: "1", Fn: mcTerminalColor},

		// ========== 数据库 ==========
		f{Name: "新建mysql", L: "3", Fn: mysqlNew},
		f{Name: "打开sqlite", L: "1|2", Fn: sqliteOpen},
		f{Name: "读sqlite", L: "1|2|3", Fn: readSqlite},
		f{Name: "写sqlite", L: "2|3", Fn: writeSqlite},
		f{Name: "关闭数据库", L: "1", Fn: dbClose},
		f{Name: "db_写", L: "1|2", Fn: dbWrite},
		f{Name: "db_读", L: "0|1|2", Fn: dbRead},
		f{Name: "db_删除", L: "2", Fn: dbDelete},
		f{Name: "db_删除文件", L: "1", Fn: dbDeleteFile},
		f{Name: "db_删除文件夹", L: "1", Fn: dbDeleteDir},
		f{Name: "db_添加", L: "3", Fn: dbMoneyAdd},
		f{Name: "db_减少", L: "3", Fn: dbMoneySub},
		f{Name: "db_设置", L: "3", Fn: dbMoneySet},
		f{Name: "db_查询", L: "1|2", Fn: dbMoneyQuery},
		f{Name: "db_查询排名", L: "1|2|3", Fn: dbMoneyRank},
		f{Name: "db_清空经济系统", L: "0|1", Fn: dbMoneyClear},

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
		f{Name: "JSON全部键", L: "1", Fn: jsonKeys},
		f{Name: "JSON美化", L: "1|2", Fn: jsonPrettyPrint},
		f{Name: "JSON重名解析", L: "2", Fn: jsonQueryByName},
		f{Name: "JSON查找文本", L: "2", Fn: jsonFindText},
		f{Name: "JSON模糊查找文本", L: "2", Fn: jsonFindTextFuzzy},
		f{Name: "JSON正则查找文本", L: "2", Fn: jsonFindTextRegex},
		f{Name: "JSON拆分", L: "2", Fn: jsonSplit},

		// ========== HTML / Markdown ==========
		f{Name: "HTML解析", L: "1..", Fn: htmlParse},
		f{Name: "HTML文本", L: "1..", Fn: htmlText},
		f{Name: "HTML编码", L: "1", Fn: htmlEncode},
		f{Name: "HTML解码", L: "1", Fn: htmlDecode},
		f{Name: "MD转HTML", L: "1", Fn: markdownToHtml},

		// ========== 画布绘图 ==========
		f{Name: "绘图", L: "1", Fn: drawImg},
		f{Name: "创建画布", L: "2|3", Fn: drawImgNew},
		f{Name: "获取画笔颜色", L: "1|2|3|4", Fn: drawImgGetColor},
		f{Name: "写图片", L: "2", Fn: writeImage},
		f{Name: "读图片", L: "1|2", Fn: readImage},

		// ========== 其他 ==========
		f{Name: "读配置", L: "2|3", Fn: readConfig},
		f{Name: "写配置", L: "2|3", Fn: writeConfig},
		f{Name: "云工具状态", L: "0", Fn: cloudToolStatus},
		f{Name: "GIF拆帧", L: "1", Fn: getGif},
		f{Name: "图片相似度", L: "2", Fn: imageSimilarity},
		f{Name: "图片最多颜色", L: "1", Fn: imageMostColor},
		f{Name: "图片平均颜色", L: "1", Fn: imageAvgColor},
		f{Name: "排序", L: "2|3", Fn: doSort},
		f{Name: "范围", L: "2", Fn: doRange},
		f{Name: "ZIP压缩", L: "2", Fn: zipCompress},
		f{Name: "ZIP解压", L: "2", Fn: zipDecompress},
		f{Name: "创建邮件", L: "4", Fn: emailCreate},
		f{Name: "主机", L: "1", Fn: host_information},
		f{Name: "时间戳转时间", L: "1|2", Fn: timestampToTime},
		f{Name: "时间转时间戳", L: "1|2", Fn: timeToTimestamp},
		f{Name: "时间间隔", L: "1", Fn: timeSince},
		f{Name: "腾讯接口", L: "6|7", Fn: tencentGetApi},
		f{Name: "取前字符", L: "2", Fn: subStrHead},
		f{Name: "取后字符", L: "2", Fn: subStrTail},
	); err != nil {
		debugLog.Infof("注册函数失败：%v", err)
	}
}

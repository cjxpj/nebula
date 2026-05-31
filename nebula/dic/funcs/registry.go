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
		f{Name: "终端.执行目录", L: "2", Fn: runCommandDir},
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

		f{Name: "AES.CBC加密", L: "3", Fn: aesCBCEncrypt},
		f{Name: "AES.CBC解密", L: "3", Fn: aesCBCDecrypt},
		f{Name: "AES.CFB加密", L: "3", Fn: aesCFBEncrypt},
		f{Name: "AES.CFB解密", L: "3", Fn: aesCFBDecrypt},
		f{Name: "AES.GCM加密", L: "2", Fn: aesGCMEncrypt},
		f{Name: "AES.GCM解密", L: "2", Fn: aesGCMDecrypt},
		f{Name: "AES.CTR加密", L: "3", Fn: aesCTREncrypt},
		f{Name: "AES.CTR解密", L: "3", Fn: aesCTRDecrypt},

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

		f{Name: "四舍五入", L: "1|2", Fn: round},

		f{Name: "炫酷文字", L: "1|2", Fn: coolText},

		f{Name: "sqlite.打开", L: "1|2", Fn: sqliteOpen},
		f{Name: "sqlite.写", L: "3|4", Fn: sqliteWrite},
		f{Name: "sqlite.读", L: "1|2|3|4", Fn: sqliteRead},
		f{Name: "sqlite.执行", L: "2..", Fn: sqliteExec},
		f{Name: "关闭数据库", L: "1", Fn: dbClose},

		f{Name: "读.sqlite", L: "1|2|3", Fn: readSqlite},
		f{Name: "写.sqlite", L: "2|3", Fn: writeSqlite},

		f{Name: "MD转义", L: "1", Fn: mdEscape},

		f{Name: "时间间隔", L: "1", Fn: timeSince},

		f{Name: "json.查找文本", L: "2", Fn: jsonFindText},
		f{Name: "json.模糊查找文本", L: "2", Fn: jsonFindTextFuzzy},
		f{Name: "json.正则查找文本", L: "2", Fn: jsonFindTextRegex},

		f{Name: "捕获输出", L: "0", Fn: captureOutput},
		f{Name: "拦截输出", L: "0", Fn: interceptOutput},
		f{Name: "STOP", L: "0", Fn: stopProgram},

		f{Name: "大写字母", L: "1", Fn: toUpper},
		f{Name: "小写字母", L: "1", Fn: toLower},

		f{Name: "加密词库", L: "1", Fn: encodeDic},

		f{Name: "ZIP压缩", L: "2", Fn: zipCompress},
		f{Name: "ZIP解压", L: "2", Fn: zipDecompress},

		f{Name: "文件夹大小", L: "1", Fn: dirSize},
		f{Name: "文件大小", L: "1", Fn: fileSize},
		f{Name: "重命名", L: "2", Fn: fileRename},
		f{Name: "复制粘贴", L: "2", Fn: fileCopy},

		f{Name: "计算", L: "2..", Fn: doCount},
		f{Name: "随机数", L: "2", Fn: doRandNum},
		f{Name: "随机大小字母", L: "1", Fn: randLetterUpperLower},
		f{Name: "随机大写字母", L: "1", Fn: randLetterUpper},
		f{Name: "随机小写字母", L: "1", Fn: randLetterLower},
		f{Name: "随机大小字母数字", L: "1", Fn: randLetterUpperLowerNum},
		f{Name: "随机小写字母数字", L: "1", Fn: randLetterLowerNum},
		f{Name: "随机大写字母数字", L: "1", Fn: randLetterUpperNum},
		f{Name: "随机数字", L: "1", Fn: randNumber},

		f{Name: "时间戳格式化时间", L: "2|3", Fn: timestampFormattingTime},

		f{Name: "JSON解析", L: "1|2", Fn: queryJson},
		f{Name: "json解析", L: "1|2", Fn: queryJson},
		f{Name: "JSON判断", L: "1", Fn: isJson},
		f{Name: "JSON追加", L: "2", Fn: jsonAdd},
		f{Name: "JSON追加字", L: "2", Fn: jsonAddString},
		f{Name: "JSON删", L: "2", Fn: jsonDelete},
		f{Name: "JSON存在", L: "2", Fn: jsonIsKey},
		f{Name: "JSON长度", L: "1", Fn: jsonLen},
		f{Name: "JSON美化", L: "1|2", Fn: jsonPrettyPrint},

		f{Name: "HTML解析", L: "1", Fn: htmlParse},
		f{Name: "HTML编码", L: "1", Fn: htmlEncode},
		f{Name: "HTML解码", L: "1", Fn: htmlDecode},

		f{Name: "编码", L: "1|2", Fn: enUtf8},
		f{Name: "解码", L: "1|2", Fn: deUtf8},

		f{Name: "GIF拆帧", L: "1", Fn: getGif},

		f{Name: "绘图", L: "1", Fn: drawImg},

		f{Name: "排序", L: "2|3", Fn: doSort},
		f{Name: "范围", L: "2", Fn: doRange},

		f{Name: "Ed25519从种子生成密钥", L: "1", Fn: ed25519NewKeyFromSeed},
		f{Name: "Ed25519签名", L: "2", Fn: ed25519Sign},
		f{Name: "Ed25519验证签名", L: "3", Fn: ed25519Verify},
		f{Name: "Ed25519公钥转换为Curve25519", L: "1", Fn: ed25519PublicKeyToCurve25519},
		f{Name: "Ed25519私钥转换为Curve25519", L: "1", Fn: ed25519PrivateKeyToCurve25519},
		f{Name: "Ed25519从Curve25519生成密钥", L: "1", Fn: ed25519NewKeyFromCurve25519},

		f{Name: "画笔.字体", L: "1", Fn: drawImgLoadFont},
		f{Name: "画笔.大小", L: "2", Fn: drawImgSetSize},
		f{Name: "绘制.文本", L: "6|7", Fn: drawImgText},
		f{Name: "绘制.点", L: "3", Fn: drawImgPoint},
		f{Name: "绘制.线", L: "6", Fn: drawImgLine},
		f{Name: "绘制.喷漆", L: "6", Fn: drawImgBrushLine},
		f{Name: "绘制.波浪", L: "7", Fn: drawImgWaveLine},
		f{Name: "绘制.油漆桶", L: "6", Fn: drawImgFloodFill},
		f{Name: "绘制.方形", L: "5|6", Fn: drawImgRectangleFill},
		f{Name: "绘制.方形描边", L: "5|6", Fn: drawImgRectangleStroke},
		f{Name: "绘制.椭圆", L: "5|6", Fn: drawImgEllipseFill},
		f{Name: "绘制.椭圆描边", L: "4|5", Fn: drawImgEllipse},
		f{Name: "绘制.圆形", L: "5|6", Fn: drawImgPieFill},
		f{Name: "绘制.圆形描边", L: "4|5", Fn: drawImgPie},
		f{Name: "绘制.多边形", L: "2..", Fn: drawImgPolygon},
		f{Name: "绘制.多边形描边", L: "2..", Fn: drawImgPolygons},
		f{Name: "绘制.图片", L: "3|4|5|6|7", Fn: drawImgPaste},
		f{Name: "画布.旋转", L: "2", Fn: drawImgRotate},
		f{Name: "画布.圆形", L: "2", Fn: drawImgRoundCorners},
		f{Name: "绘制.随机点", L: "2", Fn: drawImgRandomDots},
		f{Name: "绘制.随机线条", L: "2", Fn: drawImgRandomLines},
		f{Name: "画布.灰度", L: "1", Fn: drawImgGrayscale},
		f{Name: "绘制.高斯模糊", L: "2", Fn: drawImgGaussianBlur},
		f{Name: "绘制.马赛克", L: "4|5", Fn: drawImgMosaic},
		f{Name: "画布.马赛克", L: "1", Fn: drawImgAllMosaic},
		f{Name: "绘制.圆弧", L: "6", Fn: drawImgArc},

		f{Name: "图片相似度", L: "2", Fn: imageSimilarity},

		f{Name: "GC回收", L: "0", Fn: gcCollect},

		f{Name: "腾讯.接口", L: "6|7", Fn: tencentGetApi},
		f{Name: "腾讯.调用", L: "2", Fn: tencentGetApiCall},

		f{Name: "终端.解码器", L: "2", Fn: runCommandDecoder},
		f{Name: "终端.变量", L: "2", Fn: runCommandVar},
		f{Name: "终端.断开", L: "1", Fn: runCommandClose},
		f{Name: "终端.输入", L: "2", Fn: runCommandInputText},
	); err != nil {
		fmt.Println(err)
	}
}

package juice

import (
	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/dic"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

func Run() {

	// NewFileQueue("log.txt").DeleteFile()
	// Log("v"+s.Version, "system")

	// // 指定无法使用的日期
	// disabledDateString := "2024/5/5"

	// // 调用函数检查是否可以使用
	// if isAvailable(disabledDateString) {
	// 	LogStop("无法使用，测试版时间已到。")
	// }

	// 创建 private 文件夹
	// CreateFolderIfNotExists("private")
	// CreateFolderIfNotExists("private/system")
	// CreateFolderIfNotExists("private/utils")
	// CreateFolderIfNotExists("private/ttf")

	// 创建 database 文件夹
	// CreateFolderIfNotExists("database")

	// 创建 public 文件夹
	// mdfile := NewFileQueue("public/README.md")
	// if !mdfile.FileExists() {
	// 	Log("正在下载网络文档")
	// 	if ok := mdfile.Download("https://cjxpj.com/doc/juice.md"); ok {
	// 		Log("下载网络文档成功")
	// 	} else {
	// 		Error("尝试下载网络文档失败")
	// 	}
	// }

	file := utils.NewFile()
	file.SetPath("README.md").WriteFileByte(appfiles.DicMD)

	file.SetPath("system/ttf/font.ttf")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.TTF)
	}

	file.SetPath("system/start.n")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.Dicstart)
	}

	FileData, err := file.ReadFromFile()
	if err != nil {
		utils.ErrorStop("启动脚本不存在", "system")
	}

	GV := dto.NewVal()
	GV.Set("版本", appfiles.Version)
	dic := dic.NewDic(FileData, "Main", "system")
	dic.SetGlobal_v(GV)
	RunData := dic.Run()
	utils.Log(RunData, "system")

	// 清空多余数据
	appfiles.DicMD = nil
	appfiles.TTF = nil
	appfiles.Dicstart = nil
}

// func isAvailable(dateStr string) bool {
// 	layout := "2006/1/2"
// 	disabledDate, err := time.Parse(layout, dateStr)
// 	if err != nil {
// juice.ErrorStop("日期解析错误")
// 	}

// 	// 获取当前时间
// 	currentTime := time.Now()

// 	// 检查当前时间是否在指定日期之后，并直接返回结果
// 	return currentTime.After(disabledDate)
// }

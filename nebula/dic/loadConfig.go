package dic

import (
	"fmt"
	"net/http"

	"github.com/cjxpj/nebula/appfiles"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	dic_server "github.com/cjxpj/nebula/server"
	"github.com/cjxpj/nebula/utils"
)

func Start() {

	file := utils.NewFile()
	file.SetPath("README.md").WriteFileByte(appfiles.GetFile("dic.md"))

	loadConfig()

	file.SetPath("private/system/start.n")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/system/start.n"))
	}

	FileData, err := file.ReadFromFile()
	if err != nil {
		utils.ErrorStop("启动词库不存在")
	}

	GV := dto.NewVal()
	GV.Set("版本", appfiles.Version)
	infoDic := dic_dto.NewDic("private/system/start.n", FileData)
	infoDic.SetGlobal_v(GV)

	res := dic_server.Start(dto.ServerConfig.Router.Http.Addr)
	// 遍历res
	for _, t := range res {
		if dicRes := dic_api.Api.DicRun(infoDic, t); dicRes != "" {
			fmt.Println(dicRes)
		}
	}
}

func loadConfig() {

	file := utils.NewFile()

	file.SetPath("private/ttf/font.ttf")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("font.ttf"))
	}

	file.SetPath(dto.CONFIG_SYSTEM_PATH)
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/system/system.ini"))
	}

	httpData, err := file.LoadIni()
	if err != nil {
		utils.ErrorStop("系统配置不存在")
	}

	HTTP_Config := httpData.Section("HTTP")
	infoServerPath := HTTP_Config.Key("server").String()
	corsOk, _ := HTTP_Config.Key("跨域").Bool()
	dto.ServerConfig.Router = &dto.ServerHTTP{
		Cors: corsOk,
		Http: &http.Server{
			Addr:    infoServerPath,
			Handler: http.HandlerFunc(webRun),
		},
	}

	opUi := httpData.Section("管理面板")
	if ok, _ := opUi.Key("启用").Bool(); ok {
		path := opUi.Key("访问路径").String()
		if path == "nebula" {
			fmt.Println("管理面板的密码忘记可以去配置文件看或者自己改，请不要泄漏导致服务器被攻击！")
			path = fmt.Sprintf("%s/%s", path, utils.RandomString("大小字母", 12))
			opUi.Key("访问路径").SetValue(path)
			file.SaveIni(httpData)
		}
		dto.ServerConfig.OPUI = &dto.OPUI{
			Addr: "/" + path,
		}
	}

	file.SetPath(dto.CONFIG_PATH)
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/system/config.ini"))
	}

	botData, err := file.LoadIni()
	if err != nil {
		utils.ErrorStop("对接配置不存在")
	}

	// 路由词库
	file.SetPath("private/system/router.n")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/system/router.n"))

		// 主页文件
		file.SetPath("public")
		if !file.DirExists() {
			// 默认主页
			file.SetPath("public/index.wn")
			file.WriteToFile(appfiles.GetFileString("dic/public/index.wn"))
			// 默认图标
			file.SetPath("public/favicon.ico")
			file.WriteFileByte(appfiles.GetFile("dic/public/favicon.ico"))
			// 默认样板文件
			file.SetPath("public/api.n")
			file.WriteToFile(appfiles.GetFileString("dic/public/api.n"))
			// 404文件
			file.SetPath("public/404.wn")
			file.WriteFileByte(appfiles.GetFile("dic/public/404.wn"))
		}
	}

	Ngrok_Config := httpData.Section("Ngrok")
	if ok, _ := Ngrok_Config.Key("启用").Bool(); ok {
		ngrokUrl := Ngrok_Config.Key("访问链接").String()
		authToken := Ngrok_Config.Key("密钥").String()
		dto.ServerConfig.Ngrok = &dto.NgrokConfig{
			Addr:  ngrokUrl,
			Token: authToken,
		}
	}

	WebSocket_Config := httpData.Section("WebSocket")
	dto.LoadConfig_websocket(WebSocket_Config)

	QQBot_Config := botData.Section("QQ")
	dto.LoadConfig_qq(QQBot_Config)

	NapCat_Config := botData.Section("NapCat")
	dto.LoadConfig_napcat(NapCat_Config)

	YunHu_Config := botData.Section("云湖")
	dto.LoadConfig_yunhu(YunHu_Config)

	FeiShu_Config := botData.Section("飞书")
	dto.LoadConfig_feishu(FeiShu_Config)

}

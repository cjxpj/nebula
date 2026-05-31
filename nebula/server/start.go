package dic_server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
)

// 启动服务器
func Start(infoServerPath string) []string {
	res := make([]string, 0)
	if dto.ServerConfig.Ngrok != nil {
		authToken := dto.ServerConfig.Ngrok.Token
		ngrokUrl := dto.ServerConfig.Ngrok.Addr
		ngrokUrlHttp := config.HTTPEndpoint()
		// if authToken == "2960965389" {
		// 	authToken = "2abzrmBDIPyXUkuPdxCYmjTJJDa_2LBiWGFFwewXpxFd4KU3n"
		// 	ngrokUrl = "brave-sunfish-lightly.ngrok-free.app"
		// 	// authTokenOptions := []string{
		// 	// 	"2abzrmBDIPyXUkuPdxCYmjTJJDa_2LBiWGFFwewXpxFd4KU3n",
		// 	// 	"2fxaUPFlFox97uGLr2WRM5XSMwO_4X6fihRKMNzwwgM6pgAFr",
		// 	// }
		// 	// authToken = authTokenOptions[rand.Intn(len(authTokenOptions))]
		// }
		if ngrokUrl != "" {
			ngrokUrlHttp = config.HTTPEndpoint(
				config.WithDomain(ngrokUrl),
			)
		}
		if listener, err := ngrok.Listen(context.Background(),
			ngrokUrlHttp,
			ngrok.WithAuthtoken(authToken),
		); err == nil {
			go func() {
				if err = http.Serve(listener, dto.ServerConfig.Router.Http.Handler); err != nil {
					utils.Error("Ngrok启动失败>" + err.Error())
					panic(err)
				}
			}()

			// return fmt.Sprintf("Ngrok启动成功 %s", listener.URL())
			res = append(res, fmt.Sprintf("Ngrok启动成功 %s", listener.URL()))
		} else {
			debugLog.Errorf("Ngrok配置失败>%v", err)
		}
	}

	// 使用 Goroutine 启动服务器
	go func() {
		if err := dto.ServerConfig.Router.Http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			utils.Error("启动失败>" + err.Error())
			panic(err)
		}
	}()

	if _, port, err := net.SplitHostPort(dto.ServerConfig.Router.Http.Addr); err == nil {
		fmt.Println("WebUi：", fmt.Sprintf("http://%s:%s%s", "localhost", port, dto.ServerConfig.OPUI.Addr))
	}

	res = append(res, "Main")
	return res
}

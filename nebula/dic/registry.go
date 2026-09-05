package dic

import (
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
)

type f = dto.RegisterDicFunc

func init() {
	go funcs.Setup()
	go funcs.Registers(
		f{Name: "执行词库", L: "1|2|3", Fn: runDic},
		f{Name: "执行词库文件", L: "1|2|3", Fn: runDicFile},
		f{Name: "回调", L: "1..", Fn: callDic},
		f{Name: "重定向触发词", L: "1", Fn: redirectTrigger},
		f{Name: "执行PHP网页词库", L: "1", Fn: runWebPHPDic},
		f{Name: "执行PHP网页词库文件", L: "1", Fn: runWebPHPDicFile},
		f{Name: "执行网页词库", L: "1", Fn: runWebDic},
		f{Name: "执行网页词库文件", L: "1", Fn: runWebDicFile},
		f{Name: "终端_监听执行", L: "2", Fn: cmdListenRun},
		f{Name: "WS连接", L: "1|2", Fn: wsConnect},
		f{Name: "WS断开", L: "1", Fn: wsClose},
		f{Name: "WS发送", L: "2", Fn: wsSend},
		f{Name: "创建WS", L: "2|3", Fn: wsCreate},
		f{Name: "创建服务器", L: "2", Fn: createServer},
		f{Name: "编译词库", L: "1", Fn: compileDic},
		f{Name: "核心服务器", L: "0", Fn: coreServer},
		f{Name: "设置Ngrok", L: "0|1|2|3|4", Fn: setNgrok},
		f{Name: "读词库", L: "1|2|3", Fn: readDicFile},
		f{Name: "写词库", L: "1|2|3", Fn: writeDicFile},
	)
}

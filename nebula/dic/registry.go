package dic

import (
	"github.com/cjxpj/nebula/dic/funcs"
)

func init() {
	go funcs.Setup()
	go funcs.Register("执行词库", "1|2|3", runDic)
	go funcs.Register("执行词库文件", "1|2|3", runDicFile)
	go funcs.Register("回调", "1..", callDic)
	go funcs.Register("执行网页词库", "1", runWebDic)
	go funcs.Register("执行网页词库文件", "1", runWebDicFile)
	go funcs.Register("终端.监听执行", "2", cmdListenRun)
	go funcs.Register("函数", "1|2", runFunc)
	go funcs.Register("异步函数", "1|2", runAsyncFunc)
}

package run

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/utils"
)

// chdirToAppWin 切换到 nebula/app/win（NebulaData 所在），基于源码文件位置计算绝对路径，多次调用幂等
func chdirToAppWin() {
	_, file, _, _ := runtime.Caller(0)
	winDir := filepath.Join(filepath.Dir(file), "..", "app", "win")
	if err := os.Chdir(winDir); err != nil {
		panic(err)
	}
}

func Test_log(t *testing.T) {
	t.Run("build_dic", func(t *testing.T) {
		chdirToAppWin()
		text := strings.NewReader(`开头
		#引入=f
		
		Main
		$test ok$

		[F]test a b=ok2 #{
		%a%
		
		>
		
		$b%
		}#

		[L]内部
		ok` + "\r\n" + `3

		`)
		r := Parse("test.n", text)
		debugLog.Infof("===========词库===========")
		debugLog.Infof("%v", utils.AnyToString(r))
		debugLog.Infof("===========结尾===========")
	})
}

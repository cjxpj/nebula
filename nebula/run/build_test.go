package run

import (
	"strings"
	"testing"

	"os"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/utils"
)

func Test_log(t *testing.T) {
	t.Run("build_dic", func(t *testing.T) {
		err := os.Chdir("../../nebulaApp/win")
		if err != nil {
			panic(err)
		}
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

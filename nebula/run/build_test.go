package run

import (
	"fmt"
	"strings"
	"testing"

	"os"

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
		fmt.Println("===========词库===========")
		fmt.Println(utils.AnyToString(r))
		fmt.Println("===========结尾===========")
	})
}

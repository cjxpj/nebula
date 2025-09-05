package funcs

import (
	"fmt"
	"strings"

	"github.com/cjxpj/nebula/utils"

	"github.com/cjxpj/nebula/dto"
)

func log(d *dto.DicInputs) (any, error) {
	switch d.Inputs.StringDefault(2, "成功") {
	case "成功":
		utils.Log(strings.ReplaceAll(d.Inputs.String(1), "\\n", "\n"))
	case "失败":
		utils.Error(strings.ReplaceAll(d.Inputs.String(1), "\\n", "\n"))
	}
	return "", nil
}

func print(d *dto.DicInputs) (any, error) {
	fmt.Println(d.Inputs.List[1:]...)
	return "", nil
}

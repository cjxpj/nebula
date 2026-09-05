package dic

import (
	"os"
	"path/filepath"
	"testing"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
)

// TestResourceDirective 验证 //@资源 编译期资源指令（多行块形式）：
// //@资源 单独一行，下一行「变量名:文件路径」读入文件内容，
// 编译期读入、运行时注入局部变量表，供 %变量名% 引用。
// 使用绝对路径，避免依赖全局应用数据目录状态。
func TestResourceDirective(t *testing.T) {
	chdirToAppWin()
	dir := t.TempDir()

	txt := filepath.Join(dir, "res.txt")
	if err := os.WriteFile(txt, []byte("第一行\n第二行"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 多行块形式（文本）
	D := dic_dto.NewDic("t.n", "//@资源\n内容:"+txt+"\n\nMain\n%内容%")
	if got := dic_api.Api.DicRun(D, "Main"); got != "第一行\n第二行" {
		t.Errorf("文本资源加载错误，得到 %q", got)
	}
}

// TestOnceResourceDirective 验证 //@一次性资源 编译期资源指令：
// 与 //@资源 一样编译期读入文件内容、运行时注入局部变量表；
// 区别是变量读取一次后即销毁，再次 %变量名% 读取表现为未定义（渲染为 %变量名%）。
func TestOnceResourceDirective(t *testing.T) {
	chdirToAppWin()
	dir := t.TempDir()

	txt := filepath.Join(dir, "res.txt")
	if err := os.WriteFile(txt, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 同一正文连续读取两次：第一次读到内容，第二次已销毁
	D := dic_dto.NewDic("t.n", "//@一次性资源\n内容:"+txt+"\n\nMain\n%内容%%内容%")
	if got := dic_api.Api.DicRun(D, "Main"); got != "hello%内容%" {
		t.Errorf("一次性资源应读取一次后销毁，得到 %q", got)
	}
}

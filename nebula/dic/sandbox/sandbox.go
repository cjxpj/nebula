// Package sandbox 是引擎内置的词库沙箱，收敛词库（.n）能触达的能力边界。
//
// 文件读写由引擎自身负责限制：词库里的路径只允许相对路径，且必须落在该词库自己所在的
// 目录内（见 nebula/dic/funcs/file.go 的 resolveDicPath），越界与绝对路径一律拒绝。
// 平台侧调用本包即可获得统一的默认策略：
//
//	Sandbox：把引擎工作目录收到词库根目录，兜住仍按工作目录解析的引擎内部能力；
//	Block：注销会执行系统命令、控制进程、起常驻协程的函数；
//	NetGuard：给词库的出网（访问/下载/绘图/MySQL 等）加统一守卫，禁止打到内网。
//
// 宿主平台也可以用 Scan 对用户提交的词库内容做静态拦截，把恶意词库挡在上传阶段。
package sandbox

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/utils"
)

// blocked 需要注销的内置函数，按用途分组。
// 这些能力宿主平台用不到，且一旦被用户词库调用就能读到词库目录之外的文件、
// 拿到项目数据库里的机器人密钥，或者直接控制整个进程。
var blocked = []string{
	"创建终端", "创建Shell终端", "终端_监听执行",
	"STOP", "重启",
	"添加定时任务", "删除定时任务", "定时任务列表",
	"创建WS", "WS连接", "WS断开", "WS发送",
	"创建服务器", "核心服务器", "设置Ngrok",
	"设置工作目录",
	"线程变量",
	"PHP", "Python", "执行DEX", "Shizuku检查", "Shizuku执行",
	"回收站", "打开浏览器", "音频转Silk", "发送通知",
	"设备信息", "设备电量",
}

// blockedSet 是 blocked 的查找集合
var blockedSet = func() map[string]bool {
	m := make(map[string]bool, len(blocked))
	for _, name := range blocked {
		m[name] = true
	}
	return m
}()

// Enable 一键启用内置沙箱：限定工作目录、注销高危函数、禁止出网访问内网。
// 返回实际注销掉的高危函数名。
func Enable(dir string) []string {
	Sandbox(dir)
	NetGuard()
	return Block()
}

// Sandbox 把引擎的工作目录限定为 dir，此后词库的文件类函数只在 dir 内读写。
func Sandbox(dir string) {
	utils.SetAppDir(dir)
}

// Block 注销 blocked 中的内置函数，返回实际注销掉的名字。
func Block() []string {
	removed := make([]string, 0, len(blocked))
	for _, name := range blocked {
		if _, ok := funcs.GetFunc(name); !ok {
			continue
		}
		funcs.Unregister(name)
		removed = append(removed, name)
	}
	return removed
}

// NetGuard 安装统一出网守卫：词库发起的外连（访问/访问POST/访问转发/新建访问/
// 下载文件/绘图取图/腾讯云 API/新建mysql 等）一律禁止访问内网地址。
// 公网访问不受影响，因此「新建mysql」连远程数据库仍然可用。
func NetGuard() {
	utils.SetNetGuard(func(ip net.IP) error {
		if isLAN(ip) {
			return fmt.Errorf("禁止访问内网地址 %s", ip)
		}
		return nil
	})
}

// isLAN 判断 IP 是否落在需要禁止的内网范围内：
// 私有段（10/172.16/192.168、fc00::/7）、回环（127/8、::1）、
// 未指定（0.0.0.0、::）、链路本地（169.254/16、fe80::/10）以及多播。
func isLAN(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast()
}

// callRe 匹配 $函数名 参数…$ 形式的调用
var callRe = regexp.MustCompile(`\$([^$\n]{0,200}?)\$`)

// Scan 扫描词库内容，返回其中调用到的被禁用函数名（去重、按名称排序）。
// 行内 // 之后的注释会被忽略。
func Scan(content string) []string {
	hit := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		for _, m := range callRe.FindAllStringSubmatch(line, -1) {
			fields := strings.Fields(m[1])
			if len(fields) == 0 {
				continue
			}
			if blockedSet[fields[0]] {
				hit[fields[0]] = true
			}
		}
	}
	out := make([]string, 0, len(hit))
	for name := range hit {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

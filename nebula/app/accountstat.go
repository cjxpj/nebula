//go:build dll

// accountstat.go 提供引擎侧「按账号」的词库执行并发限制与 CPU 占用统计。
//
// 背景：单个 DLL 进程内所有账号共享同一个 Go 运行时，无法做 OS 级核绑定。这里以
// 「按账号控制词库执行的并发度」实现管理员分配 CPU 的效果：
//   - 每个账号持有一个容量 = 分配核数的令牌（默认 1），词库执行前取令牌、结束后归还，
//     因此「分配 N 核」= 允许该账号最多 N 个词库执行并行；
//   - 同时累计每个账号词库执行占用的墙钟时间，两次采样间隔内折算为「占 1 核的百分比」。
//
// 账号键由词库路径反推：.../<uid>/机器人词库|储存|网站词库[/...] → uid。
package main

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	dic "github.com/cjxpj/nebula/dic"
	"github.com/cjxpj/nebula/utils"
)

// defaultAccountCores 未单独配置时每个账号允许的词库执行并行度（核）
const defaultAccountCores = 1

// maxAccountCores 单账号可分配的最大核数，防止把引擎线程池压垮
const maxAccountCores = 64

// acctState 单个账号的执行并发限制与耗时累计
type acctState struct {
	mu    sync.Mutex
	cond  *sync.Cond
	cores int   // 分配核数（并发上限）
	busy  int   // 当前正在执行的词库数
	acc   int64 // 累计执行耗时（纳秒）
	prev  int64 // 上次采样时的累计执行耗时
	prevT time.Time
	// vipExpire 会员到期时间（unix 秒），由宿主下发；执行时 now < vipExpire 视为会员。
	// 会员词库执行超时上限 1 分钟，非会员 10 秒。
	vipExpire int64
}

var (
	acctMu sync.Mutex
	accts  = map[string]*acctState{}
)

func newAcctState(cores int) *acctState {
	if cores < 1 {
		cores = defaultAccountCores
	}
	a := &acctState{cores: cores, prevT: time.Now()}
	a.cond = sync.NewCond(&a.mu)
	return a
}

// accountKeyOfPath 由词库路径反推账号键（uid）；不在账号目录下返回空串。
func accountKeyOfPath(path string) string {
	if path == "" {
		return ""
	}
	dir, ok := utils.AccountRootOf(path)
	if !ok {
		return ""
	}
	return filepath.Base(dir)
}

func acctStateOf(key string) *acctState {
	acctMu.Lock()
	defer acctMu.Unlock()
	a := accts[key]
	if a == nil {
		a = newAcctState(defaultAccountCores)
		accts[key] = a
	}
	return a
}

// accountBegin 在账号维度开始一次词库执行：先按并发上限取令牌（满则阻塞等待），再开始计时。
// 返回的 done 必须被调用（defer），用于累计耗时并归还令牌。
// 路径无法反推账号时返回 nil，表示不限制、不统计。
func accountBegin(path string) func() {
	key := accountKeyOfPath(path)
	if key == "" {
		return nil
	}
	a := acctStateOf(key)
	a.mu.Lock()
	for a.busy >= a.cores {
		a.cond.Wait()
	}
	a.busy++
	a.mu.Unlock()

	start := time.Now()
	return func() {
		elapsed := time.Since(start).Nanoseconds()
		a.mu.Lock()
		a.acc += elapsed
		a.busy--
		a.cond.Signal()
		a.mu.Unlock()
	}
}

// setAccountCores 设置某账号可用的词库执行并发上限（核）。cores<1 视为默认值。
func setAccountCores(key string, cores int) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if cores < 1 {
		cores = defaultAccountCores
	}
	if cores > maxAccountCores {
		cores = maxAccountCores
	}
	a := acctStateOf(key)
	a.mu.Lock()
	a.cores = cores
	a.cond.Broadcast() // 调大后唤醒等待中的执行
	a.mu.Unlock()
}

// acctStat 单个账号的 CPU 占用快照
type acctStat struct {
	Cores   int     `json:"cores"`   // 分配核数
	Active  int     `json:"active"`  // 当前并行执行的词库数
	Percent float64 `json:"percent"` // 占 1 核的百分比（>100 表示同时占用多核）
}

// accountStats 采样各账号的 CPU 占用：以「上次采样至今累计执行耗时 / 实际间隔」折算为
// 占 1 核的百分比。宿主按固定周期调用即可得到近实时占用。
func accountStats() map[string]acctStat {
	now := time.Now()
	acctMu.Lock()
	states := make(map[string]*acctState, len(accts))
	for k, v := range accts {
		states[k] = v
	}
	acctMu.Unlock()

	out := make(map[string]acctStat, len(states))
	for k, a := range states {
		a.mu.Lock()
		delta := a.acc - a.prev
		span := now.Sub(a.prevT).Seconds()
		a.prev = a.acc
		a.prevT = now
		stat := acctStat{Cores: a.cores, Active: a.busy}
		a.mu.Unlock()

		if span > 0 && delta > 0 {
			stat.Percent = float64(delta) / 1e9 / span * 100
		}
		out[k] = stat
	}
	return out
}

// dllSetUserCores 供宿主下发某账号可用的词库执行并发上限（核）。
func dllSetUserCores(key string, cores int) string {
	return guarded(func() string {
		setAccountCores(key, cores)
		return okEnv(nil)
	})
}

// setAccountVipExpire 记录某账号的会员到期时间（unix 秒），供词库执行超时按会员状态取限。
func setAccountVipExpire(key string, expire int64) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	a := acctStateOf(key)
	a.mu.Lock()
	a.vipExpire = expire
	a.mu.Unlock()
}

// dicTimeoutLimitOfPath 按词库路径解析该账号的词库执行超时上限：会员 1 分钟、非会员 10 秒。
// 路径不在账号目录下时返回 0，由 dic 层回退默认值。
func dicTimeoutLimitOfPath(path string) time.Duration {
	key := accountKeyOfPath(path)
	if key == "" {
		return 0
	}
	a := acctStateOf(key)
	a.mu.Lock()
	expire := a.vipExpire
	a.mu.Unlock()
	if expire > time.Now().Unix() {
		return dic.MemberDicRunTimeout
	}
	return dic.GuestDicRunTimeout
}

// dllSetUserVip 供宿主下发某账号的会员到期时间（unix 秒）。
func dllSetUserVip(key string, expire int64) string {
	return guarded(func() string {
		setAccountVipExpire(key, expire)
		return okEnv(nil)
	})
}

// init 注入超时解析器：引擎每次词库执行按账号会员状态取超时上限。
func init() {
	dic.SetDicRunTimeoutResolver(dicTimeoutLimitOfPath)
}

// dllAccountStats 供宿主采样各账号的 CPU 占用快照（key 为账号 uid）。
func dllAccountStats() string {
	return guarded(func() string {
		return okEnv(accountStats())
	})
}

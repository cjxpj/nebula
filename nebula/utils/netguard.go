package utils

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// netGuard 由宿主注入的出网守卫：返回非 nil 表示拒绝该 IP。
// 未安装时不做任何限制，保证引擎独立运行时行为不变。
var (
	netGuardMu sync.RWMutex
	netGuard   func(net.IP) error
)

// SetNetGuard 安装全局出网守卫，传 nil 表示取消守卫。
// 引擎里所有词库可触达的 HTTP 客户端与 MySQL 拨号都应经过该守卫。
func SetNetGuard(fn func(net.IP) error) {
	netGuardMu.Lock()
	netGuard = fn
	netGuardMu.Unlock()
}

// currentNetGuard 读取当前守卫
func currentNetGuard() func(net.IP) error {
	netGuardMu.RLock()
	defer netGuardMu.RUnlock()
	return netGuard
}

// GuardIP 用当前守卫校验单个 IP，守卫未安装或放行时返回 nil。
func GuardIP(ip net.IP) error {
	if fn := currentNetGuard(); fn != nil {
		return fn(ip)
	}
	return nil
}

// GuardAddr 校验 host:port 目标地址是否被守卫允许。
// 供无法注入 dialer 的场景（如 SMTP）在连接前做预检。
func GuardAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return fmt.Errorf("目标地址为空")
	}
	if fn := currentNetGuard(); fn == nil {
		return nil
	}
	_, err = resolveGuardHost(context.Background(), host)
	return err
}

// resolveGuardHost 解析主机名并逐个校验 IP，返回校验通过的 IP 列表。
// 返回解析结果供调用方直接拨号，避免二次解析被 DNS rebinding 绕过。
func resolveGuardHost(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if err := GuardIP(ip); err != nil {
			return nil, err
		}
		return []net.IP{ip}, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("解析域名 %s 失败: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("域名 %s 无解析结果", host)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, ia := range addrs {
		if err := GuardIP(ia.IP); err != nil {
			return nil, err
		}
		ips = append(ips, ia.IP)
	}
	return ips, nil
}

// GuardedDialContext 带守卫的拨号：先解析并校验目标 IP，再用校验过的 IP 直连。
// 未安装守卫时退化为普通拨号，行为与之前一致。
func GuardedDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	if currentNetGuard() == nil {
		return dialer.DialContext(ctx, network, addr)
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// 无端口时按原样交给底层处理
		return dialer.DialContext(ctx, network, addr)
	}

	ips, err := resolveGuardHost(ctx, host)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, ip := range ips {
		conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("无法连接目标 %s", addr)
	}
	return nil, lastErr
}

// GuardTransport 给 http.Transport 装上出网守卫并返回同一实例。
// t 为 nil 时新建一个空的 Transport。
func GuardTransport(t *http.Transport) *http.Transport {
	if t == nil {
		t = &http.Transport{}
	}
	t.DialContext = GuardedDialContext
	return t
}

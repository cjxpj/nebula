package funcs

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dto"
)

// dnsResolve 解析域名。
// 参数1：域名或 IP（允许带 http(s):// 前缀、路径与端口，会自动提取主机名）。
// 参数2（可选）：记录类型，默认 A。支持 A/AAAA/IP/CNAME/MX/NS/TXT/SRV/PTR。
// 返回：命中记录以 JSON 数组字符串返回；无结果或解析失败返回 "[]"。
func dnsResolve(d *dto.DicInputs) (any, error) {
	name := dnsHostOf(d.Inputs.String(1))
	if name == "" {
		return "[]", nil
	}
	typ := strings.ToUpper(strings.TrimSpace(d.Inputs.StringDefault(2, "A")))
	if typ == "" {
		typ = "A"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := net.DefaultResolver

	empty := func() (any, error) { return "[]", nil }

	switch typ {
	case "A", "IPV4":
		ips, err := r.LookupIP(ctx, "ip4", name)
		if err != nil {
			return empty()
		}
		return dnsToJSON(dnsIPStrings(ips))
	case "AAAA", "IPV6":
		ips, err := r.LookupIP(ctx, "ip6", name)
		if err != nil {
			return empty()
		}
		return dnsToJSON(dnsIPStrings(ips))
	case "IP":
		addrs, err := r.LookupIPAddr(ctx, name)
		if err != nil {
			return empty()
		}
		out := make([]string, 0, len(addrs))
		for _, a := range addrs {
			out = append(out, a.IP.String())
		}
		return dnsToJSON(out)
	case "CNAME":
		cname, err := r.LookupCNAME(ctx, name)
		if err != nil {
			return empty()
		}
		return dnsToJSON([]string{strings.TrimSuffix(cname, ".")})
	case "MX":
		list, err := r.LookupMX(ctx, name)
		if err != nil {
			return empty()
		}
		out := make([]string, 0, len(list))
		for _, m := range list {
			out = append(out, strings.TrimSuffix(m.Host, "."))
		}
		return dnsToJSON(out)
	case "NS":
		list, err := r.LookupNS(ctx, name)
		if err != nil {
			return empty()
		}
		out := make([]string, 0, len(list))
		for _, ns := range list {
			out = append(out, strings.TrimSuffix(ns.Host, "."))
		}
		return dnsToJSON(out)
	case "TXT":
		list, err := r.LookupTXT(ctx, name)
		if err != nil {
			return empty()
		}
		return dnsToJSON(list)
	case "SRV":
		_, list, err := r.LookupSRV(ctx, "", "", name)
		if err != nil {
			return empty()
		}
		out := make([]string, 0, len(list))
		for _, s := range list {
			out = append(out, strings.TrimSuffix(s.Target, "."))
		}
		return dnsToJSON(out)
	case "PTR":
		list, err := r.LookupAddr(ctx, name)
		if err != nil {
			return empty()
		}
		out := make([]string, 0, len(list))
		for _, p := range list {
			out = append(out, strings.TrimSuffix(p, "."))
		}
		return dnsToJSON(out)
	}
	return empty()
}

// dnsToJSON 把记录列表编码为 JSON 数组字符串。
func dnsToJSON(list []string) (any, error) {
	if len(list) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]", nil
	}
	return string(b), nil
}

// dnsIPStrings 把 net.IP 列表转换为字符串列表。
func dnsIPStrings(ips []net.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out
}

// dnsHostOf 从任意输入中提取主机名：兼容 https://host/path?x=1、host:port、[::1]:port、末尾点等写法。
func dnsHostOf(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	} else if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		s = s[1 : len(s)-1]
	}
	return strings.TrimSuffix(strings.TrimSpace(s), ".")
}

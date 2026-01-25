package utils

import (
	"net"
	"net/http"
	"strings"
)

func GetClientIP(r *http.Request) string {
	// 尝试从X-Forwarded-For头获取IP
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For可能包含多个IP，通常第一个是最原始的客户端IP
		parts := strings.Split(forwarded, ", ")
		if len(parts) > 0 && net.ParseIP(parts[0]) != nil {
			return parts[0]
		}
	}

	// 尝试从X-Real-IP头获取IP
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" && net.ParseIP(realIP) != nil {
		return realIP
	}

	// 如果X-Forwarded-For和X-Real-IP都不存在或无效，则回退到RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return ip
}

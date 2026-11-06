package main

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

// TargetURL 是规范化后的目标地址。
type TargetURL struct {
	Scheme   string // 输入里显式给出的 scheme；空表示 https->http 自动回退
	Host     string // punycode 小写主机名
	Port     string
	Path     string
	RawQuery string
}

func (t TargetURL) String(scheme string) string {
	host := t.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if t.Port != "" {
		host += ":" + t.Port
	}
	u := url.URL{Scheme: scheme, Host: host, Path: t.Path}
	if t.RawQuery != "" {
		u.RawQuery = t.RawQuery
	}
	return u.String()
}

// normalize 兼容 "example.com"、"example.com/path"、"http://..."、"https://..."
// 以及带端口的写法。
func normalize(raw string) (TargetURL, error) {
	var out TargetURL

	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "\ufeff")
	s = strings.Trim(s, "\"'")
	if s == "" {
		return out, fmt.Errorf("空字符串")
	}
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = s[:i]
	}

	if i := strings.Index(s, "://"); i >= 0 {
		scheme := strings.ToLower(strings.TrimSpace(s[:i]))
		if scheme != "http" && scheme != "https" {
			return out, fmt.Errorf("不支持的协议 %q", scheme)
		}
		out.Scheme = scheme
		s = s[i+3:]
	}
	s = strings.TrimPrefix(s, "//")

	if s == "" {
		return out, fmt.Errorf("缺少主机名")
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return out, fmt.Errorf("包含空白字符")
	}

	authority := s
	rest := ""
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		authority, rest = s[:i], s[i:]
	}
	if authority == "" {
		return out, fmt.Errorf("缺少主机名")
	}
	if strings.Contains(authority, "@") {
		return out, fmt.Errorf("不支持带用户信息的地址")
	}

	host, port := splitHostPort(authority)
	if host == "" {
		return out, fmt.Errorf("缺少主机名")
	}

	if ip := net.ParseIP(host); ip == nil {
		if isASCII(host) {
			// 纯 ASCII 主机名不走 IDNA：DNS 允许下划线（如 fcon_1000.projects.nitrc.org），
			// 而 idna.Lookup 会拒绝。
			if !validASCIIHost(host) {
				return out, fmt.Errorf("非法主机名 %q", host)
			}
			host = strings.ToLower(host)
		} else {
			ascii, err := idna.Lookup.ToASCII(host)
			if err != nil {
				return out, fmt.Errorf("非法主机名 %q: %w", host, err)
			}
			host = strings.ToLower(ascii)
		}
	} else {
		host = ip.String()
	}
	if host == "" || strings.HasPrefix(host, ".") || strings.Contains(host, "..") {
		return out, fmt.Errorf("非法主机名")
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return out, fmt.Errorf("非法端口 %q", port)
		}
	}

	out.Host = host
	out.Port = port
	out.Path = "/"
	if rest != "" {
		if strings.HasPrefix(rest, "?") {
			out.RawQuery = rest[1:]
		} else if i := strings.IndexByte(rest, '?'); i >= 0 {
			out.Path = rest[:i]
			out.RawQuery = rest[i+1:]
		} else {
			out.Path = rest
		}
	}
	if !strings.HasPrefix(out.Path, "/") {
		out.Path = "/" + out.Path
	}
	return out, nil
}

func splitHostPort(s string) (host, port string) {
	if strings.HasPrefix(s, "[") {
		if i := strings.Index(s, "]"); i >= 0 {
			host = s[1:i]
			if rest := s[i+1:]; strings.HasPrefix(rest, ":") {
				port = rest[1:]
			}
			return host, port
		}
	}
	if strings.Count(s, ":") == 1 {
		if i := strings.LastIndexByte(s, ':'); i >= 0 {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

// isASCII 判断字符串是否全为 ASCII。
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

// validASCIIHost 只放行 DNS 主机名常见字符（含下划线，兼容部分学术站点）。
func validASCIIHost(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '.', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// isPrivateHost 判断字面 IP 是否属于不应从公网探测的地址段。
func isPrivateHost(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

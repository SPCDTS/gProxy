package utils

import (
	"errors"
	"net"
	"net/http"
	"strings"
)

/*
* X-Real-IP：只包含客户端机器的一个IP，如果为空，某些代理服务器（如Nginx）会填充此header。
* X-Forwarded-For：一系列的IP地址列表，以,分隔，每个经过的代理服务器都会添加一个IP。
* RemoteAddr：包含客户端的真实IP地址。这是Web服务器从其接收连接并将响应发送到的实际物理IP地址。但是，如果客户端通过代理连接，它将提供代理的IP地址。
 */
// GetIP returns request real ip.
func GetIP(r *http.Request) (string, error) {
	ip := r.Header.Get("X-Real-IP")
	if net.ParseIP(ip) != nil {
		return ip, nil
	}

	ip = r.Header.Get("X-Forward-For")
	for _, i := range strings.Split(ip, ",") {
		if net.ParseIP(i) != nil {
			return i, nil
		}
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", err
	}

	if net.ParseIP(ip) != nil {
		return ip, nil
	}

	return "", errors.New("no valid ip found")
}

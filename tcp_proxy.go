package gproxy

import (
	"encoding/json"
	"fmt"
	epio "g-proxy/epio"
	"log"
	"net"
	"os"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

const dataFile = "/app/proxyEntry.json"

type PortProxy struct {
	Server *net.TCPAddr
	lcp    int // listen client port, proxy server在这个端口侦听client的连接
	done   chan struct{}
}

func NewPortProxy(server *net.TCPAddr) *PortProxy {
	return &PortProxy{
		Server: server,
		done:   make(chan struct{}),
	}
}

// 通过判断done channel是否打开来确定是否正在进行转发
func (p PortProxy) Running() bool {
	return p.done != nil
}

// 新增代理对
func (p *ProxyServer) addProxy(name string, addr *net.TCPAddr) {
	proxyPair, ok := p.proxyDict[name]
	if !ok {
		proxyPair = NewPortProxy(addr)
		p.proxyDict[name] = proxyPair
	}
	proxyPair.Server = addr
	Map2File(p.proxyDict)
}

// 侦听对应代理服务的端口
func (p *ProxyServer) tcpListen(name string) string {
	proxy := p.proxyDict[name]
	proxy.lcp = <-p.port
	addr := localIP + ":" + strconv.Itoa(proxy.lcp)

	acceptor, err := epio.NewAcceptor(p.forAccept, p.forNewFd,
		func() epio.EvHandler { return NewProxyC(p.connector, proxy.Server.String()) },
		addr,
		epio.ListenBacklog(256),
		epio.SockRcvBufSize(8*1024))
	if err != nil {
		return ""
	}
	go func() {
		<-proxy.done
		close(acceptor.Close)
		p.port <- proxy.lcp
		fmt.Println("port " + strconv.Itoa(proxy.lcp) + " returned")
	}()
	// 返回绑定的地址
	log.Printf("正在侦听: %s\n", addr)
	return addr
}

func reuseConfig() net.ListenConfig {
	cfg := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})
		},
	}
	return cfg
}

func Map2File(dic map[string]*PortProxy) (err error) {
	fPtr, err := os.Create(dataFile)

	if err != nil {
		return
	}

	fmt.Printf("正在写入: %s\n", dataFile)
	defer fPtr.Close()
	encoder := json.NewEncoder(fPtr)
	err = encoder.Encode(dic)
	return
}

func File2Map(dic *map[string]*PortProxy) (err error) {
	fPtr, err := os.Open(dataFile)
	if err != nil {
		return
	}
	defer fPtr.Close()
	decoder := json.NewDecoder(fPtr)
	decoder.Decode(&dic)
	return
}

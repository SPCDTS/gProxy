package proxy_server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"
)

const jsonContentType = "application/json"
const Client = "client"
const Server = "server"
const localIP = "172.119.1.2"
const startPort = 33333
const endPort = 33444

var logger = log.Default()

type ProxyServer struct {
	http.Handler
	proxyDict    map[string]*portProxy
	clientIP     string
	serverIP     string
	proxyMinPort int
	proxyMaxPort int
}

// 侦听对应代理服务的端口
func (p *ProxyServer) tcpListen(name string) string {
	// heartbeatStream := make(chan interface{}, 1)
	proxy := p.proxyDict[name]
	ln, err := CustomListen(p.serverIP, p.proxyMinPort, p.proxyMaxPort, 5)
	if err != nil {
		fmt.Println(err)
		return ""
	}

	log.Printf("正在侦听: %s\n", ln.Addr().String())

	cnn_chan := make(chan net.Conn, 1)
	// 新建一个goroutine去不断地侦听端口，当ln被close的时候，会退出
	go func() {
		for {
			tcp_Conn, err := ln.Accept()
			if err != nil {
				fmt.Printf("停止接收连接: %s\n", err)
				return
			}
			cnn_chan <- tcp_Conn
		}
	}()

	go func() {
		defer ln.Close()
		for {
			select {
			case tcp_Conn := <-cnn_chan:
				go p.tcpHandle(proxy.Server, tcp_Conn) //创建新的协程进行转发
			case <-proxy.done:
				close(cnn_chan)
				fmt.Println("proxy server: close client connection.")
				return
			}
		}
	}()

	return ln.Addr().String()

}

// 处理建立的连接
func (p *ProxyServer) tcpHandle(server net.TCPAddr, tcpConn net.Conn) {

	remote_tcp, err := CustomConn(5*time.Second, localIP, &server, p.proxyMinPort, p.proxyMaxPort, 5)
	if err != nil {
		fmt.Printf("无法连接至目标服务器: %s\n", err)
		if remote_tcp != nil {
			remote_tcp.Close()
		}
		tcpConn.Close()
		fmt.Println("Client closed")
		return
	}

	go func() {
		defer tcpConn.Close()
		defer fmt.Println("Client closed")
		defer remote_tcp.Close()
		defer fmt.Println("Server closed")
		fmt.Printf("proxy-server:%s <-- %s == %s <-- %s\n", remote_tcp.RemoteAddr(), remote_tcp.LocalAddr(), tcpConn.LocalAddr(), tcpConn.RemoteAddr())
		io.Copy(remote_tcp, tcpConn)
		//handleConnection(remote_tcp, tcpConn)
	}()

	go func() {
		defer tcpConn.Close()
		defer fmt.Println("Client closed")
		defer remote_tcp.Close()
		defer fmt.Println("Server closed")
		fmt.Printf("proxy-server:%s --> %s == %s --> %s\n", remote_tcp.RemoteAddr(), remote_tcp.LocalAddr(), tcpConn.LocalAddr(), tcpConn.RemoteAddr())
		io.Copy(tcpConn, remote_tcp)
		//handleConnection(tcpConn, remote_tcp)
	}()

}

// 新增代理对
func (p *ProxyServer) addProxy(name string, addr net.TCPAddr, position string) {
	proxyPair, ok := p.proxyDict[name]
	if !ok {
		proxyPair = new(portProxy)
	}
	switch position {
	case Client:
		proxyPair.Client = addr
	case Server:
		proxyPair.Server = addr
	default:
	}

	p.proxyDict[name] = proxyPair

	if proxyPair.Ready() {
		// 在代理对的客户端和服务端都准备好时落盘
		Map2File(p.proxyDict)
	}
}

func (p *ProxyServer) match(name string, position string) (dst net.TCPAddr) {
	proxyPair, ok := p.proxyDict[name]
	if ok && proxyPair.Ready() {
		switch position {
		case Client:
			return proxyPair.Client
		case Server:
			return proxyPair.Server
		default:
		}
	}
	return
}

func (p *ProxyServer) Register(w http.ResponseWriter, r *http.Request) {
	logger.Println("Register")
	name, addr, position, err := getRegisterParams(r)
	if err != nil {
		logger.Printf("%v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// 如果已经有正在进行的连接，则拒绝注册请求
	if proxy, ok := p.proxyDict[name]; ok {
		if proxy.Ready() && proxy.done != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	p.addProxy(name, addr, position)
	w.WriteHeader(http.StatusAccepted)
	fmt.Printf("Register [%s-%s]: %s\n", name, position, addr.String())
}

func (p *ProxyServer) Query(w http.ResponseWriter, r *http.Request) {
	logger.Println("Query")
	name, position, err := getQueryParams(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	result_addr := p.match(name, position)
	json.NewEncoder(w).Encode(result_addr)
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(http.StatusOK)
}

func (p *ProxyServer) Forwarding(w http.ResponseWriter, r *http.Request) {
	logger.Println("Forwarding")
	r.ParseForm()
	name := r.Form.Get("name")
	if name == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	proxy, ok := p.proxyDict[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if !proxy.Ready() {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	proxy.done = make(chan interface{})
	proxyAddr := p.tcpListen(name)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(proxyAddr))
}

func (p *ProxyServer) StopForwarding(w http.ResponseWriter, r *http.Request) {
	logger.Println("Stop")
	r.ParseForm()
	name := r.Form.Get("name")
	proxy := p.proxyDict[name]
	close(proxy.done)
	w.WriteHeader(http.StatusOK)
}

func getRegisterParams(r *http.Request) (name string, addr net.TCPAddr, position string, err error) {
	r.ParseForm()
	name = r.Form.Get("name")
	host := r.Form.Get("host")
	port, err := strconv.Atoi(r.Form.Get("port"))
	position = r.Form.Get("position")

	addr = net.TCPAddr{
		IP:   net.ParseIP(host),
		Port: port,
	}
	return
}

func getQueryParams(r *http.Request) (name string, position string, err error) {
	r.ParseForm()
	name = r.Form.Get("name")
	position = r.Form.Get("position")
	return
}

func NewProxyServer() *ProxyServer {
	p := new(ProxyServer)
	p.proxyDict = make(map[string]*portProxy)
	File2Map(&p.proxyDict)

	router := http.NewServeMux()
	router.Handle("/register", http.HandlerFunc(p.Register))
	router.Handle("/query", http.HandlerFunc(p.Query))
	router.Handle("/forwarding", http.HandlerFunc(p.Forwarding))
	router.Handle("/stop", http.HandlerFunc(p.StopForwarding))

	p.Handler = router
	p.clientIP = localIP
	p.serverIP = localIP
	p.proxyMinPort = startPort
	p.proxyMaxPort = endPort
	return p
}

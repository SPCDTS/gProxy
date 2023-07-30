package gproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	epio "g-proxy/epio"
	"g-proxy/utils"
	"io"
	"log"
	"math/rand"
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
	proxyDict    map[string]*PortProxy
	clientIP     string
	serverIP     string
	proxyMinPort int
	proxyMaxPort int
	gpoll        *utils.GoPool
	forAccept    *epio.Reactor
	forNewFd     *epio.Reactor
	forConn      *epio.Connector
}

func (p *ProxyServer) ListenClient(maxTry int) (net.Listener, error) {
	lc := ReuseConfig()
	for i := 0; i < maxTry; i++ {
		for j := 0; j < 5; j++ {
			port := getPort(p.proxyMinPort, p.proxyMaxPort)
			address := p.serverIP + ":" + strconv.Itoa(port)
			fmt.Printf("[ListenClient] 正在进行第<%d>次尝试，使用:%s\n", j+1, address)
			ln, err := lc.Listen(context.Background(), "tcp", address) // 监听Client端口
			if err == nil {
				return ln, nil
			} else {
				fmt.Printf("[ListenClient] 无法连接:%s\n", err.Error())
			}

		}
		fmt.Println("[ListenClient] 正在退避")
		time.Sleep(time.Duration(rand.Int63n(5)) * time.Second) // 找不到就先退避
	}
	return nil, errors.New("[ListenClient] 无空闲端口，无法监听客户端连接")
}

// 连接至目标服务器
func ConnectRemote(timeout time.Duration, remoteAddress net.Addr, maxTry int) (net.Conn, error) {
	for i := 0; i < maxTry; i++ {
		d := net.Dialer{
			Timeout: timeout,
		}
		if remote_tcp, err := d.Dial("tcp", remoteAddress.String()); err == nil {
			return remote_tcp, nil
		} else {
			fmt.Printf("[ConnectRemote] 无法连接:%s\n", err.Error())
		}
	}
	return nil, errors.New("[ConnectRemote] 无法建立连接")
}

// 侦听对应代理服务的端口
func (p *ProxyServer) tcpListen(name string) string {

	proxy := p.proxyDict[name]
	ln, err := p.ListenClient(5)
	go func() {
		<-proxy.done
		ln.Close()
	}()
	if err != nil {
		fmt.Println(err)
		return ""
	}
	proxy.lcp = ln.Addr().(*net.TCPAddr).Port

	log.Printf("正在侦听: %s\n", ln.Addr().String())

	// 新建一个goroutine去不断地侦听端口，当ln被close的时候，会退出
	p.gpoll.Go(func() {
		for {
			tcp_Conn, err := ln.Accept()
			if err != nil {
				fmt.Printf("停止接收连接: %s\n", err)
				return
			}
			p.gpoll.Go(func() { p.tcpHandle(proxy.Server, tcp_Conn) }) //创建新的协程进行转发
		}
	})

	// 返回绑定的地址
	return ln.Addr().String()
}

// 处理建立的连接
func (p *ProxyServer) tcpHandle(server *net.TCPAddr, tcpConn net.Conn) {
	//fmt.Println("[tcpListen] incoming connection: ", tcpConn.RemoteAddr().String())
	remote_tcp, err := ConnectRemote(5*time.Second, server, 5)
	if err != nil {
		fmt.Printf("无法连接至目标服务器: %s\n", err)
		if remote_tcp != nil {
			remote_tcp.Close()
		}
		tcpConn.Close()
		fmt.Println("Client closed")
		return
	}

	p.gpoll.Go(func() {
		defer tcpConn.Close()
		defer remote_tcp.Close()
		io.Copy(tcpConn, remote_tcp)
	})

	p.gpoll.Go(func() {
		defer tcpConn.Close()
		defer remote_tcp.Close()
		io.Copy(remote_tcp, tcpConn)
	})
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

// 根据名称和mode返回对应的地址
func (p *ProxyServer) match(name, mode string) (dst *net.TCPAddr) {
	proxyPair, ok := p.proxyDict[name]
	if !ok {
		return
	}

	if mode == "direct" {
		dst = proxyPair.Server
	} else {
		dst.IP = net.ParseIP(p.clientIP)
		dst.Port = proxyPair.lcp
	}
	return
}

func (p *ProxyServer) Register(w http.ResponseWriter, r *http.Request) {
	logger.Println("Register")
	name, addr, err := getRegisterParams(r)
	if err != nil {
		logger.Printf("%v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// 如果已经有正在进行的连接，则拒绝注册请求
	if proxy, ok := p.proxyDict[name]; ok {
		if proxy.Running() {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	p.addProxy(name, addr)
	w.WriteHeader(http.StatusAccepted)
	fmt.Printf("Register [%s]: %s\n", name, addr.String())
}

func (p *ProxyServer) Query(w http.ResponseWriter, r *http.Request) {
	logger.Println("Query")
	name, mode, err := getQueryParams(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	result_addr := p.match(name, mode)
	json.NewEncoder(w).Encode(result_addr)
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(http.StatusOK)
}

// 开始转发时，会返回代理服务器侦听客户端的端口
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

	if proxy.Server == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if proxy.Running() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Already forwarding"))
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
	if proxy != nil && proxy.done != nil {
		close(proxy.done)
		proxy.done = nil
	}
	w.WriteHeader(http.StatusOK)
}

func getRegisterParams(r *http.Request) (name string, addr *net.TCPAddr, err error) {
	r.ParseForm()
	name = r.Form.Get("name")
	host := r.Form.Get("host")
	port, err := strconv.Atoi(r.Form.Get("port"))

	addr = &net.TCPAddr{
		IP:   net.ParseIP(host),
		Port: port,
	}
	return
}

func getQueryParams(r *http.Request) (name string, mode string, err error) {
	r.ParseForm()
	name = r.Form.Get("name")
	mode = r.Form.Get("mode")
	return
}

func NewProxyServer() *ProxyServer {
	p := new(ProxyServer)
	forAccept, err := epio.NewReactor(
		epio.EvDataArrSize(0), // default val
		epio.EvPollNum(1),
		epio.EvReadyNum(8), // only accept fd
	)
	if err != nil {
		panic(err.Error())
	}
	forNewFd, err := epio.NewReactor(
		epio.EvDataArrSize(0), // default val
		epio.EvPollNum(1),
		epio.EvReadyNum(512), // auto calc
	)
	if err != nil {
		panic(err.Error())
	}
	forConn, err := epio.NewConnector(forNewFd)
	if err != nil {
		panic(err.Error())
	}
	p.forAccept = forAccept
	p.forNewFd = forNewFd
	p.forConn = forConn

	p.gpoll = utils.NewGoPool(64, 32, 1024)
	p.proxyDict = make(map[string]*PortProxy)
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

func getPort(minP, maxP int) (port int) {
	port = minP + rand.Intn(maxP-minP+1)
	return
}
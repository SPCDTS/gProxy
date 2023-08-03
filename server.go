package gproxy

import (
	"encoding/json"
	"fmt"
	epio "g-proxy/epio"
	"g-proxy/utils"
	"log"
	"net"
	"net/http"
	"strconv"
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
	connector    *epio.Connector
	port         chan int
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
	}()
	// 返回绑定的地址
	log.Printf("正在侦听: %s\n", addr)
	return addr
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
		w.Write([]byte(localIP + ":" + strconv.Itoa(proxy.lcp)))
		return
	}
	proxy.done = make(chan struct{})
	proxyAddr := p.tcpListen(name)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(proxyAddr))
}

func (p *ProxyServer) StopForwarding(w http.ResponseWriter, r *http.Request) {
	logger.Println("Stop")
	r.ParseForm()
	name := r.Form.Get("name")
	proxy := p.proxyDict[name]
	if proxy != nil && proxy.Running() {
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
		epio.ReuseAddr(true),
	)
	if err != nil {
		panic(err.Error())
	}
	forNewFd, err := epio.NewReactor(
		epio.EvDataArrSize(0), // default val
		epio.EvPollNum(13),
		epio.EvReadyNum(512), // auto calc
		epio.TimerHeapInitSize(10000),
		epio.ReuseAddr(true),
	)
	if err != nil {
		panic(err.Error())
	}
	connector, err := epio.NewConnector(forNewFd)
	if err != nil {
		panic(err.Error())
	}
	p.forAccept = forAccept
	p.forNewFd = forNewFd
	p.connector = connector
	go func() {
		if err := p.forAccept.Run(); err != nil {
			panic(err.Error())
		}
	}()
	go func() {
		if err := p.forNewFd.Run(); err != nil {
			panic(err.Error())
		}
	}()
	p.gpoll = nil //utils.NewGoPool(64, 32, 1024)
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
	p.port = make(chan int, endPort-startPort+1)
	for i := startPort; i <= endPort; i++ {
		p.port <- i
	}
	return p
}

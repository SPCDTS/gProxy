package gproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"g-proxy/utils"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReuse(t *testing.T) {
	t.Run("先通配符", func(t *testing.T) {
		lc1 := ReuseConfig()
		ln1, err := lc1.Listen(context.Background(), "tcp", "0.0.0.0:11111")
		if err != nil {
			t.Fatalf("创建通配绑定出错: %s", err)
		} else {
			defer ln1.Close()
		}
		lc2 := ReuseConfig()
		ln2, err := lc2.Listen(context.Background(), "tcp", "172.119.1.2:11111")
		if err != nil {
			t.Logf("创建特定绑定出错: %s", err)
		} else {
			defer ln2.Close()
		}

	})

	t.Run("后通配符", func(t *testing.T) {
		lc2 := ReuseConfig()
		ln2, err := lc2.Listen(context.Background(), "tcp", "172.119.1.2:11111")
		if err != nil {
			t.Fatalf("创建特定绑定出错: %s", err)
		} else {
			defer ln2.Close()
		}

		lc1 := ReuseConfig()
		ln1, err := lc1.Listen(context.Background(), "tcp", "0.0.0.0:11111")
		if err != nil {
			t.Errorf("创建通配绑定出错: %s", err)
		} else {
			defer ln1.Close()
		}

	})
}

func TestJson(t *testing.T) {
	dic := map[string]*PortProxy{
		"test": {
			Server: &net.TCPAddr{
				IP:   net.ParseIP("11.11.11.22"),
				Port: 1002,
			},
		},
	}
	err := Map2File(dic)

	dic["gitlab"] = &PortProxy{
		Server: &net.TCPAddr{
			IP:   net.ParseIP("11.11.222.22"),
			Port: 1111,
		},
	}
	Map2File(dic)
	t.Log(err)
	dic2 := make(map[string]*PortProxy)
	err = File2Map(&dic2)
	if err != nil {
		t.Log(err)
	}
	t.Logf("%#v", dic2["test"].Server)

}

func TestRegister(t *testing.T) {
	addr1 := net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 8081,
	}

	name := "test"
	proxyServer := NewProxyServer()
	EchoServer(addr1.String())
	t.Run("注册1个地址,并查询它", func(t *testing.T) {

		request_1 := newRegisterRequest(name, addr1)
		response_1 := httptest.NewRecorder()
		proxyServer.ServeHTTP(response_1, request_1)
		assertStatus(t, response_1, http.StatusAccepted)
		assertProxyPair(t, proxyServer.proxyDict[name].Server, &addr1)

		query_request := newQueryRequest(name, "direct")
		query_response := httptest.NewRecorder()
		proxyServer.ServeHTTP(query_response, query_request)
		assertStatus(t, query_response, http.StatusOK)
		result_addr := getQueryBody(t, query_response)
		assertProxyPair(t, result_addr, &addr1)
	})

	forwardingRequest := newForwardingRequest(name)
	forwardingResponse := httptest.NewRecorder()
	proxyServer.ServeHTTP(forwardingResponse, forwardingRequest)
	proxy_addr := forwardingResponse.Body.String()
	t.Run("测试TCP转发", func(t *testing.T) {
		for i := 0; i < 1; i++ {
			ShortConnect(t, proxy_addr, 8082)
		}
		stopRequest := newStopRequest(name)
		proxyServer.ServeHTTP(httptest.NewRecorder(), stopRequest)
	})
}

func TestProxy(t *testing.T) {
	proxyServer := NewProxyServer()
	dic := map[string]*PortProxy{
		"test": {
			Server: &net.TCPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 4321,
			},
		},
	}
	proxyServer.proxyDict = dic

}

func assertProxyPair(t *testing.T, addr *net.TCPAddr, target_addr *net.TCPAddr) {
	t.Helper()
	if addr.String() != target_addr.String() {
		t.Errorf("go %v, want %v", addr, target_addr)
	}
}

func assertStatus(t *testing.T, got *httptest.ResponseRecorder, want int) {
	t.Helper()
	if got.Code != want {
		t.Errorf("did not get correct status, got %d, want %d ", got.Code, want)
	}
}

func newRegisterRequest(name string, addr net.TCPAddr) *http.Request {
	request, _ := http.NewRequest(http.MethodPost, "/register", nil)
	params := url.Values{}
	params.Set("name", name)
	params.Set("host", addr.IP.String())
	params.Set("port", strconv.Itoa(addr.Port))
	request.Form = params
	return request
}

func newQueryRequest(name string, mode string) *http.Request {
	request, _ := http.NewRequest(http.MethodPost, "/query", nil)
	params := url.Values{}
	params.Set("name", name)
	params.Set("mode", mode)
	request.Form = params
	return request
}

func newForwardingRequest(name string) *http.Request {
	request, _ := http.NewRequest(http.MethodPost, "/forwarding", nil)
	params := url.Values{}
	params.Set("name", name)
	request.Form = params
	return request
}

func newStopRequest(name string) *http.Request {
	request, _ := http.NewRequest(http.MethodPost, "/stop", nil)
	params := url.Values{}
	params.Set("name", name)
	request.Form = params
	return request
}

func getQueryBody(t *testing.T, response *httptest.ResponseRecorder) (addr *net.TCPAddr) {
	addr = new(net.TCPAddr)
	err := json.NewDecoder(response.Body).Decode(addr)
	if err != nil {
		t.Fatalf("Unable to parse response from server '%s' into address, %v", response.Body, err)
	}
	return
}

func EchoServer(server_addr string) {
	server_ln, err := net.Listen("tcp", server_addr)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			server_conn, _ := server_ln.Accept()
			buf := make([]byte, 4096)
			go func() {
				fmt.Println("[Echo Server] incoming connection: ", server_conn.RemoteAddr().String())
				defer server_conn.Close()
				for {
					nr, err := server_conn.Read(buf)
					if err != nil {
						return
					}
					server_conn.Write(buf[:nr])
				}
			}()
		}
	}()
}
func ShortConnect(t *testing.T, proxy_addr string, client_port int) {
	d := net.Dialer{
		Timeout: 5 * time.Second,
		LocalAddr: &net.TCPAddr{
			Port: client_port,
		},
	}
	var proxy_cnn net.Conn
	var err error
	proxy_cnn, err = d.Dial("tcp", proxy_addr) // 连接至代理服务器
	assert.Nil(t, err)
	readBuf := make([]byte, 1024)
	writeS := utils.RandString(100)
	writeBytes := []byte(writeS)
	total := 0
	for i := 0; i < 100; i++ {
		_, err = proxy_cnn.Write(writeBytes)
		assert.Nil(t, err)
		nr, err := proxy_cnn.Read(readBuf)
		assert.Nil(t, err)
		assert.Equal(t, writeS, string(readBuf[:nr]))
		total += nr
	}
	fmt.Printf("[ShortConnect] total RW bytes: %d\n", total)
	proxy_cnn.Close()
}

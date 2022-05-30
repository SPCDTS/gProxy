package proxy_server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"
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
	dic := map[string]*portProxy{
		"test": {
			Client: net.TCPAddr{
				IP:   net.ParseIP("11.11.11.11"),
				Port: 1000,
			},
			Server: net.TCPAddr{
				IP:   net.ParseIP("11.11.11.22"),
				Port: 1002,
			},
		},
	}
	err := Map2File(dic)

	dic["gitlab"] = &portProxy{
		Client: net.TCPAddr{
			IP:   net.ParseIP("172.19.243.18"),
			Port: 80,
		},
		Server: net.TCPAddr{
			IP:   net.ParseIP("11.11.222.22"),
			Port: 1111,
		},
	}
	Map2File(dic)
	t.Log(err)
	dic2 := make(map[string]*portProxy)
	err = File2Map(&dic2)
	if err != nil {
		t.Log(err)
	}
	t.Logf("%#v", dic2["test"].Client)

}

func TestRegister(t *testing.T) {
	addr1 := net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 8081,
	}
	addr2 := net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 8082,
	}
	name := "test"
	proxyServer := NewProxyServer()

	t.Run("注册两个地址,并查询它", func(t *testing.T) {

		request_1 := newRegisterRequest(name, addr1, Client)
		response_1 := httptest.NewRecorder()
		proxyServer.ServeHTTP(response_1, request_1)
		assertStatus(t, response_1, http.StatusAccepted)
		assertProxyPair(t, proxyServer.proxyDict[name].Client, addr1)

		request_2 := newRegisterRequest(name, addr2, Server)
		response_2 := httptest.NewRecorder()
		proxyServer.ServeHTTP(response_2, request_2)
		assertProxyPair(t, proxyServer.proxyDict[name].Server, addr2)

		query_request := newQueryRequest(name, Server)
		query_response := httptest.NewRecorder()
		proxyServer.ServeHTTP(query_response, query_request)
		assertStatus(t, query_response, http.StatusOK)
		result_addr := getQueryBody(t, query_response)
		assertProxyPair(t, *result_addr, addr2)
	})
	forwardingRequest := newForwardingRequest(name)
	proxyServer.ServeHTTP(httptest.NewRecorder(), forwardingRequest)

	t.Run("测试TCP转发", func(t *testing.T) {
		go proxyServer.tcpListen(name)
		client_address := proxyServer.cAddr
		server_address := addr2
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			d := net.Dialer{
				Timeout: 5 * time.Second,
				LocalAddr: &net.TCPAddr{
					Port: 8001,
				},
			}

			var client_cnn net.Conn

			for {
				var err error
				client_cnn, err = d.Dial("tcp", client_address.String()) // 尝试从8001端口连接客户端端口
				if err == nil {
					break
				}
			}

			var i int32
			for i = 0; i < 5; i++ {
				bytesBuffer := bytes.NewBuffer([]byte{})
				time.Sleep(1 * time.Second)
				binary.Write(bytesBuffer, binary.BigEndian, i)
				n, _ := client_cnn.Write(bytesBuffer.Bytes())

				fmt.Printf("client: %d, %d bytes\n", i, n)
			}
			client_cnn.Close()
			fmt.Println("client: close client->proxy connection")
		}()

		go func() {
			defer wg.Done()
			server_ln, _ := net.Listen("tcp", server_address.String())
			server_conn, _ := server_ln.Accept()
			fmt.Println(server_conn.RemoteAddr())
			buf := make([]byte, 4096)
			for {
				n, err := server_conn.Read(buf)
				if n == 0 || err != nil {
					server_conn.Close()
					break
				}
				fmt.Printf("server: %d, %d bytes\n", bytes2Int(buf[0:n]), n)
			}
		}()
		wg.Wait()
		stopRequest := newStopRequest(name)
		proxyServer.ServeHTTP(httptest.NewRecorder(), stopRequest)
	})
}

func assertProxyPair(t *testing.T, addr net.TCPAddr, target_addr net.TCPAddr) {
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

func newRegisterRequest(name string, addr net.TCPAddr, position string) *http.Request {
	request, _ := http.NewRequest(http.MethodPost, "/register", nil)
	params := url.Values{}
	params.Set("name", name)
	params.Set("host", addr.IP.String())
	params.Set("port", strconv.Itoa(addr.Port))
	params.Set("position", position)
	request.Form = params
	return request
}

func newQueryRequest(name string, position string) *http.Request {
	request, _ := http.NewRequest(http.MethodPost, "/query", nil)
	params := url.Values{}
	params.Set("name", name)
	params.Set("position", position)
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

func bytes2Int(buffer []byte) int {
	var x int32
	binary.Read(bytes.NewBuffer(buffer), binary.BigEndian, &x)
	return int(x)
}

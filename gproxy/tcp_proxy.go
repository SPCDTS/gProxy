package gproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type PortProxy struct {
	Server *net.TCPAddr
	lcp    int // listen client port, proxy server在这个端口侦听client的连接
	done   chan interface{}
}

func NewPortProxy(server *net.TCPAddr) *PortProxy {
	return &PortProxy{
		Server: server,
		done:   make(chan interface{}),
	}
}

// 通过判断done channel是否打开来确定是否正在进行转发
func (p PortProxy) Running() bool {
	if p.done != nil {
		select {
		case _, open := <-p.done:
			if open {
				return true
			}
		default:
		}
	}
	return false
}

func getPort(minP, maxP int) (port int) {
	port = minP + rand.Intn(maxP-minP+1)
	return
}

// 连接至目标服务器
func ConnectRemote(timeout time.Duration, localIP string, remoteAddress net.Addr, minP, maxP int, maxTry int) (net.Conn, error) {
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

func ListenClient(ip string, minP, maxP int, maxTry int) (net.Listener, error) {
	lc := ReuseConfig()
	for i := 0; i < maxTry; i++ {
		for j := 0; j < 5; j++ {
			port := getPort(minP, maxP)
			address := ip + ":" + strconv.Itoa(port)
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

func ReuseConfig() net.ListenConfig {
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

const dataFile = "/app/proxyEntry.json"

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

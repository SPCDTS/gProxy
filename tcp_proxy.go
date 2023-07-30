package gproxy

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"syscall"

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

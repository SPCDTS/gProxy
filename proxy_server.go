package gproxy

import (
	"fmt"
	"g-proxy/io"
	"syscall"
)

type ProxyC struct {
	io.Event
	c     *io.Connector
	buddy *ProxyS
}

func (p *ProxyC) OnOpen(fd int, now int64) bool {
	if err := p.GetReactor().AddEvHandler(p, fd, io.EvIn); err != nil {
		return false
	}

	p.buddy.buddy = p
	if err := p.c.Connect(p.buddy.addr, p.buddy, 500); err != nil {
		return false
	}

	return true
}

func (p *ProxyC) OnRead(fd int, evPollSharedBuff []byte, now int64) bool {
	buf := make([]byte, 0, 4096)
	for {
		n, err := io.Read(fd, buf)
		if err != nil {
			if err == syscall.EAGAIN { // epoll ET mode
				break
			}
			fmt.Println("read: ", err.Error())
			return false
		}
		if n > 0 { // n > 0
			io.Write(p.buddy.GetFd(), buf[0:n])
		} else { // n == 0 connection closed,  will not < 0
			if n == 0 {
				fmt.Println("peer closed. ", n)
			}
			return false
		}
	}
	return true
}

type ProxyS struct {
	io.Event
	buddy *ProxyC
	addr  string
}

func (p *ProxyS) OnOpen(fd int, now int64) bool {
	if err := p.GetReactor().AddEvHandler(p, fd, io.EvIn); err != nil {
		return false
	}
	return true
}
func (p *ProxyS) OnRead(fd int, evPollSharedBuff []byte, now int64) bool {
	buf := make([]byte, 0, 4096)
	for {
		n, err := io.Read(fd, buf)
		if err != nil {
			if err == syscall.EAGAIN { // epoll ET mode
				break
			}
			fmt.Println("read: ", err.Error())
			return false
		}
		if n > 0 { // n > 0
			io.Write(p.buddy.GetFd(), buf[0:n])
		} else { // n == 0 connection closed,  will not < 0
			if n == 0 {
				fmt.Println("peer closed. ", n)
			}
			return false
		}
	}
	return true
}

package gproxy

import (
	"fmt"
	epio "g-proxy/epio"
	"sync"
	"syscall"
)

type ProxyC struct {
	epio.Event
	c         *epio.Connector
	buddy     *ProxyS
	closeOnce *sync.Once
}

func NewProxyC(c *epio.Connector, buddyAddr string) *ProxyC {
	pc := &ProxyC{c: c, closeOnce: &sync.Once{}}
	ps := &ProxyS{addr: buddyAddr, ready: make(chan struct{}), closeOnce: &sync.Once{}}
	pc.buddy = ps
	ps.buddy = pc
	pc.SetFd(-1)
	ps.SetFd(-1)
	if err := c.Connect(buddyAddr, ps, 30000); err != nil {
		panic(err.Error())
	}
	return pc
}

func (p *ProxyC) OnOpen(fd int, now int64) bool {
	if err := p.GetReactor().AddEvHandler(p, fd, epio.EvIn); err != nil {
		return false
	}
	p.SetFd(fd)
	return true
}

func (p *ProxyC) OnRead(fd int, evPollSharedBuff []byte, now int64) bool {
	buf := evPollSharedBuff
	if p.buddy.GetFd() == -1 {
		return true
	}

	for {
		n, err := epio.Read(fd, buf)
		if err != nil {
			if err == syscall.EAGAIN { // epoll ET mode
				break
			}
			fmt.Println("read: ", err.Error())
			return false
		}
		if n > 0 { // n > 0
			epio.Write(p.buddy.GetFd(), buf[0:n])
		} else { // n == 0 connection closed,  will not < 0
			return false
		}
	}
	return true
}

func (p *ProxyC) OnClose(fd int) {
	p.GetReactor().RemoveEvHandler(p, fd)
	epio.Close(fd)
	p.buddy.GetReactor().RemoveEvHandler(p.buddy, p.buddy.GetFd())
	epio.Close(p.buddy.GetFd())
}

type ProxyS struct {
	epio.Event
	buddy     *ProxyC
	addr      string
	ready     chan struct{}
	closeOnce *sync.Once
}

func (p *ProxyS) OnOpen(fd int, now int64) bool {
	if err := p.GetReactor().AddEvHandler(p, fd, epio.EvIn); err != nil {
		return false
	}
	p.SetFd(fd)
	return true
}
func (p *ProxyS) OnRead(fd int, evPollSharedBuff []byte, now int64) bool {
	buf := evPollSharedBuff
	for {
		n, err := epio.Read(fd, buf)
		if err != nil {
			if err == syscall.EAGAIN { // epoll ET mode
				break
			}
			fmt.Println("read: ", err.Error())
			return false
		}
		if n > 0 { // n > 0
			epio.Write(p.buddy.GetFd(), buf[0:n])
		} else { // n == 0 connection closed,  will not < 0
			return false
		}
	}
	return true
}

func (p *ProxyS) OnClose(fd int) {
	p.GetReactor().RemoveEvHandler(p, fd)
	epio.Close(fd)
	p.buddy.GetReactor().RemoveEvHandler(p.buddy, p.buddy.GetFd())
	epio.Close(p.buddy.GetFd())
}
func (p *ProxyS) OnConnectFail(err error) {
	fmt.Println("ProxyS: " + err.Error())
	p.buddy.c.Connect(p.addr, p, 3000)
}

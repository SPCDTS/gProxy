package io

import "syscall"

const (
	// 边缘触发
	EPOLLET = 1 << 31

	EvIn uint32 = syscall.EPOLLIN | syscall.EPOLLRDHUP

	EvOut uint32 = syscall.EPOLLOUT | syscall.EPOLLRDHUP

	EvInET uint32 = EvIn | EPOLLET

	EvOutET uint32 = EvOut | EPOLLET

	EvEventfd uint32 = syscall.EPOLLIN | syscall.EPOLLRDHUP

	EvAccept uint32 = syscall.EPOLLIN | syscall.EPOLLRDHUP

	EvConnect uint32 = syscall.EPOLLIN | syscall.EPOLLOUT | syscall.EPOLLRDHUP
)

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// EvHandler is the event handling interface of the Reactor core
//
// The same EvHandler is repeatedly registered with the Reactor
type EvHandler interface {
	setEvPoll(ep *evPoll)
	getEvPoll() *evPoll

	setReactor(r *Reactor)
	GetReactor() *Reactor

	// Call by acceptor on `accept` a new fd or connector on `connect` successful
	// The parameter 'millisecond' represents the time of batch retrieval of epoll events, not the current
	// precise time. Use it with caution (as it can reduce the frequency of obtaining the current
	// time to some extent).
	//
	// Call OnClose() when return false
	OnOpen(fd int, millisecond int64) bool

	// EvPoll catch readable i/o event
	// The parameter 'millisecond' represents the time of batch retrieval of epoll events, not the current
	// precise time. Use it with caution (as it can reduce the frequency of obtaining the current
	// time to some extent).
	//
	// Call OnClose() when return false
	OnRead(fd int, evPollSharedBuff []byte, millisecond int64) bool

	// EvPoll catch writeable i/o event
	// The parameter 'millisecond' represents the time of batch retrieval of epoll events, not the current
	// precise time. Use it with caution (as it can reduce the frequency of obtaining the current
	// time to some extent).
	//
	// Call OnClose() when return false
	OnWrite(fd int, millisecond int64) bool

	// EvPoll catch connect result
	// Only be asynchronously called after connector.Connect() returns nil
	//
	// Will not call OnClose() after OnConnectFail() (So you don't need to manually release the fd)
	// The param err Refer to ev_handler.go: ErrConnect*
	OnConnectFail(err error)

	// EvPoll catch timeout event
	// The parameter 'millisecond' represents the time of batch retrieval of epoll events, not the current
	// precise time. Use it with caution (as it can reduce the frequency of obtaining the current
	// time to some extent).
	// Note: Don't call Reactor.SchedueTimer() or Reactor.CancelTimer() in OnTimeout, it will deadlock
	//
	// Remove timer when return false
	OnTimeout(millisecond int64) bool

	// Call by reactor(OnOpen must have been called before calling OnClose.)
	//
	// You need to manually release the fd resource call fd.Close()
	// You'd better only call fd.Close() here.
	OnClose(fd int)
	SetFd(fd int)
	GetFd() int
}

// Event is the base class of event handling objects
type Event struct {
	noCopy

	_r *Reactor // atomic.Pointer[Reactor]
	// 这里不需要保护, 在set之前Get是没有任何调用机会的(除非框架之外乱搞)

	_ep *evPoll // atomic.Pointer[evPoll]
	// 这里不需要保护, 在set之前Get是没有任何调用机会的(除非框架之外乱搞)
	fd int
}

func (e *Event) SetFd(fd int) {
	e.fd = fd
}

func (e *Event) GetFd() int {
	return e.fd
}

func (e *Event) setEvPoll(ep *evPoll) {
	e._ep = ep
}
func (e *Event) getEvPoll() *evPoll {
	return e._ep
}
func (e *Event) setReactor(r *Reactor) {
	e._r = r
}

// GetReactor can retrieve the current event object bound to which Reactor
func (e *Event) GetReactor() *Reactor {
	return e._r
}

// OnOpen please make sure you want to reimplement it.
func (*Event) OnOpen(fd int, millisecond int64) bool {
	panic("Event OnOpen")
}

// OnRead please make sure you want to reimplement it.
func (*Event) OnRead(fd int, evPollSharedBuff []byte, millisecond int64) bool {
	panic("Event OnRead")
}

// OnWrite please make sure you want to reimplement it.
func (*Event) OnWrite(fd int, millisecond int64) bool {
	panic("Event OnWrite")
}

// OnConnectFail please make sure you want to reimplement it.
func (*Event) OnConnectFail(err error) {
	panic("Event OnConnectFail")
}

// OnTimeout please make sure you want to reimplement it.
func (*Event) OnTimeout(millisecond int64) bool {
	panic("Event OnTimeout")
}

// OnClose please make sure you want to reimplement it.
func (*Event) OnClose(fd int) {
	panic("Event OnClose")
}

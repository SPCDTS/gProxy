package epio

import (
	"errors"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Notifier provides a mechanism for communication with evpool, only offering a wakeup mechanism
// to wake epoll up from waiting.
type Notifier interface {
	// Tread-safe
	Notify()

	// Tread-safe close the notifier
	Close()
}

type notify struct {
	Event

	efd        int
	notifyOnce atomic.Int32 // used to avoid duplicate call evHandler
	closeOnce  atomic.Int32 // used to avoid duplicate close
}

var (
	notifyV           int64 = 1
	notifyWriteV            = (*(*[8]byte)(unsafe.Pointer(&notifyV)))[:]
	notifyCloseV      int64 = 31415927
	notifyCloseWriteV       = (*(*[8]byte)(unsafe.Pointer(&notifyCloseV)))[:]
)

func newNotify(ep *evPoll) (Notifier, error) {
	// since Linux 2.6.27
	fd, err := unix.Eventfd(0, unix.EFD_NONBLOCK|unix.EFD_CLOEXEC)
	if err != nil {
		return nil, errors.New("eventfd: " + err.Error())
	}
	nt := &notify{
		efd: fd,
	}
	if err = ep.add(nt.efd, EvEventfd, nt); err != nil {
		syscall.Close(fd)
		return nil, errors.New("Notify add to evpoll fail! " + err.Error())
	}
	return nt, nil
}

// Notify send sends a notification to evpool
func (nt *notify) Notify() {
	if !nt.notifyOnce.CompareAndSwap(0, 1) {
		return
	}
	for {
		n, err := syscall.Write(nt.efd, notifyWriteV) // man 2 eventfd
		if n == 8 {
			return
		} else if err != nil {
			if err == syscall.EINTR {
				continue
			}
			if err == syscall.EAGAIN {
				return
			}
		}
		break // TODO add evOptions.debug? panic("Notify: write eventfd failed!")
	}
}

func (nt *notify) Close() {
	if !nt.closeOnce.CompareAndSwap(0, 1) {
		return
	}
	for {
		n, err := syscall.Write(nt.efd, notifyCloseWriteV) // man 2 eventfd
		if n == 8 {
			return
		}
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			if err == syscall.EAGAIN {
				return
			}
		}
		break // TODO add evOptions.debug? panic("Notify: write eventfd failed!")
	}
}

// Prohibit external calls
func (nt *notify) OnRead(fd int, evPollSharedBuff []byte, now int64) bool {
	if fd != nt.efd { // 防止外部调用!
		panic("Prohibit external calls")
	}
	var tmp [8]byte
	for {
		n, err := syscall.Read(nt.efd, tmp[:])
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			if err == syscall.EAGAIN {
				nt.notifyOnce.Store(0)
				return true
			}
			return false // TODO add evOptions.debug? panic("Notify: read eventfd failed!")
		}
		if n == 8 {
			if *(*int64)(unsafe.Pointer(&tmp[0])) == notifyV {
				nt.notifyOnce.Store(0)
				return true
			}
			if *(*int64)(unsafe.Pointer(&tmp[0])) == notifyCloseV {
				nt.closeOnce.Store(0) // optional
				return false          // goto OnClose
			}
			return false // TODO add evOptions.debug? panic("Notify: read unknown value!")
		}
	}
}

func (nt *notify) OnClose(fd int) {
	syscall.Close(fd)
	nt.efd = -1
}

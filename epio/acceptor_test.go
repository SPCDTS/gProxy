package epio

import (
	"fmt"
	"g-proxy/utils"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	buffPool *sync.Pool
)

type Http struct {
	Event
}

func (h *Http) OnOpen(fd int, now int64) bool {
	// fd.SetNoDelay(1) // New socket has been set to non-blocking
	if err := h.GetReactor().AddEvHandler(h, fd, EvIn); err != nil {
		return false
	}
	return true
}
func (h *Http) OnRead(fd int, evPollSharedBuff []byte, now int64) bool {
	buf := buffPool.Get().([]byte) // just read
	defer buffPool.Put(buf)

	readN := 0
	for {
		if readN >= cap(buf) { // alloc new buff to read
			break
		}
		n, err := Read(fd, buf[readN:])
		if err != nil {
			if err == syscall.EAGAIN { // epoll ET mode
				break
			}
			return false
		}
		if n > 0 { // n > 0
			readN += n
		} else { // n == 0 connection closed,  will not < 0
			return false
		}
	}
	Write(fd, buf[:readN]) // Connection: close
	return true            // will goto OnClose
}
func (h *Http) OnClose(fd int) {
	Close(fd)
}

type Https struct {
	Http
}

func TestAcceptor(t *testing.T) {
	StartServer(t)
	ShortConnect(t, "127.0.0.1:3141")
}

func StartServer(t *testing.T) {
	fmt.Println("hello boy")
	buffPool = &sync.Pool{
		New: func() any {
			return make([]byte, 4096)
		},
	}
	forAccept, err := NewReactor(
		EvDataArrSize(0), // default val
		EvPollNum(1),
		EvReadyNum(8), // only accept fd
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	forNewFd, err := NewReactor(
		EvDataArrSize(0), // default val
		EvPollNum(1),
		EvReadyNum(512), // auto calc
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	if err != nil {
		t.Fatal(err.Error())
	}
	go func() {
		if err = forAccept.Run(); err != nil {
			panic(err.Error())
		}
	}()
	go func() {
		if err = forNewFd.Run(); err != nil {
			panic(err.Error())
		}
	}()
	//= http
	_, err = NewAcceptor(forAccept, forNewFd, func() EvHandler { return new(Http) },
		":3141",
		ListenBacklog(256),
		SockRcvBufSize(8*1024), // 短链接, 不需要很大的缓冲区
	)
}

func ShortConnect(t *testing.T, proxy_addr string) {
	d := net.Dialer{
		Timeout: 5 * time.Second,
	}
	var proxy_cnn net.Conn
	var err error
	proxy_cnn, err = d.Dial("tcp", proxy_addr) // 连接至代理服务器
	assert.Nil(t, err)
	readBuf := make([]byte, 1024)
	writeS := utils.RandString(100)
	writeBytes := []byte(writeS)
	total := 0
	cnt := 0
	for i := 0; i < 100; i++ {
		_, err = proxy_cnn.Write(writeBytes)
		assert.Nil(t, err)
		nr, err := proxy_cnn.Read(readBuf)
		assert.Nil(t, err)
		if writeS != string(readBuf[:nr]) {
			cnt += 1
		}
	}
	fmt.Printf("[ShortConnect] total RW bytes: %d, %d/100\n", total, cnt)
	proxy_cnn.Close()
}

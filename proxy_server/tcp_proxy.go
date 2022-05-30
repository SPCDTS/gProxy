package proxy_server

import (
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type portProxy struct {
	Client net.TCPAddr
	Server net.TCPAddr
	done   chan interface{}
}

func addrReady(addr net.TCPAddr) bool {
	return addr.IP != nil && addr.Port != 0
}

// 两组地址都设置
func (p portProxy) Ready() bool {
	return addrReady(p.Client) && addrReady(p.Server)
}

const startPort = 33333
const endPort = 33444

func getPort() (port int) {
	port = startPort + rand.Intn(endPort-startPort+1)
	return
}

func GetCustomConn(timeout time.Duration, localIP string, remoteAddress net.Addr, maxTry int) (net.Conn, error) {
	for i := 0; i < maxTry; i++ {
		for j := 0; j < 5; j++ {
			localAddr := net.TCPAddr{
				IP:   net.ParseIP(localIP),
				Port: getPort(),
			}
			d := net.Dialer{
				Timeout:   timeout,
				LocalAddr: &localAddr,
			}
			log.Printf("正在进行第<%d>次尝试，使用:%s", j+1, &localAddr)
			if remote_tcp, err := d.Dial("tcp", remoteAddress.String()); err == nil {
				return remote_tcp, nil
			} else {
				log.Printf("无法连接:%s\n", err.Error())
			}
		}
		log.Println("正在退避")
		time.Sleep(time.Duration(rand.Int63n(5)) * time.Second) // 找不到就先退避
	}

	return nil, errors.New("无空闲端口，无法建立连接")
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

const dataFile = "../proxyEntry.json"

func Map2File(dic map[string]*portProxy) (err error) {
	fPtr, err := os.Create(dataFile)

	if err != nil {
		return
	}

	log.Printf("正在写入: %s\n", dataFile)
	defer fPtr.Close()
	encoder := json.NewEncoder(fPtr)
	err = encoder.Encode(dic)
	return
}

func File2Map(dic *map[string]*portProxy) (err error) {
	fPtr, err := os.Open(dataFile)
	if err != nil {
		return
	}
	defer fPtr.Close()
	decoder := json.NewDecoder(fPtr)
	decoder.Decode(&dic)
	return
}

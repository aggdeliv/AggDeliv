package mpquic_trans

import (
	"net"
	"sync"
	//	"strings"
)

type connection interface {
	Write([]byte) error
	Read([]byte) (int, net.Addr, error)
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetCurrentRemoteAddr(net.Addr)
}

type conn struct {
	mutex sync.RWMutex

	pconn       net.PacketConn
	currentAddr net.Addr
	cnt1        int64
	cnt2        int64
}

var _ connection = &conn{}

func (c *conn) Write(p []byte) error {
	/*tmp := c.pconn.LocalAddr().String()
	if find := strings.Contains(tmp, "192.168.1.203"); find {
		c.cnt1 += int64(len(p))
	}
	if find := strings.Contains(tmp, "192.168.1.205"); find {
		c.cnt2 += int64(len(p))
	}
	println("cnt1:", c.cnt1, ", cnt2:", c.cnt2)*/
	//println("localAddr:", c.pconn.LocalAddr().String())
	_, err := c.pconn.WriteTo(p, c.currentAddr)
	return err
}

func (c *conn) Read(p []byte) (int, net.Addr, error) {
	return c.pconn.ReadFrom(p)
}

func (c *conn) SetCurrentRemoteAddr(addr net.Addr) {
	c.mutex.Lock()
	c.currentAddr = addr
	c.mutex.Unlock()
}

func (c *conn) LocalAddr() net.Addr {
	return c.pconn.LocalAddr()
}

func (c *conn) RemoteAddr() net.Addr {
	c.mutex.RLock()
	addr := c.currentAddr
	c.mutex.RUnlock()
	return addr
}

func (c *conn) Close() error {
	return c.pconn.Close()
}

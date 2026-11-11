package lan

import (
	"bufio"
	"net"
	"sync"
	"time"

	"xin.dart.zchat/internal/proto"
)

// peerConn is one TCP connection and its writer queue. Frames never block
// the UI: a slow peer fills the queue, and the connection closes rather
// than holding the sender.
type peerConn struct {
	node *Node
	id   string
	conn net.Conn
	br   *bufio.Reader
	out  chan proto.Frame
	done chan struct{}
	once sync.Once
}

func newPeerConn(n *Node, conn net.Conn, br *bufio.Reader, id string) *peerConn {
	return &peerConn{
		node: n,
		id:   id,
		conn: conn,
		br:   br,
		out:  make(chan proto.Frame, 64),
		done: make(chan struct{}),
	}
}

func (c *peerConn) start() {
	go c.readLoop()
	go c.writeLoop()
}

func (c *peerConn) send(f proto.Frame) bool {
	select {
	case <-c.done:
		return false
	default:
	}
	select {
	case c.out <- f:
		return true
	case <-c.done:
		return false
	default:
		c.close()
		return false
	}
}

func (c *peerConn) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

func (c *peerConn) readLoop() {
	defer c.node.removeConn(c.id, c)
	for {
		f, err := proto.ReadFrame(c.br)
		if err != nil {
			c.close()
			return
		}
		c.node.handleFrame(c.id, f)
	}
}

func (c *peerConn) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case f := <-c.out:
			_ = c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if err := proto.WriteFrame(c.conn, f); err != nil {
				c.close()
				return
			}
		}
	}
}

// The lowest level of XMPP protocol, where TLS is applied after the
// initial handshake.

package xmpp

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

// If enabled, print all sent and received XML.
var Debug = false

var l1interval = time.Second

type layer1 struct {
	sock      net.Conn
	recvSocks chan<- net.Conn
	sendSocks chan net.Conn
}

func (cl *Client) startLayer1(sock net.Conn, recvWriter io.WriteCloser,
	sendReader io.ReadCloser, status <-chan Status) *layer1 {
	l1 := layer1{sock: sock}
	recvSocks := make(chan net.Conn)
	l1.recvSocks = recvSocks
	sendSocks := make(chan net.Conn, 1)
	l1.sendSocks = sendSocks
	go cl.recvTransport(recvSocks, recvWriter, status)
	go cl.sendTransport(sendSocks, sendReader)
	recvSocks <- sock
	sendSocks <- sock
	return &l1
}

func (l1 *layer1) startTls(conf *tls.Config) {
	sendSockToSender := func(sock net.Conn) {
		for {
			select {
			case <-l1.sendSocks:
			case l1.sendSocks <- sock:
				return
			}
		}
	}

	sendSockToSender(nil)
	l1.recvSocks <- nil
	l1.sock = tls.Client(l1.sock, conf)
	sendSockToSender(l1.sock)
	l1.recvSocks <- l1.sock
}

func (cl *Client) recvTransport(socks <-chan net.Conn, w io.WriteCloser,
	status <-chan Status) {

	defer w.Close()
	var sock net.Conn
	p := make([]byte, 1024)
	for {
		select {
		case stat := <-status:
			if stat.Fatal() {
				return
			}

		case sock = <-socks:
		default:
		}

		if sock == nil {
			time.Sleep(l1interval)
		} else {
			sock.SetReadDeadline(time.Now().Add(l1interval))
			nr, err := sock.Read(p)
			if nr == 0 {
				if errno, ok := err.(*net.OpError); ok {
					if errno.Timeout() {
						continue
					}
				}
				cl.setError(fmt.Errorf("recv: %v", err))
				return
			}
			if Debug {
				log.Printf("recv: %s", p[:nr])
			}
			nw, err := w.Write(p[:nr])
			if nw < nr {
				cl.setError(fmt.Errorf("recv: %v", err))
				return
			}
		}
	}
}

func (cl *Client) sendTransport(socks <-chan net.Conn, r io.Reader) {
	var sock net.Conn
	p := make([]byte, 1024)
	for {
		nr, err := r.Read(p)
		if nr == 0 {
			cl.setError(fmt.Errorf("send: %v", err))
			break
		}
		if nr > 0 && Debug {
			log.Printf("send: %s", p[:nr])
		}
		for nr > 0 {
			select {
			case sock = <-socks:
				if sock != nil {
					defer sock.Close()
				}
			default:
			}

			if sock == nil {
				time.Sleep(l1interval)
			} else {
				nw, err := sock.Write(p[:nr])
				nr -= nw
				if nr != 0 {
					cl.setError(fmt.Errorf("send: %v", err))
					break
				}
			}
		}
	}
}

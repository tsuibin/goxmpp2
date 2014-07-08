// This layer of the XMPP protocol reads XMLish structures and
// responds to them. It negotiates TLS and authentication.

package xmpp

import (
	"encoding/xml"
	"fmt"
	"log"
)

// Callback to handle a stanza with a particular id.
type callback struct {
	id string
	f  func(Stanza)
}

// Receive XMPP stanzas from the client and send them on to the
// remote. Don't allow the client to send us any stanzas until
// negotiation has completed.  This loop is paused until resource
// binding is complete. Otherwise the app might inject something
// inappropriate into our negotiations with the server. The control
// channel controls this loop's activity.
func sendStream(sendXml chan<- interface{}, recvXmpp <-chan Stanza,
	status <-chan Status) {
	defer close(sendXml)

	var input <-chan Stanza
	for {
		select {
		case stat, ok := <-status:
			if !ok {
				return
			}
			switch stat {
			default:
				input = nil
			case StatusRunning:
				input = recvXmpp
			}
		case x, ok := <-input:
			if !ok {
				return
			}
			if x == nil {
				if Debug {
					log.Println("Won't send nil stanza")
				}
				continue
			}
			sendXml <- x
		}
	}
}

// Receive XMLish structures, handle all the stream-related ones, and
// send XMPP stanzas on to the client once the connection is running.
func (cl *Client) recvStream(recvXml <-chan interface{}, sendXmpp chan<- Stanza,
	status <-chan Status) {
	defer close(sendXmpp)
	defer cl.statmgr.close()

	handlers := make(map[string]func(Stanza))
	doSend := false
	for {
		select {
		case stat := <-status:
			switch stat {
			default:
				doSend = false
			case StatusRunning:
				doSend = true
			}
		case h := <-cl.handlers:
			handlers[h.id] = h.f
		case x, ok := <-recvXml:
			if !ok {
				return
			}
			switch obj := x.(type) {
			case *stream:
				// Do nothing.
			case *streamError:
				cl.setError(fmt.Errorf("%#v", obj))
				return
			case *Features:
				cl.handleFeatures(obj)
			case *starttls:
				cl.handleTls(obj)
			case *auth:
				cl.handleSasl(obj)
			case Stanza:
				id := obj.GetHeader().Id
				if handlers[id] != nil {
					f := handlers[id]
					delete(handlers, id)
					f(obj)
				}
				if doSend {
					sendXmpp <- obj
				}
			default:
				if Debug {
					log.Printf("Unrecognized input: %T %#v",
						x, x)
				}
			}
		}
	}
}

func (cl *Client) handleFeatures(fe *Features) {
	cl.Features = fe
	if fe.Starttls != nil {
		start := &starttls{XMLName: xml.Name{Space: NsTLS,
			Local: "starttls"}}
		cl.sendRaw <- start
		return
	}

	if len(fe.Mechanisms.Mechanism) > 0 {
		cl.chooseSasl(fe)
		return
	}

	if fe.Bind != nil {
		cl.bind()
		return
	}
}

func (cl *Client) handleTls(t *starttls) {
	cl.layer1.startTls(&cl.tlsConfig)

	cl.setStatus(StatusConnectedTls)

	// Now re-send the initial handshake message to start the new
	// session.
	cl.sendRaw <- &stream{To: cl.Jid.Domain(), Version: XMPPVersion}
}

// Send a request to bind a resource. RFC 3920, section 7.
func (cl *Client) bind() {
	res := cl.Jid.Resource()
	bindReq := &bindIq{}
	if res != "" {
		bindReq.Resource = &res
	}
	msg := &Iq{Header: Header{Type: "set", Id: NextId(),
		Nested: []interface{}{bindReq}}}
	f := func(st Stanza) {
		iq, ok := st.(*Iq)
		if !ok {
			cl.setError(fmt.Errorf("non-iq response to bind %#v",
				st))
			return
		}
		if iq.Type == "error" {
			cl.setError(fmt.Errorf("Resource binding failed"))
			return
		}
		var bindRepl *bindIq
		for _, ele := range iq.Nested {
			if b, ok := ele.(*bindIq); ok {
				bindRepl = b
				break
			}
		}
		if bindRepl == nil {
			cl.setError(fmt.Errorf("Bad bind reply: %#v", iq))
			return
		}
		jid := bindRepl.Jid
		if jid == nil || *jid == "" {
			cl.setError(fmt.Errorf("empty resource in bind %#v",
				iq))
			return
		}
		cl.Jid = JID(*jid)
		cl.setStatus(StatusBound)
	}
	cl.SetCallback(msg.Id, f)
	cl.sendRaw <- msg
}

// Register a callback to handle the next XMPP stanza (iq, message, or
// presence) with a given id. The provided function will not be called
// more than once. If it returns false, the stanza will not be made
// available on the normal Client.Recv channel. The callback must not
// read from that channel, as deliveries on it cannot proceed until
// the handler returns true or false.
func (cl *Client) SetCallback(id string, f func(Stanza)) {
	h := &callback{id: id, f: f}
	cl.handlers <- h
}

// Track the current status of the connection to the server.

package xmpp

import (
	"fmt"
)

// Status of the connection.
type Status int

const (
	statusUnconnected = iota
	statusConnected
	statusConnectedTls
	statusAuthenticated
	statusBound
	statusRunning
	statusShutdown
	statusError
)

var (
	// The client has not yet connected, or it has been
	// disconnected from the server.
	StatusUnconnected Status = statusUnconnected
	// Initial connection established.
	StatusConnected Status = statusConnected
	// Like StatusConnected, but with TLS.
	StatusConnectedTls Status = statusConnectedTls
	// Authentication succeeded.
	StatusAuthenticated Status = statusAuthenticated
	// Resource binding complete.
	StatusBound Status = statusBound
	// Session has started and normal message traffic can be sent
	// and received.
	StatusRunning Status = statusRunning
	// The session has closed, or is in the process of closing.
	StatusShutdown Status = statusShutdown
	// The session has encountered an error. Otherwise identical
	// to StatusShutdown.
	StatusError Status = statusError
)

// Does the status value indicate that the client is or has
// disconnected?
func (s Status) Fatal() bool {
	switch s {
	default:
		return false
	case StatusShutdown, StatusError:
		return true
	}
}

type statmgr struct {
	newStatus   chan Status
	newlistener chan chan Status
}

func newStatmgr(client chan<- Status) *statmgr {
	s := statmgr{}
	s.newStatus = make(chan Status)
	s.newlistener = make(chan chan Status)
	go s.manager(client)
	return &s
}

func (s *statmgr) manager(client chan<- Status) {
	// We handle this specially, in case the client doesn't read
	// our final status message.
	defer func() {
		if client != nil {
			select {
			case client <- StatusShutdown:
			default:
			}
			close(client)
		}
	}()

	stat := StatusUnconnected
	listeners := []chan Status{}
	for {
		select {
		case stat = <-s.newStatus:
			for _, l := range listeners {
				sendToListener(l, stat)
			}
			if client != nil && stat != StatusShutdown {
				client <- stat
			}
		case l, ok := <-s.newlistener:
			if !ok {
				return
			}
			defer close(l)
			sendToListener(l, stat)
			listeners = append(listeners, l)
		}
	}
}

func sendToListener(listen chan Status, stat Status) {
	for {
		select {
		case <-listen:
		case listen <- stat:
			return
		}
	}
}

func (cl *Client) setStatus(stat Status) {
	cl.statmgr.setStatus(stat)
}

func (s *statmgr) setStatus(stat Status) {
	s.newStatus <- stat
}

func (s *statmgr) newListener() <-chan Status {
	l := make(chan Status, 1)
	s.newlistener <- l
	return l
}

func (s *statmgr) close() {
	close(s.newlistener)
}

func (s *statmgr) awaitStatus(waitFor Status) error {
	// BUG(chris): This routine leaks one channel each time it's
	// called. Listeners are never removed.
	l := s.newListener()
	for current := range l {
		if current == waitFor {
			return nil
		}
		if current.Fatal() {
			break
		}
		if current > waitFor {
			return nil
		}
	}
	return fmt.Errorf("shut down waiting for status change")
}

package xmpp

import (
	"testing"
	"time"
)

func TestStatusListen(t *testing.T) {
	sm := newStatmgr(nil)
	l := sm.newListener()
	stat, ok := <-l
	if !ok {
		t.Error()
	} else if stat != StatusUnconnected {
		t.Errorf("got %d", stat)
	}

	sm.setStatus(StatusConnected)
	stat, ok = <-l
	if !ok {
		t.Error()
	} else if stat != StatusConnected {
		t.Errorf("got %d", stat)
	}

	sm.setStatus(StatusBound)
	stat, ok = <-l
	if !ok {
		t.Error()
	} else if stat != StatusBound {
		t.Errorf("got %d", stat)
	}

	sm.setStatus(StatusShutdown)
	stat = <-l
	if stat != StatusShutdown {
		t.Errorf("got %d", stat)
	}
}

func TestAwaitStatus(t *testing.T) {
	sm := newStatmgr(nil)

	syncCh := make(chan int)

	go func() {
		sm.setStatus(StatusConnected)
		sm.setStatus(StatusBound)
		time.Sleep(100 * time.Millisecond)
		syncCh <- 0
	}()

	err := sm.awaitStatus(StatusBound)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-syncCh:
		t.Fatal("didn't wait")
	default:
	}
	<-syncCh
}

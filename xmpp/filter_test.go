package xmpp

import (
	"fmt"
	"strconv"
	"testing"
)

func TestCloseIn(t *testing.T) {
	add := make(chan Filter)
	in := make(chan Stanza)
	out := make(chan Stanza)
	go filterMgr(add, in, out)
	close(in)
	_, ok := <-out
	if ok {
		fmt.Errorf("out didn't close")
	}
}

func passthru(in <-chan Stanza, out chan<- Stanza) {
	defer close(out)
	for stan := range in {
		out <- stan
	}
}

func TestFilters(t *testing.T) {
	for n := 0; n < 10; n++ {
		filterN(n, t)
	}
}

func filterN(numFilts int, t *testing.T) {
	add := make(chan Filter)
	in := make(chan Stanza)
	defer close(in)
	out := make(chan Stanza)
	go filterMgr(add, in, out)
	for i := 0; i < numFilts; i++ {
		add <- passthru
	}
	go func() {
		for i := 0; i < 100; i++ {
			msg := Message{}
			msg.Id = fmt.Sprintf("%d", i)
			in <- &msg
		}
	}()
	for i := 0; i < 100; i++ {
		stan := <-out
		msg, ok := stan.(*Message)
		if !ok {
			t.Errorf("N = %d: msg %d not a Message: %#v", numFilts,
				i, stan)
			continue
		}
		n, err := strconv.Atoi(msg.Header.Id)
		if err != nil {
			t.Errorf("N = %d: msg %d parsing ID '%s': %v", numFilts,
				i, msg.Header.Id, err)
			continue
		}
		if n != i {
			t.Errorf("N = %d: msg %d wrong id %d", numFilts, i, n)
		}
	}
}

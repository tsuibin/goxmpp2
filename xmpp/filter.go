package xmpp

// Manages the stack of filters that can read and modify stanzas on
// their way from the remote to the application.

// Receive new filters on filterAdd; those new filters get added to
// the top of the stack. Receive stanzas at the bottom of the stack on
// input. Send stanzas out the top of the stack on output.
func filterMgr(filterAdd <-chan Filter, input <-chan Stanza, output chan<- Stanza) {
	defer close(output)
	for {
		select {
		case stan, ok := <-input:
			if !ok {
				return
			}
			output <- stan

		case filt := <-filterAdd:
			ch := make(chan Stanza)
			go filt(input, ch)
			input = ch
		}
	}
}

// AddRecvFilter adds a new filter to the top of the stack through which
// incoming stanzas travel on their way up to the client.
func (cl *Client) AddRecvFilter(filt Filter) {
	if filt == nil {
		return
	}
	cl.recvFilterAdd <- filt
}

// AddSendFilter adds a new filter to the top of the stack through
// which outgoing stanzas travel on their way down from the client to
// the network.
func (cl *Client) AddSendFilter(filt Filter) {
	if filt == nil {
		return
	}
	cl.sendFilterAdd <- filt
}

package xmpp

// Code to generate unique IDs for outgoing messages.

import (
	"fmt"
)

var id <-chan string

func init() {
	// Start the unique id generator.
	idCh := make(chan string)
	id = idCh
	go func(ch chan<- string) {
		id := int64(1)
		for {
			str := fmt.Sprintf("id_%d", id)
			ch <- str
			id++
		}
	}(idCh)
}

// This function may be used as a convenient way to generate a unique
// id for an outgoing iq, message, or presence stanza.
func NextId() string {
	return <-id
}

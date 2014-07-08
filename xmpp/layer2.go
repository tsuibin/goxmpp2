// This layer of the XMPP protocol translates between bytes and XMLish
// structures.

package xmpp

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"reflect"
	"strings"
)

// Read bytes from a reader, unmarshal them as XML into structures of
// the appropriate type, and send those structures on a channel.
func (cl *Client) recvXml(r io.Reader, ch chan<- interface{},
	extStanza map[xml.Name]reflect.Type) {

	defer close(ch)

	// This trick loads our namespaces into the parser.
	nsstr := fmt.Sprintf(`<a xmlns="%s" xmlns:stream="%s">`,
		NsClient, NsStream)
	nsrdr := strings.NewReader(nsstr)
	p := xml.NewDecoder(io.MultiReader(nsrdr, r))
	p.Token()

Loop:
	for {
		// Sniff the next token on the stream.
		t, err := p.Token()
		if t == nil {
			if err != io.EOF {
				cl.setError(fmt.Errorf("recv: %v", err))
			}
			break
		}
		var se xml.StartElement
		var ok bool
		if se, ok = t.(xml.StartElement); !ok {
			continue
		}

		// Allocate the appropriate structure for this token.
		var obj interface{}
		switch se.Name.Space + " " + se.Name.Local {
		case NsStream + " stream":
			st, err := parseStream(se)
			if err != nil {
				cl.setError(fmt.Errorf("recv: %v", err))
				break Loop
			}
			ch <- st
			continue
		case "stream error", NsStream + " error":
			obj = &streamError{}
		case NsStream + " features":
			obj = &Features{}
		case NsTLS + " proceed", NsTLS + " failure":
			obj = &starttls{}
		case NsSASL + " challenge", NsSASL + " failure",
			NsSASL + " success":
			obj = &auth{}
		case NsClient + " iq":
			obj = &Iq{}
		case NsClient + " message":
			obj = &Message{}
		case NsClient + " presence":
			obj = &Presence{}
		default:
			obj = &Generic{}
			if Debug {
				log.Printf("Ignoring unrecognized: %s %s",
					se.Name.Space, se.Name.Local)
			}
		}

		// Read the complete XML stanza.
		err = p.DecodeElement(obj, &se)
		if err != nil {
			cl.setError(fmt.Errorf("recv: %v", err))
			break Loop
		}

		// If it's a Stanza, we try to unmarshal its innerxml
		// into objects of the appropriate respective
		// types. This is specified by our extensions.
		if st, ok := obj.(Stanza); ok {
			err = parseExtended(st.GetHeader(), extStanza)
			if err != nil {
				cl.setError(fmt.Errorf("recv: %v", err))
				break Loop
			}
		}

		// Put it on the channel.
		ch <- obj
	}
}

func parseExtended(st *Header, extStanza map[xml.Name]reflect.Type) error {
	// Now parse the stanza's innerxml to find the string that we
	// can unmarshal this nested element from.
	reader := strings.NewReader(st.Innerxml)
	p := xml.NewDecoder(reader)
	for {
		t, err := p.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if se, ok := t.(xml.StartElement); ok {
			if typ, ok := extStanza[se.Name]; ok {
				nested := reflect.New(typ).Interface()

				// Unmarshal the nested element and
				// stuff it back into the stanza.
				err := p.DecodeElement(nested, &se)
				if err != nil {
					return err
				}
				st.Nested = append(st.Nested, nested)
			}
		}
	}

	return nil
}

// Receive structures on a channel, marshal them to XML, and send the
// bytes on a writer.
func (cl *Client) sendXml(w io.Writer, ch <-chan interface{}) {
	defer func(w io.Writer) {
		if c, ok := w.(io.Closer); ok {
			c.Close()
		}
	}(w)

	enc := xml.NewEncoder(w)

	for obj := range ch {
		if st, ok := obj.(*stream); ok {
			_, err := w.Write([]byte(st.String()))
			if err != nil {
				cl.setError(fmt.Errorf("send: %v", err))
				break
			}
		} else {
			err := enc.Encode(obj)
			if err != nil {
				cl.setError(fmt.Errorf("send: %v", err))
				break
			}
		}
	}
}

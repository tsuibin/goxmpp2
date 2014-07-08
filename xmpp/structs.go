package xmpp

// This file contains data structures.

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"log"
	"reflect"
	"strings"
)

// BUG(cjyar): Doesn't use stringprep. Could try the implementation at
// "code.google.com/p/go-idn/src/stringprep"

// JID represents an entity that can communicate with other
// entities. It looks like node@domain/resource. Node and resource are
// sometimes optional.
type JID string

// XMPP's <stream:stream> XML element
type stream struct {
	XMLName xml.Name `xml:"stream=http://etherx.jabber.org/streams stream"`
	To      string   `xml:"to,attr"`
	From    string   `xml:"from,attr"`
	Id      string   `xml:"id,attr"`
	Lang    string   `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	Version string   `xml:"version,attr"`
}

var _ fmt.Stringer = &stream{}

// <stream:error>
type streamError struct {
	XMLName xml.Name `xml:"http://etherx.jabber.org/streams error"`
	Any     Generic  `xml:",any"`
	Text    *errText
}

type errText struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-streams text"`
	Lang    string   `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	Text    string   `xml:",chardata"`
}

type Features struct {
	Starttls   *starttls `xml:"urn:ietf:params:xml:ns:xmpp-tls starttls"`
	Mechanisms mechs     `xml:"urn:ietf:params:xml:ns:xmpp-sasl mechanisms"`
	Bind       *bindIq
	Session    *Generic
	Any        *Generic
}

type starttls struct {
	XMLName  xml.Name
	Required *string
}

type mechs struct {
	Mechanism []string `xml:"urn:ietf:params:xml:ns:xmpp-sasl mechanism"`
}

type auth struct {
	XMLName   xml.Name
	Chardata  string `xml:",chardata"`
	Mechanism string `xml:"mechanism,attr,omitempty"`
	Any       *Generic
}

type Stanza interface {
	GetHeader() *Header
}

// One of the three core XMPP stanza types: iq, message, presence. See
// RFC3920, section 9.
type Header struct {
	To       JID    `xml:"to,attr,omitempty"`
	From     JID    `xml:"from,attr,omitempty"`
	Id       string `xml:"id,attr,omitempty"`
	Type     string `xml:"type,attr,omitempty"`
	Lang     string `xml:"http://www.w3.org/XML/1998/namespace lang,attr,omitempty"`
	Innerxml string `xml:",innerxml"`
	Error    *Error
	Nested   []interface{}
}

// message stanza
type Message struct {
	XMLName xml.Name `xml:"jabber:client message"`
	Header
	Subject []Text `xml:"jabber:client subject"`
	Body    []Text `xml:"jabber:client body"`
	Thread  *Data  `xml:"jabber:client thread"`
}

var _ Stanza = &Message{}

// presence stanza
type Presence struct {
	XMLName xml.Name `xml:"presence"`
	Header
	Show     *Data  `xml:"jabber:client show"`
	Status   []Text `xml:"jabber:client status"`
	Priority *Data  `xml:"jabber:client priority"`
}

var _ Stanza = &Presence{}

// iq stanza
type Iq struct {
	XMLName xml.Name `xml:"iq"`
	Header
}

var _ Stanza = &Iq{}

// Describes an XMPP stanza error. See RFC 3920, Section 9.3.
type Error struct {
	XMLName xml.Name `xml:"error"`
	// The error type attribute.
	Type string `xml:"type,attr"`
	// Any nested element, if present.
	Any *Generic
}

var _ error = &Error{}

// Used for resource binding as a nested element inside <iq/>.
type bindIq struct {
	XMLName  xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-bind bind"`
	Resource *string  `xml:"resource"`
	Jid      *JID     `xml:"jid"`
}

// Holds human-readable text, with an optional language
// specification. Generally multiple instances of these can be found
// together, allowing the software to choose which language to present
// to the user.
type Text struct {
	XMLName  xml.Name
	Lang     string `xml:"http://www.w3.org/XML/1998/namespace lang,attr,omitempty"`
	Chardata string `xml:",chardata"`
}

// Non-human-readable content of some sort, used by the protocol.
type Data struct {
	XMLName  xml.Name
	Chardata string `xml:",chardata"`
}

// Holds an XML element not described by the more specific types.
type Generic struct {
	XMLName  xml.Name
	Any      *Generic `xml:",any"`
	Chardata string   `xml:",chardata"`
}

var _ fmt.Stringer = &Generic{}

func (j JID) Node() string {
	at := strings.Index(string(j), "@")
	if at == -1 {
		return ""
	}
	return string(j[:at])
}

func (j JID) Domain() string {
	at := strings.Index(string(j), "@")
	slash := strings.LastIndex(string(j), "/")
	if slash == -1 {
		slash = len(j)
	}
	return string(j[at+1 : slash])
}

func (j JID) Resource() string {
	slash := strings.LastIndex(string(j), "/")
	if slash == -1 {
		return ""
	}
	return string(j[slash+1:])
}

// Returns the bare JID, which is the JID without the resource part.
func (j JID) Bare() JID {
	node := j.Node()
	if node == "" {
		return JID(j.Domain())
	}
	return JID(fmt.Sprintf("%s@%s", node, j.Domain()))
}

func (s *stream) String() string {
	var buf bytes.Buffer
	buf.WriteString(`<stream:stream xmlns="`)
	buf.WriteString(NsClient)
	buf.WriteString(`" xmlns:stream="`)
	buf.WriteString(NsStream)
	buf.WriteString(`"`)
	if s.To != "" {
		buf.WriteString(` to="`)
		xml.Escape(&buf, []byte(s.To))
		buf.WriteString(`"`)
	}
	if s.From != "" {
		buf.WriteString(` from="`)
		xml.Escape(&buf, []byte(s.From))
		buf.WriteString(`"`)
	}
	if s.Id != "" {
		buf.WriteString(` id="`)
		xml.Escape(&buf, []byte(s.Id))
		buf.WriteString(`"`)
	}
	if s.Lang != "" {
		buf.WriteString(` xml:lang="`)
		xml.Escape(&buf, []byte(s.Lang))
		buf.WriteString(`"`)
	}
	if s.Version != "" {
		buf.WriteString(` version="`)
		xml.Escape(&buf, []byte(s.Version))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
	return buf.String()
}

func parseStream(se xml.StartElement) (*stream, error) {
	s := &stream{}
	for _, attr := range se.Attr {
		switch strings.ToLower(attr.Name.Local) {
		case "to":
			s.To = attr.Value
		case "from":
			s.From = attr.Value
		case "id":
			s.Id = attr.Value
		case "lang":
			s.Lang = attr.Value
		case "version":
			s.Version = attr.Value
		}
	}
	return s, nil
}

func (iq *Iq) GetHeader() *Header {
	return &iq.Header
}

func (m *Message) GetHeader() *Header {
	return &m.Header
}

func (p *Presence) GetHeader() *Header {
	return &p.Header
}

func (u *Generic) String() string {
	if u == nil {
		return "nil"
	}
	var sub string
	if u.Any != nil {
		sub = u.Any.String()
	}
	return fmt.Sprintf("<%s %s>%s%s</%s %s>", u.XMLName.Space,
		u.XMLName.Local, sub, u.Chardata, u.XMLName.Space,
		u.XMLName.Local)
}

func (er *Error) Error() string {
	buf, err := xml.Marshal(er)
	if err != nil {
		log.Println("double bad error: couldn't marshal error")
		return "unreadable error"
	}
	return string(buf)
}

var bindExt Extension = Extension{}

func init() {
	bindExt.StanzaTypes = make(map[xml.Name]reflect.Type)
	bName := xml.Name{Space: NsBind, Local: "bind"}
	bindExt.StanzaTypes[bName] = reflect.TypeOf(bindIq{})
}

package xmpp

// This file contains support for roster management, RFC 3921, Section 7.

import (
	"encoding/xml"
	"reflect"
)

// Roster query/result
type RosterQuery struct {
	XMLName xml.Name     `xml:"jabber:iq:roster query"`
	Item    []RosterItem `xml:"item"`
}

// See RFC 3921, Section 7.1.
type RosterItem struct {
	XMLName      xml.Name `xml:"jabber:iq:roster item"`
	Jid          JID      `xml:"jid,attr"`
	Subscription string   `xml:"subscription,attr"`
	Name         string   `xml:"name,attr"`
	Group        []string
}

type Roster struct {
	Extension
	get      chan []RosterItem
	toServer chan Stanza
}

type rosterClient struct {
	rosterChan   <-chan []RosterItem
	rosterUpdate chan<- RosterItem
}

func (r *Roster) rosterMgr(upd <-chan Stanza) {
	roster := make(map[JID]RosterItem)
	var snapshot []RosterItem
	var get chan<- []RosterItem
	for {
		select {
		case get <- snapshot:

		case stan, ok := <-upd:
			if !ok {
				return
			}
			iq, ok := stan.(*Iq)
			if !ok {
				continue
			}
			if iq.Type != "result" && iq.Type != "set" {
				continue
			}
			var rq *RosterQuery
			for _, ele := range iq.Nested {
				if q, ok := ele.(*RosterQuery); ok {
					rq = q
					break
				}
			}
			if rq == nil {
				continue
			}
			for _, item := range rq.Item {
				roster[item.Jid] = item
			}
			snapshot = []RosterItem{}
			for _, ri := range roster {
				snapshot = append(snapshot, ri)
			}
			get = r.get
		}
	}
}

func (r *Roster) makeFilters() (Filter, Filter) {
	rosterUpdate := make(chan Stanza)
	go r.rosterMgr(rosterUpdate)
	recv := func(in <-chan Stanza, out chan<- Stanza) {
		defer close(out)
		defer close(rosterUpdate)
		for stan := range in {
			rosterUpdate <- stan
			out <- stan
		}
	}
	send := func(in <-chan Stanza, out chan<- Stanza) {
		defer close(out)
		for {
			select {
			case stan, ok := <-in:
				if !ok {
					return
				}
				out <- stan
			case stan := <-r.toServer:
				out <- stan
			}
		}
	}
	return recv, send
}

func newRosterExt() *Roster {
	r := Roster{}
	r.StanzaTypes = make(map[xml.Name]reflect.Type)
	rName := xml.Name{Space: NsRoster, Local: "query"}
	r.StanzaTypes[rName] = reflect.TypeOf(RosterQuery{})
	r.RecvFilter, r.SendFilter = r.makeFilters()
	r.get = make(chan []RosterItem)
	r.toServer = make(chan Stanza)
	return &r
}

// Return the most recent snapshot of the roster status. This is
// updated automatically as roster updates are received from the
// server. This function may block immediately after the XMPP
// connection has been established, until the first roster update is
// received from the server.
func (r *Roster) Get() []RosterItem {
	return <-r.get
}

// Asynchronously fetch this entity's roster from the server.
func (r *Roster) update() {
	iq := &Iq{Header: Header{Type: "get", Id: NextId(),
		Nested: []interface{}{RosterQuery{}}}}
	r.toServer <- iq
}

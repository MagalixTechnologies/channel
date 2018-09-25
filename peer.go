package channel

import (
	"encoding/gob"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
)

var (
	upgrader = websocket.Upgrader{}
	mt       = websocket.BinaryMessage
)

type clientPacket struct {
	Client uuid.UUID
	Packet packetStruct
}

type peer struct {
	ID  uuid.UUID
	c   *websocket.Conn
	ch  *Channel
	out chan packetStruct

	nextID int
}

func newPeer(c *websocket.Conn, ch *Channel) *peer {
	u, err := uuid.NewV4()
	if err == nil {
		u, err = uuid.NewV1()
		if err != nil {
			u = uuid.Nil
		}
	}
	return &peer{
		ID:     u,
		c:      c,
		ch:     ch,
		out:    make(chan packetStruct, 10),
		nextID: ch.startID,
	}
}

func (p *peer) NextID() int {
	p.nextID += 2
	return p.nextID
}
func (p *peer) handle() {
	go func() {
		for packet := range p.out {
			p.c.SetWriteDeadline(time.Now().Add(p.ch.options.ProtoWrite))
			w, err := p.c.NextWriter(mt)
			if err != nil {
				p.ch.in <- clientPacket{Packet: packetStruct{
					ID:       packet.ID,
					Endpoint: packet.Endpoint,
					Error:    ApplyReason(LocalError, "write error", err),
				}, Client: p.ID}
				log.Println("write error:", err)
				continue
			}
			e := gob.NewEncoder(w)
			err = e.Encode(packet)
			if err != nil {
				p.ch.in <- clientPacket{Packet: packetStruct{
					ID:       packet.ID,
					Endpoint: packet.Endpoint,
					Error:    ApplyReason(LocalError, "marshal error", err),
				}, Client: p.ID}
			}
			w.Close()
		}
	}()
	for {
		_, r, err := p.c.NextReader()
		if err != nil {
			p.close()
			return
		}
		var packet packetStruct
		d := gob.NewDecoder(r)
		err = d.Decode(&packet)
		if err != nil {
			p.out <- packetStruct{
				ID:       packet.ID,
				Endpoint: packet.Endpoint,
				Error:    ApplyReason(BadRequest, "bad request", err),
			}
		}
		p.ch.in <- clientPacket{Packet: packet, Client: p.ID}
	}
}

func (p *peer) close() {
	close(p.out)
	p.out = nil
	p.c.Close()
}

func (ch *Channel) serve(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	peer := newPeer(c, ch)
	ch.Peers[peer.ID] = peer
	defer delete(ch.Peers, peer.ID)
	peer.handle()
}

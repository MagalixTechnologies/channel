package channel

import (
	"encoding/gob"
	"log"
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
	ID   uuid.UUID
	URI  string
	Exit chan struct{}
	c    *websocket.Conn
	ch   *Channel
	out  chan packetStruct

	nextID int
}

func newPeer(c *websocket.Conn, ch *Channel, uri string) *peer {
	u, err := uuid.NewV4()
	if err == nil {
		u, err = uuid.NewV1()
		if err != nil {
			u = uuid.Nil
		}
	}
	return &peer{
		ID:     u,
		URI:    uri,
		Exit:   make(chan struct{}, 1),
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
			log.Printf("writing packet with id %d.\n", packet.ID)
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
			log.Printf("written packet with id %d.\n", packet.ID)
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
		log.Printf("reading packet with id %d.\n", packet.ID)
		p.ch.in <- clientPacket{Packet: packet, Client: p.ID}
		log.Printf("read packet with id %d.\n", packet.ID)
	}
}

func (p *peer) close() {
	close(p.out)
	p.out = nil
	p.Exit <- struct{}{}
	p.c.Close()
}

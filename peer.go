package channel

import (
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
)

const (
	outChannelSize = 2048
)

var (
	mt = websocket.BinaryMessage
)

type clientPacket struct {
	Client uuid.UUID
	Packet packetStruct
}

type Receiver interface {
	received(clientPacket)
}

type Conn interface {
	SetWriteDeadline(time.Time) error
	NextWriter(int) (io.WriteCloser, error)
	NextReader() (messageType int, r io.Reader, err error)
	RemoteAddr() net.Addr
	Close() error
}

type peerOptions struct {
	startID      int
	writeTimeout time.Duration
}

type peer struct {
	ID         uuid.UUID
	URI        string
	RemoteAddr string
	Exit       chan struct{}
	c          Conn
	ch         Receiver
	out        chan packetStruct

	nextID  int
	options peerOptions

	m sync.Mutex
}

func newPeer(c Conn, ch Receiver, options peerOptions, uri string) *peer {
	u, err := uuid.NewV4()
	if err == nil {
		u, err = uuid.NewV1()
		if err != nil {
			u = uuid.Nil
		}
	}
	return &peer{
		ID:         u,
		URI:        uri,
		RemoteAddr: c.RemoteAddr().String(),
		Exit:       make(chan struct{}, 1),
		c:          c,
		ch:         ch,
		out:        make(chan packetStruct, outChannelSize),
		nextID:     options.startID,
		options:    options,
		m:          sync.Mutex{},
	}
}

func (p *peer) NextID() int {
	p.m.Lock()
	defer p.m.Unlock()
	p.nextID += 2
	return p.nextID
}

func (p *peer) startedHere(id int) bool {
	return p.nextID%2 == id%2
}

func (p *peer) handle() {
	go func() {
		for packet := range p.out {
			if len(p.out) > cap(p.out)-2 {
				fmt.Println("out channel is almost full, this might cause timeout issues")
			}
			p.c.SetWriteDeadline(time.Now().Add(p.options.writeTimeout))
			w, err := p.c.NextWriter(mt)
			if err != nil {
				p.close()
				return
			}
			e := gob.NewEncoder(w)
			err = e.Encode(packet)
			if err != nil {
				if p.startedHere(packet.ID) {
					p.ch.received(clientPacket{Packet: packetStruct{
						ID:       packet.ID,
						Endpoint: packet.Endpoint,
						Error:    ApplyReason(LocalError, "marshal error", err),
					}, Client: p.ID})
				}
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
		p.ch.received(clientPacket{Packet: packet, Client: p.ID})
	}
}

func (p *peer) close() {
	close(p.out)
	p.out = nil
	p.Exit <- struct{}{}
	p.c.Close()
}

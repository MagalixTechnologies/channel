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
	flush(clientPacket)
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

	sync.RWMutex
	out chan packetStruct

	nextID  int
	options peerOptions

	idMutex sync.Mutex
	closed  bool
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
		idMutex:    sync.Mutex{},
	}
}

func (p *peer) NextID() int {
	p.idMutex.Lock()
	defer p.idMutex.Unlock()
	p.nextID += 2
	return p.nextID
}

func (p *peer) Send(packet packetStruct) error {
	p.RLock()
	if p.out == nil {
		return ApplyReason(ClientNotConnected, "connection already closed", nil)
	}
	p.out <- packet
	p.RUnlock()
	return nil
}

func (p *peer) startedHere(id int) bool {
	return p.nextID%2 == id%2
}

func (p *peer) drain() {

	// drain has to be called wht p locked
	for i := 0; i < len(p.out); i++ {
		packet := <-p.out
		if p.startedHere(packet.ID) {
			p.ch.received(clientPacket{Packet: packetStruct{
				ID:       packet.ID,
				Endpoint: packet.Endpoint,
				Error:    ApplyReason(ClientNotConnected, "connection closed", nil),
			}, Client: p.ID})
		}
	}
	p.ch.flush(clientPacket{Packet: packetStruct{
		Error: ApplyReason(ClientNotConnected, "connection closed", nil),
	}, Client: p.ID})
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
				if p.startedHere(packet.ID) {
					p.ch.received(clientPacket{Packet: packetStruct{
						ID:       packet.ID,
						Endpoint: packet.Endpoint,
						Error:    ApplyReason(ClientNotConnected, "write error", nil),
					}, Client: p.ID})
				}
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
			p.Send(packetStruct{
				ID:       packet.ID,
				Endpoint: packet.Endpoint,
				Error:    ApplyReason(BadRequest, "bad request", err),
			})
		}
		p.ch.received(clientPacket{Packet: packet, Client: p.ID})
	}
}

func (p *peer) close() {
	if p.closed {
		return
	}
	p.Lock()
	defer p.Unlock()
	p.closed = true
	p.drain()
	close(p.out)
	p.out = nil
	p.Exit <- struct{}{}
	p.c.Close()
}

package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"

	uuid "github.com/MagalixTechnologies/uuid-go"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{}
	mt       = websocket.TextMessage
)

type packetBody interface{}

type packetStruct struct {
	ID       int            `json:"id"`
	Endpoint string         `json:"endpoint"`
	Body     packetBody     `json:"body,omitempty"`
	Error    *ProtocolError `json:"error,omitempty"`
}

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
	return &peer{
		ID:     uuid.NewV4(),
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
			message, err := json.Marshal(packet)
			if err != nil {
				p.ch.in <- clientPacket{Packet: packetStruct{
					ID:       packet.ID,
					Endpoint: packet.Endpoint,
					Body:     nil,
					Error:    ApplyReason(LocalError, "marshal error", err),
				}, Client: p.ID}
				continue
			}
			err = p.c.WriteMessage(mt, message)
			if err != nil {
				p.ch.in <- clientPacket{Packet: packetStruct{
					ID:       packet.ID,
					Endpoint: packet.Endpoint,
					Error:    ApplyReason(LocalError, "write error", err),
				}, Client: p.ID}
				log.Println("write error:", err)
			}
		}
	}()
	for {
		_, message, err := p.c.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err) {
				p.close()
				return
			}
			log.Println("read error:", err)
			continue
		}
		var packet packetStruct
		err = json.Unmarshal(message, &packet)
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

type ChannelOptions struct{}

type packetSelector struct {
	client uuid.UUID
	id     int
}

type Channel struct {
	Peers   map[uuid.UUID]*peer
	options ChannelOptions

	// odd ids for client and even ids for server
	startID int

	in chan clientPacket

	listeners map[string]func(uuid.UUID, packetBody) (packetBody, error)

	receivers map[packetSelector]chan packetStruct
}

func newChannel(startID int, channelOptions ChannelOptions) *Channel {
	ch := &Channel{
		Peers:   make(map[uuid.UUID]*peer),
		options: channelOptions,

		startID: startID,

		in: make(chan clientPacket),

		listeners: make(map[string]func(uuid.UUID, packetBody) (packetBody, error)),
		receivers: make(map[packetSelector]chan packetStruct),
	}
	return ch
}

func (ch *Channel) isResponse(id int) bool {
	return ch.startID%2 == id%2
}

func (ch *Channel) AddListener(endpoint string, listener func(uuid.UUID, packetBody) (packetBody, error)) {
	// TODO: check if exists
	ch.listeners[endpoint] = listener
}

func (ch *Channel) Init() {
	for req := range ch.in {
		spew.Dump(req)
		if ch.isResponse(req.Packet.ID) {
			selector := packetSelector{
				client: req.Client,
				id:     req.Packet.ID,
			}
			if receiver, ok := ch.receivers[selector]; ok {
				receiver <- req.Packet
				close(receiver)
			}
		} else {
			if listener, ok := ch.listeners[req.Packet.Endpoint]; ok {
				go func(body packetBody) {
					resp, err := listener(req.Client, body)
					var e *ProtocolError

					if tmp, ok := err.(ProtocolError); ok {
						e = &tmp
					} else if tmp, ok := err.(*ProtocolError); ok {
						e = tmp
					} else {
						e = ApplyReason(InternalError, "internal error", err)
					}

					if peer, ok := ch.Peers[req.Client]; ok && peer != nil {
						peer.out <- packetStruct{
							ID:       req.Packet.ID,
							Endpoint: req.Packet.Endpoint,
							Body:     resp,
							Error:    e,
						}
					}
				}(req.Packet.Body)
			} else {
				spew.Dump("ASD")
				if peer, ok := ch.Peers[req.Client]; ok && peer != nil {
					spew.Dump("DEF")
					peer.out <- packetStruct{
						ID:       req.Packet.ID,
						Endpoint: req.Packet.Endpoint,
						Error:    ApplyReason(NotFound, "api not found", nil),
					}
				}
			}
			// TODO: else
		}

	}
}

func (ch *Channel) Send(client uuid.UUID, endpoint string, body packetBody) (packetBody, error) {
	// Note: skipping if the peer is not connected
	if peer, ok := ch.Peers[client]; ok {
		id := peer.NextID()
		receiver := make(chan packetStruct, 1)
		selector := packetSelector{
			client: client,
			id:     id,
		}
		ch.receivers[selector] = receiver
		peer.out <- packetStruct{
			ID:       id,
			Endpoint: endpoint,
			Body:     &body,
		}
		var body packetBody
		var err error
		select {
		case resp := <-receiver:
			body, err = resp.Body, resp.Error
			// TODO: get from config
		case <-time.After(time.Second):
			err = ApplyReason(Timeout, "timeout while receiving response", nil)

		}
		delete(ch.receivers, selector)
		return body, err
	}
	spew.Dump(client, ch.Peers)
	return nil, errors.New("client not found")
}

type Server struct {
	Addr    string
	Channel *Channel
}

func NewServer(addr string, channelOptions ChannelOptions) *Server {
	ch := newChannel(1, channelOptions)
	http.HandleFunc("/", ch.serve)
	return &Server{
		Addr:    addr,
		Channel: ch,
	}
}

func (s *Server) Listen() {
	go s.Channel.Init()
	log.Print(http.ListenAndServe(s.Addr, nil))
}

func (s *Server) Send(client uuid.UUID, endpoint string, body packetBody) (packetBody, error) {
	return s.Channel.Send(client, endpoint, body)
}

type Client struct {
	Addr    string
	Channel *Channel

	server uuid.UUID
}

func NewClient(addr string, channelOptions ChannelOptions) *Client {
	ch := newChannel(0, channelOptions)
	client := &Client{
		Addr:    addr,
		Channel: ch,
	}
	return client
}

func (client *Client) Listen() {
	u := url.URL{Scheme: "ws", Host: client.Addr, Path: "/"}
	go client.Channel.Init()
	for try := 0; ; try++ {
		c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			log.Printf("failed to connect: %s", err)
			// TODO: with backoff
			time.Sleep(time.Second)
			continue
		}
		defer c.Close()
		peer := newPeer(c, client.Channel)
		client.Channel.Peers[peer.ID] = peer
		client.server = peer.ID
		client.Channel.Peers[client.server].handle()
	}
}

func (c *Client) Send(endpoint string, body packetBody) (packetBody, error) {
	return c.Channel.Send(c.server, endpoint, body)
}

func main() {
	switch os.Args[1] {
	case "server":
		s := NewServer("127.0.0.1:"+os.Args[2], ChannelOptions{})
		s.Channel.AddListener("/", func(u uuid.UUID, body packetBody) (packetBody, error) {
			spew.Dump(u, body)
			ret := body.(string) + body.(string)
			return ret, nil
		})
		go func() {
			for {
				time.Sleep(10 * time.Second)
				for u, _ := range s.Channel.Peers {
					spew.Dump(s.Send(u, "/mmm", "asd124"))
				}
			}
		}()
		s.Listen()
	case "client":
		c := NewClient("127.0.0.1:"+os.Args[2], ChannelOptions{})
		c.Channel.AddListener("/mmm", func(u uuid.UUID, body packetBody) (packetBody, error) {
			return nil, errors.New("MIE says hi")
		})
		go func() {
			time.Sleep(time.Second)
			spew.Dump(c.Send("/", "asd"))
		}()
		c.Listen()
	}
}

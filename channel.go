package channel

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
)

// ChannelOptions options for the channel
type ChannelOptions struct {
	ProtoHandshake time.Duration
	ProtoWrite     time.Duration
	ProtoRead      time.Duration
	ProtoReconnect time.Duration
}

type packetSelector struct {
	client uuid.UUID
	id     int
}

// Channel abstracts a connection to an either a server or a client
type Channel struct {
	// A map of a client uuid (generated randomly everytime) and the peer
	peers   sync.Map
	options ChannelOptions

	// odd ids for client and even ids for server
	startID int

	in chan clientPacket

	listeners map[string]func(uuid.UUID, []byte) ([]byte, error)

	receivers map[packetSelector]chan packetStruct

	onConnect    *func(id uuid.UUID, uri string) error
	onDisconnect *func(id uuid.UUID)
}

func newChannel(startID int, channelOptions ChannelOptions) *Channel {
	ch := &Channel{
		peers:   sync.Map{},
		options: channelOptions,

		startID: startID,

		in: make(chan clientPacket),

		listeners: make(map[string]func(uuid.UUID, []byte) ([]byte, error)),
		receivers: make(map[packetSelector]chan packetStruct),
	}
	return ch
}

func (ch *Channel) isResponse(id int) bool {
	return ch.startID%2 == id%2
}

func (ch *Channel) AddListener(endpoint string, listener func(uuid.UUID, []byte) ([]byte, error)) error {
	if _, ok := ch.listeners[endpoint]; ok {
		return errors.New("listener already exists")
	}
	ch.listeners[endpoint] = listener
	return nil
}

func (ch *Channel) Init() {
	for req := range ch.in {
		if ch.isResponse(req.Packet.ID) {
			selector := packetSelector{
				client: req.Client,
				id:     req.Packet.ID,
			}
			if receiver, ok := ch.receivers[selector]; ok {
				receiver <- req.Packet
			}
		} else {
			if listener, ok := ch.listeners[req.Packet.Endpoint]; ok {
				go func(body []byte) {
					resp, err := listener(req.Client, body)
					var e *ProtocolError

					if tmp, ok := err.(ProtocolError); ok {
						e = &tmp
					} else if tmp, ok := err.(*ProtocolError); ok {
						e = tmp
					} else if err != nil {
						e = ApplyReason(InternalError, "internal error", err)
					}

					if peer := ch.GetPeer(req.Client); peer != nil {
						peer.out <- packetStruct{
							ID:       req.Packet.ID,
							Endpoint: req.Packet.Endpoint,
							Body:     resp,
							Error:    e,
						}
					}
				}(req.Packet.Body)
			} else {
				if peer := ch.GetPeer(req.Client); peer != nil {
					peer.out <- packetStruct{
						ID:       req.Packet.ID,
						Endpoint: req.Packet.Endpoint,
						Error:    ApplyReason(NotFound, "api not found", nil),
					}
				}
			}
		}

	}
}

func (ch *Channel) Send(client uuid.UUID, endpoint string, body []byte) ([]byte, error) {
	// Note: skipping if the peer is not connected
	if peer := ch.GetPeer(client); peer != nil {
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
			Body:     body,
		}
		var body []byte
		var err error
		select {
		case resp := <-receiver:
			body, err = resp.Body, resp.Error
			// to make sure that nil is untyped
			if resp.Error == nil {
				err = nil
			}
		case <-time.After(ch.options.ProtoRead):
			err = ApplyReason(Timeout, "timeout while receiving response", nil)

		}
		delete(ch.receivers, selector)
		return body, err
	}
	return nil, errors.New("client not found")
}

// SetHooks sets connection and disconnection hooks
func (ch *Channel) SetHooks(
	onConnect *func(id uuid.UUID, uri string) error,
	onDisconnect *func(id uuid.UUID),
) {
	ch.onConnect = onConnect
	ch.onDisconnect = onDisconnect
}

func (ch *Channel) NewPeer(c *websocket.Conn, uri string) *peer {
	return newPeer(c, ch, uri)
}

func (ch *Channel) HandlePeer(peer *peer) {
	ch.peers.Store(peer.ID, peer)
	defer ch.peers.Delete(peer.ID)
	go peer.handle()
	if ch.onConnect != nil {
		err := (*ch.onConnect)(peer.ID, peer.URI)
		// TODO: add before, after connect
		if err != nil {
			fmt.Println(err)
		}
	}
	defer func() {
		if ch.onDisconnect != nil {
			(*ch.onDisconnect)(peer.ID)
		}
	}()
	<-peer.Exit
}

func (ch *Channel) GetPeer(id uuid.UUID) *peer {
	if peerInterface, ok := ch.peers.Load(id); ok {
		if peer, ok := peerInterface.(*peer); ok {
			return peer
		}
	}
	return nil
}

func (ch *Channel) ListPeers() []uuid.UUID {
	res := make([]uuid.UUID, 0)
	ch.peers.Range(func(u interface{}, _ interface{}) bool {
		res = append(res, u.(uuid.UUID))
		return true
	})
	return res
}

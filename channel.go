package channel

import (
	"context"
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

	listeners   map[string]func(context.Context, uuid.UUID, []byte) ([]byte, error)
	middlewares []func(context.Context, string, uuid.UUID, []byte, func(context.Context, uuid.UUID, []byte) ([]byte, error)) ([]byte, error)

	receivers sync.Map

	onConnect    *func(id uuid.UUID, uri string, remoteAddr string) error
	onDisconnect *func(id uuid.UUID)
}

func newChannel(startID int, channelOptions ChannelOptions) *Channel {
	ch := &Channel{
		peers:   sync.Map{},
		options: channelOptions,

		startID: startID,

		in: make(chan clientPacket),

		listeners:   make(map[string]func(context.Context, uuid.UUID, []byte) ([]byte, error)),
		middlewares: make([]func(context.Context, string, uuid.UUID, []byte, func(context.Context, uuid.UUID, []byte) ([]byte, error)) ([]byte, error), 0),

		receivers: sync.Map{},
	}
	return ch
}

func (ch *Channel) startedHere(id int) bool {
	return ch.startID%2 == id%2
}

func (ch *Channel) AddMiddleware(middleware func(context.Context, string, uuid.UUID, []byte, func(context.Context, uuid.UUID, []byte) ([]byte, error)) ([]byte, error)) {
	ch.middlewares = append(ch.middlewares, middleware)
}

func (ch *Channel) AddListener(endpoint string, listener func(context.Context, uuid.UUID, []byte) ([]byte, error)) error {
	if _, ok := ch.listeners[endpoint]; ok {
		return errors.New("listener already exists")
	}
	ch.listeners[endpoint] = listener
	return nil
}

func (ch *Channel) Init() {
	for req := range ch.in {
		if ch.startedHere(req.Packet.ID) {
			selector := packetSelector{
				client: req.Client,
				id:     req.Packet.ID,
			}
			if receiver, ok := ch.receivers.Load(selector); ok {
				receiver.(chan packetStruct) <- req.Packet
			}
		} else {
			if listener, ok := ch.listeners[req.Packet.Endpoint]; ok {
				last := listener
				for _, middleware := range ch.middlewares {
					wrapper := func(endpoint string, last func(c context.Context, u uuid.UUID, b []byte) ([]byte, error)) func(context.Context, uuid.UUID, []byte) ([]byte, error) {
						return func(c context.Context, u uuid.UUID, b []byte) ([]byte, error) {
							return middleware(c, endpoint, u, b, last)
						}
					}(req.Packet.Endpoint, last)
					last = wrapper
				}
				go func(client uuid.UUID, id int, endpoint string, body []byte) {
					resp, err := last(context.TODO(), client, body)
					var e *ProtocolError

					if tmp, ok := err.(ProtocolError); ok {
						e = &tmp
					} else if tmp, ok := err.(*ProtocolError); ok {
						e = tmp
					} else if err != nil {
						e = ApplyReason(InternalError, "internal error", err)
					}

					if peer := ch.GetPeer(client); peer != nil {
						peer.Send(packetStruct{
							ID:       id,
							Endpoint: endpoint,
							Body:     resp,
							Error:    e,
						})
					}
				}(req.Client, req.Packet.ID, req.Packet.Endpoint, req.Packet.Body)
			} else {
				if peer := ch.GetPeer(req.Client); peer != nil {
					peer.Send(packetStruct{
						ID:       req.Packet.ID,
						Endpoint: req.Packet.Endpoint,
						Error:    ApplyReason(NotFound, fmt.Sprintf("api not found: %s", req.Packet.Endpoint), nil),
					})
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
		ch.receivers.Store(selector, receiver)
		err := peer.Send(packetStruct{
			ID:       id,
			Endpoint: endpoint,
			Body:     body,
		})
		if err != nil {
			err = ApplyReason(ClientNotConnected, fmt.Sprintf("client not connected: %s", client), err)
			return nil, err
		}
		var body []byte
		select {
		case resp := <-receiver:
			body, err = resp.Body, resp.Error
			// to make sure that nil is untyped
			if resp.Error == nil {
				err = nil
			}
		case <-time.After(ch.options.ProtoRead):
			err = ApplyReason(Timeout, fmt.Sprintf("timeout while receiving response with endpont: %s, id: %d", endpoint, id), nil)

		}
		ch.receivers.Delete(selector)
		return body, err
	}
	return nil, errors.New("client not found")
}

func (ch *Channel) flush(packet clientPacket) {
	ch.receivers.Range(func(k, v interface{}) bool {
		selector := k.(packetSelector)
		if selector.client == packet.Client {
			receiver := v.(chan packetStruct)
			receiver <- packet.Packet
		}
		return true
	})
}

func (ch *Channel) received(packet clientPacket) {
	ch.in <- packet
}

// SetHooks sets connection and disconnection hooks
func (ch *Channel) SetHooks(
	onConnect *func(id uuid.UUID, uri string, remoteAddr string) error,
	onDisconnect *func(id uuid.UUID),
) {
	ch.onConnect = onConnect
	ch.onDisconnect = onDisconnect
}

func (ch *Channel) NewPeer(c *websocket.Conn, uri string) *peer {
	return newPeer(c, ch, peerOptions{
		startID:      ch.startID,
		writeTimeout: ch.options.ProtoWrite,
	}, uri)
}

func (ch *Channel) HandlePeer(peer *peer) {
	ch.peers.Store(peer.ID, peer)
	defer ch.peers.Delete(peer.ID)
	go peer.handle()
	if ch.onConnect != nil {
		go func() {
			err := (*ch.onConnect)(peer.ID, peer.URI, peer.RemoteAddr)
			// TODO: add before, after connect
			if err != nil {
				fmt.Println(err)
			}
		}()
	}
	<-peer.Exit
	if ch.onDisconnect != nil {
		(*ch.onDisconnect)(peer.ID)
	}
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

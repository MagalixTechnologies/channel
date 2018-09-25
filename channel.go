package channel

import (
	"errors"
	"time"

	uuid "github.com/MagalixTechnologies/uuid-go"
)

// ChannelOptions options for the channel
type ChannelOptions struct {
	protoHandshake time.Duration
	protoWrite     time.Duration
	protoRead      time.Duration
	protoReconnect time.Duration
}

type packetSelector struct {
	client uuid.UUID
	id     int
}

// Channel abstracts a connection to an either a server or a client
type Channel struct {
	// A map of a client uuid (generated randomly everytime) and the peer
	Peers   map[uuid.UUID]*peer
	options ChannelOptions

	// odd ids for client and even ids for server
	startID int

	in chan clientPacket

	listeners map[string]func(uuid.UUID, []byte) ([]byte, error)

	receivers map[packetSelector]chan packetStruct
}

func newChannel(startID int, channelOptions ChannelOptions) *Channel {
	ch := &Channel{
		Peers:   make(map[uuid.UUID]*peer),
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
				close(receiver)
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
				if peer, ok := ch.Peers[req.Client]; ok && peer != nil {
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
			Body:     body,
		}
		var body []byte
		var err error
		select {
		case resp := <-receiver:
			body, err = resp.Body, resp.Error
		case <-time.After(ch.options.protoRead):
			err = ApplyReason(Timeout, "timeout while receiving response", nil)

		}
		delete(ch.receivers, selector)
		return body, err
	}
	return nil, errors.New("client not found")
}

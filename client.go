package channel

import (
	"context"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
)

// Client a protocol client
type Client struct {
	URL     url.URL
	Channel *Channel

	server uuid.UUID
}

// NewClient Creates a new Client
// addr is the address to connect to
// channelOptions options, see ChannelOptions docs
func NewClient(url url.URL, channelOptions ChannelOptions) *Client {
	ch := newChannel(0, channelOptions)
	client := &Client{
		URL:     url,
		Channel: ch,
	}
	return client
}

// Listen starts connection loop to the server, auto connects when connection fails
func (c *Client) Listen() {
	go c.Channel.Init()
	for try := 0; ; try++ {
		dialer := websocket.Dialer{
			HandshakeTimeout: c.Channel.options.ProtoHandshake,
		}
		con, _, err := dialer.Dial(
			c.URL.String(), nil,
		)
		if err != nil {
			log.Printf("failed to connect: %s", err)
			time.Sleep(c.Channel.options.ProtoReconnect)
			continue
		}
		defer con.Close()

		peer := c.Channel.NewPeer(con, "")
		c.server = peer.ID
		c.Channel.HandlePeer(peer)
		time.Sleep(c.Channel.options.ProtoReconnect)
	}
}

// Send sends a byte array on the specified endpoint
// returns an optional response, an optional error
func (c *Client) Send(endpoint string, body []byte) ([]byte, error) {
	return c.Channel.Send(c.server, endpoint, body)
}

// IsConnected checks if a connection to the server is established
func (c *Client) IsConnected() bool {
	peer := c.Channel.GetPeer(c.server)
	return peer != nil
}

// TODO: implement AddMiddleware

// AddListener adds a listener to the channel for some endpoint
func (c *Client) AddListener(endpoint string, listener func([]byte) ([]byte, error)) error {
	// TODO: use context
	wrapper := func(_ context.Context, _ uuid.UUID, in []byte) ([]byte, error) {
		return listener(in)
	}
	return c.Channel.AddListener(endpoint, wrapper)
}

// SetHooks sets connection and disconnection hooks
func (c *Client) SetHooks(
	onConnect *func() error,
	onDisconnect *func(),
) {
	var wrapperOnConnect *func(uuid.UUID, string) error
	var wrapperOnDisconnect *func(uuid.UUID)
	if onConnect != nil {
		tmp := func(_ uuid.UUID, _ string) error {
			return (*onConnect)()
		}
		wrapperOnConnect = &tmp
	}
	if onDisconnect != nil {
		tmp := func(_ uuid.UUID) {
			(*onDisconnect)()
		}
		wrapperOnDisconnect = &tmp
	}
	c.Channel.SetHooks(wrapperOnConnect, wrapperOnDisconnect)
}

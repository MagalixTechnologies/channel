package channel

import (
	"log"
	"net/url"
	"time"

	uuid "github.com/MagalixTechnologies/uuid-go"
	"github.com/gorilla/websocket"
)

// Client a protocol client
type Client struct {
	Addr    string
	Channel *Channel

	server uuid.UUID
}

// NewClient Creates a new Client
// addr is the address to connect to
// channelOptions options, see ChannelOptions docs
func NewClient(addr string, channelOptions ChannelOptions) *Client {
	ch := newChannel(0, channelOptions)
	client := &Client{
		Addr:    addr,
		Channel: ch,
	}
	return client
}

// Listen starts connection loop to the server, auto connects when connection fails
func (c *Client) Listen() {
	u := url.URL{Scheme: "ws", Host: c.Addr, Path: "/"}
	go c.Channel.Init()
	for try := 0; ; try++ {
		dialer := websocket.Dialer{
			HandshakeTimeout: c.Channel.options.protoHandshake,
		}
		con, _, err := dialer.Dial(
			u.String(), nil,
		)
		if err != nil {
			log.Printf("failed to connect: %s", err)
			time.Sleep(c.Channel.options.protoReconnect)
			continue
		}
		defer con.Close()
		peer := newPeer(con, c.Channel)
		c.Channel.Peers[peer.ID] = peer
		c.server = peer.ID
		c.Channel.Peers[c.server].handle()
	}
}

// Send sends a byte array to the specified endpoint
// returns an optional response, an optional error
func (c *Client) Send(endpoint string, body []byte) ([]byte, error) {
	return c.Channel.Send(c.server, endpoint, body)
}

// IsConnected checks if a connection to the server is established
func (c *Client) IsConnected() bool {
	peer, ok := c.Channel.Peers[c.server]
	return ok && peer != nil
}

func (c *Client) AddListener(endpoint string, listener func(uuid.UUID, []byte) ([]byte, error)) error {
	return c.Channel.AddListener(endpoint, listener)
}

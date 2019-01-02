package channel

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
)

var (
	upgrader = websocket.Upgrader{}
)

// Server a protocol server
type Server struct {
	Addr string
	Path string

	Channel *Channel
}

// NewServer Creates a new Server
// addr is the address to listen on
// channelOptions options, see ChannelOptions docs
func NewServer(addr string, path string, channelOptions ChannelOptions) *Server {
	ch := newChannel(1, channelOptions)
	s := Server{
		Addr:    addr,
		Path:    path,
		Channel: ch,
	}
	http.HandleFunc(s.Path, s.serve)
	return &s
}

// Listen starts listening on connections
func (s *Server) Listen() {
	go s.Channel.Init()
	log.Print(http.ListenAndServe(s.Addr, nil))
}

// Send sends a byte array to some client on the specified endpoint
// returns an optional response, an optional error
func (s *Server) Send(client uuid.UUID, endpoint string, body []byte) ([]byte, error) {
	return s.Channel.Send(client, endpoint, body)
}

// IsConnected checks if the client is connected
func (s *Server) IsConnected(client uuid.UUID) bool {
	peer := s.Channel.GetPeer(client)
	return peer != nil
}

// AddListener adds a listener to the channel for some endpoint
func (s *Server) AddListener(endpoint string, listener func(context.Context, uuid.UUID, []byte) ([]byte, error)) error {
	return s.Channel.AddListener(endpoint, listener)
}

func (ch *Server) AddMiddleware(
	middleware func(context.Context, string, uuid.UUID, []byte, func(context.Context, uuid.UUID, []byte) ([]byte, error)) ([]byte, error),
) {
	ch.Channel.AddMiddleware(middleware)
}

// Peers returns a list of uuids of the connected clients
func (s *Server) Peers() []uuid.UUID {
	return s.Channel.ListPeers()
}

// SetHooks sets connection and disconnection hooks
func (s *Server) SetHooks(
	onConnect *func(id uuid.UUID, uri string, remoteAddr string) error,
	onDisconnect *func(id uuid.UUID),
) {
	s.Channel.SetHooks(onConnect, onDisconnect)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		msg := fmt.Sprintf("%s requested %s %s", r.RemoteAddr, r.Method, r.URL.String())
		log.Print("upgrade: ", err, msg)
		return
	}
	defer c.Close()
	peer := s.Channel.NewPeer(c, r.RequestURI)
	s.Channel.HandlePeer(peer)
}

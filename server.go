package channel

import (
	"log"
	"net/http"

	uuid "github.com/MagalixTechnologies/uuid-go"
)

// Server a protocol server
type Server struct {
	Addr    string
	Channel *Channel
}

// NewServer Creates a new Server
// addr is the address to listen on
// channelOptions options, see ChannelOptions docs
func NewServer(addr string, channelOptions ChannelOptions) *Server {
	ch := newChannel(1, channelOptions)
	http.HandleFunc("/", ch.serve)
	return &Server{
		Addr:    addr,
		Channel: ch,
	}
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
	peer, ok := s.Channel.Peers[client]
	return ok && peer != nil
}

// AddListener adds a listener to the channel for some endpoint
func (s *Server) AddListener(endpoint string, listener func(uuid.UUID, []byte) ([]byte, error)) error {
	return s.Channel.AddListener(endpoint, listener)
}

// Peers returns a list of uuids of the connected clients
func (s *Server) Peers() []uuid.UUID {
	peers := s.Channel.Peers
	res := make([]uuid.UUID, 0, len(peers))
	for u := range peers {
		res = append(res, u)
	}
	return res
}

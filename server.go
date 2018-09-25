package channel

import (
	"log"
	"net/http"

	"github.com/satori/go.uuid"
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

// SetHooks sets connection and disconnection hooks
func (s *Server) SetHooks(
	onConnect *func(id uuid.UUID, uri string) error,
	onDisconnect *func(id uuid.UUID),
) {
	s.Channel.SetHooks(onConnect, onDisconnect)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	peer := s.Channel.NewPeer(c, r.RequestURI)
	s.Channel.HandlePeer(peer)
}

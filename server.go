package channel

import (
	"log"
	"net/http"

	uuid "github.com/MagalixTechnologies/uuid-go"
)

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

func (s *Server) Send(client uuid.UUID, endpoint string, body []byte) ([]byte, error) {
	return s.Channel.Send(client, endpoint, body)
}

func (s *Server) IsConnected(client uuid.UUID) bool {
	peer, ok := s.Channel.Peers[client]
	return ok && peer != nil
}

func (s *Server) AddListener(endpoint string, listener func(uuid.UUID, []byte) ([]byte, error)) error {
	return s.Channel.AddListener(endpoint, listener)
}

func (s *Server) Peers() []uuid.UUID {
	peers := s.Channel.Peers
	res := make([]uuid.UUID, 0, len(peers))
	for u := range peers {
		res = append(res, u)
	}
	return res
}

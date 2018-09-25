package main

import (
	"errors"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/satori/go.uuid"

	"github.com/MagalixTechnologies/channel"
)

func main() {
	options := channel.ChannelOptions{
		ProtoHandshake: time.Second,
		ProtoWrite:     time.Second,
		ProtoRead:      2 * time.Second,
		ProtoReconnect: time.Second,
	}
	switch os.Args[1] {
	case "server":
		s := channel.NewServer("127.0.0.1:"+os.Args[2], options)
		s.AddListener("/", func(u uuid.UUID, body []byte) ([]byte, error) {
			spew.Dump("receiving req")
			ret := []byte(string(body) + string(body))
			return ret, nil
		})
		go func() {
			for {
				time.Sleep(10 * time.Second)
				for _, u := range s.Peers() {
					spew.Dump("receiving resp")
					spew.Dump(s.Send(u, "/mmm", []byte("asd124")))
				}
			}
		}()
		s.Listen()
	case "client":
		c := channel.NewClient("127.0.0.1:"+os.Args[2], options)
		c.AddListener("/mmm", func(u uuid.UUID, body []byte) ([]byte, error) {
			spew.Dump("receiving req", body)
			time.Sleep(1 * time.Second)
			return nil, errors.New("MIE says hi")
		})
		go func() {
			time.Sleep(time.Second)
			spew.Dump("receiving resp")
			spew.Dump(c.Send("/", []byte("asd")))
		}()
		c.Listen()
	}
}

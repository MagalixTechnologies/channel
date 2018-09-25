package main

import (
	"errors"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"

	"github.com/MagalixTechnologies/channel"
	uuid "github.com/MagalixTechnologies/uuid-go"
)

func main() {
	switch os.Args[1] {
	case "server":
		s := channel.NewServer("127.0.0.1:"+os.Args[2], channel.ChannelOptions{})
		s.Channel.AddListener("/", func(u uuid.UUID, body []byte) ([]byte, error) {
			spew.Dump("receiving req")
			ret := []byte(string(body) + string(body))
			return ret, nil
		})
		go func() {
			for {
				time.Sleep(10 * time.Second)
				for u := range s.Channel.Peers {
					spew.Dump("receiving resp")
					spew.Dump(s.Send(u, "/mmm", []byte("asd124")))
				}
			}
		}()
		s.Listen()
	case "client":
		c := channel.NewClient("127.0.0.1:"+os.Args[2], channel.ChannelOptions{})
		c.Channel.AddListener("/mmm", func(u uuid.UUID, body []byte) ([]byte, error) {
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

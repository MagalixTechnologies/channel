package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/MagalixTechnologies/uuid-go"
	"github.com/davecgh/go-spew/spew"

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
		s := channel.NewServer("127.0.0.1:"+os.Args[2], "/", options)
		onConnect := func(u uuid.UUID, url string, remoteAddr string) error {
			fmt.Printf("client connected %s to %s\n", u.String(), url)
			return nil
		}
		onDisconnect := func(u uuid.UUID) {
			fmt.Printf("client disconnected %s \n", u.String())
		}
		s.SetHooks(&onConnect, &onDisconnect)
		s.AddListener("/", func(c context.Context, u uuid.UUID, body []byte) ([]byte, error) {
			spew.Dump("receiving req")
			ret := []byte(string(body) + string(body))
			return ret, nil
		})
		go func() {
			for {
				time.Sleep(10 * time.Second)
				for _, u := range s.Peers() {
					spew.Dump("receiving resp")
					spew.Dump("peers", s.Peers())
					spew.Dump(s.Send(u, "/mmm", []byte("asd124")))
				}
			}
		}()
		s.Listen()
	case "client":
		url, _ := url.Parse("ws://127.0.0.1:" + os.Args[2] + "/?asd")
		c := channel.NewClient(*url, options)
		fn := func() error {
			fmt.Println("server connected")
			return nil
		}
		c.SetHooks(&fn, nil)
		c.AddListener("/mmm", func(body []byte) ([]byte, error) {
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

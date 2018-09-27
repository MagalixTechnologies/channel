# Channel

A library that implements bidirectional request/response based communication over websockets.

## Public apis

see [client](./client.go) and [server](./server.go)

## Usage

see [example](./example/main.go)

## Implementation Details

### Channel

The server and client are wrappers around `channel`. `channel` is an abstraction that can send requests from one side to another and wait for the response. Channels have connected peers, in case of the client it is only one peer that is actually the server.

Each request to a peer gets a unique id, and the response to this request uses the same id. Requests from the client uses even ids, while requests from the server uses odd ones. This way each channel can distinguish between requests from its peer and responses to previous requests.

### Packets

This protocol encapsulate each paket into this struct

```go
type packetStruct struct {
	ID       int            `json:"id"`
	Endpoint string         `json:"endpoint"`
	Body     []byte         `json:"body,omitempty"`
	Error    *ProtocolError `json:"error,omitempty"`
}
```

It uses endpoints to allow communication for different reasons, each request sent to some endpoint will be processed by the listener listening on this endpoint.

Body is the body to be sent over to the peer as a byte array. It is done this way to add the flexibility for the user to choose how he wants to encode his data.

Errro an optional error in case the listener returns one.

#### Packet Encoding

Packets are encoded using `gob`, the underlying data can be encoded however the user likes it should be passed as `[]byte` to the apis.

### Errors

Responses are awaited for for some time, if it didn't receive one, it just returns a timeout error, if a response is received afterwards, it is silently discarded. Also malformed requets give an error immediately. Both these are protocol errors.

For listeners that return errors that are not protocol errors, the error is wrapped as an internal error and the message of the error is trasnsferred to the peer.

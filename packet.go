package channel

type packetStruct struct {
	ID       int            `json:"id"`
	Endpoint string         `json:"endpoint"`
	Body     []byte         `json:"body,omitempty"`
	Error    *ProtocolError `json:"error,omitempty"`
}

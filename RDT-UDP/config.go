package main

const (
	MSS = 512
	MaxSequenceNumber = 102400
)

const (
	ServerIP = "127.0.0.1"
	ServerPort = 3456
)

type Segment struct {
	Seq          uint32    // Sequence Number
	Ack          uint32    // Acknowledgment Number
	ConnectionID uint16    // Connection Identifier
	Flag         uint16    // Flag Field:13 unused bit and ACK SYN FIN
	Data         []byte // Data Field
}

const (
	ACK = 0x01 << 1
	SYN = 0x02 << 2
	FIN = 0x03 << 3
)


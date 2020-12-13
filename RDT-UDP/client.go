package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
)

var clientId int
var file []byte

type TCPClient struct {
	WINSize uint32
	SyncSeq uint32
	NextSeq uint32

	ConnectionId uint16
	Packet       Segment

	synCh chan Segment
	dieCh chan bool
}

/*
 * Initial some variables for client
 */
func (c *TCPClient) initialize() {
	c.WINSize = 32 * MSS
	c.SyncSeq = 0
	c.NextSeq = c.SyncSeq
	c.ConnectionId = uint16(clientId)

	c.synCh = make(chan Segment, 1)
	c.dieCh = make(chan bool, 1)
}

/*
 * Send packet to server
 */
func (c *TCPClient) send(file []byte) {
	// build a connection with server
	rAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:"+strconv.Itoa(ServerPort))
	conn, err := net.DialUDP("udp", nil, rAddr)
	if err != nil {
		log.Println("Some error", err)
		return
	}
	defer conn.Close()

	lAddr := conn.LocalAddr().String()
	fmt.Println("Client's local address:", lAddr)

	// three-way handshake
	c.sendSYN(conn, c.SyncSeq)
	// listen reply
	go c.listen(conn)

	for {
		select {
		case rPacket := <-c.synCh:
			fmt.Println("Connection Success", rPacket.Ack)
		}
	}

}

func (c *TCPClient) sendSYN(conn *net.UDPConn, seq uint32) {
	var packet = Segment{
		Seq:          seq,
		Ack:          0,
		ConnectionID: c.ConnectionId,
		Flag:         SYN,
		Data:         []byte{},
	}

	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(packet)
	if err != nil {
		log.Println("Some error", err)
	}

	_, err = conn.Write(buffer.Bytes())
	if err != nil {
		log.Println("Some error", err)
	}

}

/*
 *
 */
func (c *TCPClient) listen(conn *net.UDPConn) {
	p := make([]byte, 512)
	for {
		_, _, err := conn.ReadFromUDP(p)
		if err != nil {
			log.Println("Some error", err)
		}

		var packet Segment
		buffer := bytes.NewBuffer(p)
		decoder := gob.NewDecoder(buffer)
		err = decoder.Decode(&packet)
		if err != nil {
			log.Println("Decoder error", err)
			continue
		}

		if packet.Flag&SYN != 0 {
			c.synCh <- packet
		}
		if packet.Flag&FIN != 0 {
			c.dieCh <- true
		}
	}
}

/*
 * Get file's content with a format of []byte
 */
func loadFile(filename string) []byte {
	file, err := os.Open(filename)
	if err != nil {
		log.Println("Some error", err)
	}
	defer file.Close()
	var fileBuf bytes.Buffer

	_, err = io.Copy(&fileBuf, file)
	if err != nil {
		log.Println("Some error", err)
	}

	fileBytes := fileBuf.Bytes()
	fileSize := len(fileBytes)
	fmt.Println("Load file", filename, ", total bytes are", fileSize)
	return fileBytes
}

func main() {
	argsNum := len(os.Args)
	if argsNum <= 1 {
		fmt.Println("Usage: go run client.go config.go [clientId] [filename]")
		return
	}

	clientId, _ = strconv.Atoi(os.Args[1])
	// load the file
	file = loadFile(os.Args[2])

	var c TCPClient
	// initialization
	c.initialize()

	go c.send(file)
	for {
		select {
		case <-c.dieCh:
			return
		}
	}
}

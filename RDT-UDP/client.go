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
	"sync"
	"time"
)

var clientId int
var fileData []byte
var fileSize uint32
var cReturnCh chan bool
var timer bool
var sendAll bool

type TCPClient struct {
	State   int
	WINSize uint32
	BaseSeq uint32
	NextSeq uint32
	SendAck uint32 // send ack number to server
	Timer   time.Time

	ConnectionId uint16
	Packet       Segment

	Buf       Queue
	synCh     chan Segment
	dataCh    chan uint32
	timeOutCh chan bool
	finCh     chan bool
	dieCh     chan bool
	mu        sync.Mutex
}

/*
 * Initial some variables for client
 */
func (c *TCPClient) initialize() {
	c.mu.Lock()
	c.State = Establish
	c.WINSize = 32 * MSS
	c.BaseSeq = 0
	c.NextSeq = c.BaseSeq
	c.SendAck = 0

	c.ConnectionId = uint16(clientId)

	c.synCh = make(chan Segment, 1)
	c.timeOutCh = make(chan bool, 1)
	c.dieCh = make(chan bool, 1)
	c.finCh = make(chan bool, 1)
	c.dataCh = make(chan uint32, 1)
	c.mu.Unlock()
}

/*
 * Send packet to server
 */
func (c *TCPClient) send(conn *net.UDPConn) {
	// three-way handshake
	err := c.sendSYN(conn, c.BaseSeq)
	c.mu.Lock()
	c.Timer = time.Now()
	c.mu.Unlock()
	go c.checkTimer()

	if err != nil {
		log.Println("Some error", err)
		return
	}
	Rtt0 := time.Now().UnixNano()

	for {
		select {
		case rPacket := <-c.synCh:
			Rtt1 := time.Now().UnixNano()
			fmt.Println("RTT:", float64(Rtt1-Rtt0)/1e6, "ms")
			c.mu.Lock()
			c.State = Maintain
			c.BaseSeq = rPacket.Ack
			c.NextSeq = c.BaseSeq
			c.SendAck = rPacket.Seq + 1
			c.dataCh <- c.NextSeq
			c.mu.Unlock()
		case nextSend := <-c.dataCh:
			if nextSend > fileSize {
				sendAll = true
				continue
			}
			c.mu.Lock()
			c.NextSeq = nextSend
			if c.NextSeq >= c.BaseSeq+c.WINSize {
				// if next sequence exceed the window size, refuse data
				fmt.Println("overload")
				c.mu.Unlock()
				continue
			}
			err = c.sendData(conn, c.SendAck, c.NextSeq)
			if err != nil {
				log.Println("Some error", err)
			}
			c.mu.Unlock()
		case <-c.timeOutCh:
			c.mu.Lock()
			c.Timer = time.Now()
			c.dataCh <- c.BaseSeq
			c.mu.Unlock()
			fmt.Println("test")
		case <-c.finCh:
			c.mu.Lock()
			c.State = Release
			c.Timer = time.Now()
			go c.checkTimer()
			err = c.sendFIN(conn, c.SendAck, c.NextSeq)
			if err != nil {
				log.Println("Some error", err)
			}
			c.mu.Unlock()
		case <-c.dieCh:
			c.mu.Lock()
			err = c.sendLastACK(conn, c.SendAck, c.NextSeq)
			if err != nil {
				log.Println("Some error", err)
			}
			c.mu.Unlock()
			cReturnCh <- true
			return
		}
	}

}

func (c *TCPClient) sendSYN(conn *net.UDPConn, seq uint32) error {
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
		return err
	}

	_, err = conn.Write(buffer.Bytes())
	if err != nil {
		return err
	}
	return nil
}

/*
 *
 */
func (c *TCPClient) sendData(conn *net.UDPConn, ack uint32, seq uint32) error {
	var chunkEnd = seq - 1 + 512
	if chunkEnd > fileSize {
		chunkEnd = fileSize
	}
	var sendData = fileData[seq-1 : chunkEnd]
	sPacket := Segment{
		Seq:          seq,
		Ack:          ack,
		ConnectionID: c.ConnectionId,
		Flag:         ACK,
		Data:         sendData,
	}
	//showSegment(sPacket)
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(sPacket)
	if err != nil {
		return err
	}
	_, err = conn.Write(buffer.Bytes())
	if err != nil {
		return err
	}
	if c.BaseSeq == c.NextSeq {
		c.Timer = time.Now()
	}
	c.NextSeq = chunkEnd + 1
	c.dataCh <- c.NextSeq
	return nil
}

func (c *TCPClient) sendFIN(conn *net.UDPConn, ack uint32, seq uint32) error {
	fmt.Println("Start to disconnect")
	var packet = Segment{
		Seq:          seq,
		Ack:          ack,
		ConnectionID: c.ConnectionId,
		Flag:         FIN,
		Data:         []byte{},
	}

	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(packet)
	if err != nil {
		return err
	}

	_, err = conn.Write(buffer.Bytes())
	if err != nil {
		return err
	}
	return nil
}

func (c *TCPClient) sendLastACK(conn *net.UDPConn, ack uint32, seq uint32) error {
	fmt.Println("Send last ACK to disconnect")
	var packet = Segment{
		Seq:          seq,
		Ack:          ack,
		ConnectionID: c.ConnectionId,
		Flag:         ACK,
		Data:         []byte{},
	}

	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(packet)
	if err != nil {
		return err
	}

	_, err = conn.Write(buffer.Bytes())
	if err != nil {
		return err
	}
	return nil
}
func (c *TCPClient) checkTimer() {
	timer = true
	for {
		if !timer {
			return
		}
		c.mu.Lock()
		if time.Now().UnixNano()-c.Timer.UnixNano() > TimeOut.Nanoseconds() {
			if c.State == Establish {
				log.Println("Time out during establishment,please try again")
				c.dieCh <- true
				return
			}
			if c.State == Maintain {
				log.Println("Time out during maintaining")
				c.timeOutCh <- true
			}
			if c.State == Release {
				log.Println("Time out during releasing")
			}
		}
		c.mu.Unlock()
		time.Sleep(1 * time.Millisecond)
	}
}

/*
 *
 */
func (c *TCPClient) listen(conn *net.UDPConn) {
	p := make([]byte, 1024)
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

		if packet.Flag&SYN != 0 && packet.Ack == c.BaseSeq+1 {
			c.mu.Lock()
			c.synCh <- packet
			c.mu.Unlock()
		}
		if packet.Flag == ACK {
			c.mu.Lock()
			fmt.Println("Client's state", c.State)
			c.SendAck = packet.Seq + 1
			if c.State == Maintain {
				fmt.Println("get a ack", packet.Ack,"and base sequence number:",c.BaseSeq)
				if packet.Ack > c.BaseSeq {
					c.BaseSeq = packet.Ack
				}
				if c.BaseSeq == c.NextSeq && sendAll == true {
					fmt.Println("base seq:", c.BaseSeq, "next seq", c.NextSeq)
					//stop timer
					fmt.Println("Transmission finish")
					c.finCh<-true
					timer = false
				} else {
					c.Timer = time.Now()
				}
			} else if c.State == Release {
				if packet.Ack > c.BaseSeq {
					// disconnect successfully
					fmt.Println("get a disconnection ack",packet.Ack)
					timer = false
				}
			}
			c.mu.Unlock()
		}
		if packet.Flag == FIN {
			c.mu.Lock()
			c.SendAck = packet.Seq + 1
			c.dieCh <- true
			c.mu.Unlock()
			return
		}
	}
}

/*
 * Get file's content with a format of []byte
 */
func loadFile(filename string) ([]byte, uint32) {
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
	fileSize := uint32(len(fileBytes))
	fmt.Println("Load file", filename, ", total bytes are", fileSize)
	return fileBytes, fileSize
}

/*
 *
 */
func showSegment(packet Segment) {
	fmt.Println("Seq:", packet.Seq)
	fmt.Println("Ack:", packet.Ack)
	fmt.Println("Data:", string(packet.Data))
}

func main() {
	argsNum := len(os.Args)
	if argsNum <= 1 {
		fmt.Println("Usage: go run client.go config.go [clientId] [filename]")
		return
	}

	clientId, _ = strconv.Atoi(os.Args[1])
	// load the file
	fileData, fileSize = loadFile(os.Args[2])

	var c TCPClient
	// initialization
	c.initialize()
	cReturnCh = make(chan bool, 1)
	sendAll = false
	// construct a connection
	rAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:"+strconv.Itoa(ServerPort))
	conn, err := net.DialUDP("udp", nil, rAddr)
	if err != nil {
		log.Println("Some error", err)
		return
	}
	defer conn.Close()
	// get client's local address
	lAddr := conn.LocalAddr().String()
	fmt.Println("Client's local address:", lAddr)
	// send segment
	go c.send(conn)
	go c.listen(conn)

	<-cReturnCh
	return
}

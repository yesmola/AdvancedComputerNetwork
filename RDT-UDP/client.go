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

var clientId int           // Client's Id
var fileData []byte        // File data
var fileSize uint32        // File size
var cReturnCh chan bool    // close the program
var timer bool             // If true ,then close the timer
var timerLock sync.Mutex   // Global variable's lock
var sendAll bool           // True when the whole file has been sent
var sendAllLock sync.Mutex // Global variable's lock
var sendCnt uint32         // Record the times that send a data packet, use to calculate the good put

type TCPClient struct {
	State           int             // Client's connection state: Establish, Maintain or Release
	CongestionState int             // Client's congestion state: SlowStart or CongestionAvoidance
	WINSize         uint32          // Sender's window size
	BaseSeq         uint32          // Base sequence number
	NextSeq         uint32          // Next sequence number to send
	SendAck         uint32          // Send ack number to server
	CWND            uint32          // Congestion window size
	SSThresh        uint32          // Slow Start threshold
	Timer           time.Time       // A timer and ONLY a timer
	Buf             Queue           // Receive buffer
	AckBuffer       map[uint32]bool // Use ack number to check if the ack has been buffered
	ConnectionId    uint16          // Client's Id

	synCh     chan Segment // sendSYN channel
	dataCh    chan uint32  // sendData channel
	timeOutCh chan uint32  // timeOut channel
	finCh     chan bool    // sendFin channel
	dieCh     chan bool    // sendLastACK channel
	mu        sync.Mutex
}

/*
 * Initial some variables for client
 */
func (c *TCPClient) initialize() {
	c.mu.Lock()
	c.State = Establish
	c.WINSize = BufSize * MSS
	c.CongestionState = SlowStart
	c.CWND = MSS
	c.SSThresh = InitialSSThresh
	c.BaseSeq = 0
	c.NextSeq = c.BaseSeq
	c.SendAck = 0
	c.Buf = *GetQueue()
	c.AckBuffer = make(map[uint32]bool, BufSize)
	c.ConnectionId = uint16(clientId)

	c.synCh = make(chan Segment, 1)
	c.timeOutCh = make(chan uint32, 1)
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
	if err != nil {
		log.Println("Some error", err)
		return
	}

	c.mu.Lock()
	c.Timer = time.Now()
	c.mu.Unlock()
	// start the timer
	go c.checkTimer()

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
				sendAllLock.Lock()
				sendAll = true
				sendAllLock.Unlock()
				continue
			}
			c.mu.Lock()
			c.NextSeq = nextSend
			if uint32(c.Buf.Len())*MSS >= minWin(c.WINSize, c.CWND) {
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
			// Adjust the CWND and SSThresh
			c.SSThresh = c.CWND / 2
			c.CongestionState = SlowStart
			c.CWND = MSS
			err = c.resend(conn)
			c.mu.Unlock()

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

/*
 * Send a SYN packet
 */
func (c *TCPClient) sendSYN(conn *net.UDPConn, seq uint32) error {
	var packet = Segment{
		Seq:          seq,
		Ack:          0,
		ConnectionID: c.ConnectionId,
		Flag:         SYN,
		Data:         []byte{},
	}
	c.Buf.Push(packet)
	c.AckBuffer[seq+1] = false

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
 * Send a data packet
 */
func (c *TCPClient) sendData(conn *net.UDPConn, ack uint32, seq uint32) error {
	fmt.Println("CWND:", c.CWND, "Congestion State", c.CongestionState, "SSThresh", c.SSThresh)
	sendCnt++
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
	c.Buf.Push(sPacket)
	c.AckBuffer[sPacket.Seq+uint32(len(sendData))] = false
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

/*
 * Only one timer, resend the first packet (maybe should be all the packet, but there is a bug) sent but not acknowledged
 */
func (c *TCPClient) resend(conn *net.UDPConn) error {
	sendCnt++
	// Go through the buffer
	for item := c.Buf.list.Front(); item != nil; item = item.Next() {
		sPacket := item.Value.(Segment)
		// check the ack buffer
		if !c.AckBuffer[sPacket.Seq+uint32(len(sPacket.Data))] {
			fmt.Println("resend a seq", sPacket.Seq)
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
			// just send the first one
			break
		}
	}
	return nil
}

/*
 * Send a FIN packet
 */
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

/*
 * Send the last ACK packet
 */
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

/*
 * timer
 */
func (c *TCPClient) checkTimer() {
	timerLock.Lock()
	timer = true
	timerLock.Unlock()
	for {
		timerLock.Lock()
		if !timer {
			return
		}
		timerLock.Unlock()
		c.mu.Lock()
		var state = c.State
		if time.Now().UnixNano()-c.Timer.UnixNano() > TimeOut.Nanoseconds() {
			if state == Establish {
				log.Println("Time out during establishment,please try again")
				c.dieCh <- true
			}
			if state == Maintain {
				log.Println("Time out during maintaining", c.BaseSeq)
				c.Timer = time.Now()
				c.timeOutCh <- 1
			}
			if state == Release {
				log.Println("Time out during releasing")
			}
		}
		c.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
}

/*
 * Listen the packet back from server
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
			fmt.Println("get a connection ack", packet.Ack)
			_ = c.Buf.Pop()
			delete(c.AckBuffer, packet.Ack)
			c.synCh <- packet
			c.mu.Unlock()
		}
		if packet.Flag == ACK {
			c.mu.Lock()
			//fmt.Println("Client's state", c.State)
			c.SendAck = packet.Seq + 1
			c.mu.Unlock()

			// Adjust the CWND
			c.mu.Lock()
			if c.CongestionState == SlowStart {
				c.CWND += MSS
			}
			if c.CWND >= c.SSThresh {
				c.CongestionState = CongestionAvoidance
			}
			c.mu.Unlock()
			if c.State == Maintain {
				c.mu.Lock()
				front := c.Buf.Front().(Segment)
				//fmt.Println("get a ack", packet.Ack, "and front sequence number:", front.Seq)
				if packet.Ack == front.Seq+uint32(len(front.Data)) {
					c.BaseSeq = packet.Ack
					_ = c.Buf.Pop()
					delete(c.AckBuffer, packet.Ack)
					// move the window
					for c.Buf.Len() > 0 {
						front = c.Buf.Front().(Segment)
						if c.AckBuffer[front.Seq+uint32(len(front.Data))] == true {
							c.BaseSeq = front.Seq + uint32(len(front.Data))
							_ = c.Buf.Pop()
							delete(c.AckBuffer, front.Seq+uint32(len(front.Data)))
						} else {
							break
						}
					}
				} else {
					// Buffer the ack
					//fmt.Println("buffer a ack", packet.Ack, "and buffer size is", len(c.AckBuffer))
					c.AckBuffer[packet.Ack] = true
				}
				var flag bool
				sendAllLock.Lock()
				flag = sendAll
				sendAllLock.Unlock()
				if c.BaseSeq == c.NextSeq {
					//fmt.Println("base seq:", c.BaseSeq, "next seq", c.NextSeq)
					if flag {
						fmt.Println("Transmission finish")
						// Stop timer
						c.finCh <- true
						timerLock.Lock()
						timer = false
						timerLock.Unlock()
					} else {
						if c.CongestionState == CongestionAvoidance {
							c.CWND += MSS
						}
						c.dataCh <- c.NextSeq
					}
				} else {
					c.Timer = time.Now()
				}
				c.mu.Unlock()
			} else if c.State == Release {
				c.mu.Lock()
				if packet.Ack > c.BaseSeq {
					// Disconnect successfully
					fmt.Println("get a disconnection ack", packet.Ack)
					timerLock.Lock()
					timer = false
					timerLock.Unlock()
				}
				c.mu.Unlock()
			}

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
 * Print a Segment
 */
func showSegment(packet Segment) {
	fmt.Println("Seq:", packet.Seq)
	fmt.Println("Ack:", packet.Ack)
	fmt.Println("Data:", string(packet.Data))
}

/*
 * Compare two uint32
 */
func minWin(a, b uint32) uint32 {
	if a > b {
		return b
	}
	return a
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
	var chunkCnt uint32
	if fileSize%512 == 0 {
		chunkCnt = fileSize / 512
	} else {
		chunkCnt = fileSize/512 + 1
	}
	var sendTime int64
	sendTime = time.Now().UnixNano() / 1e6

	var c TCPClient
	// Initialization
	c.initialize()
	cReturnCh = make(chan bool, 1)
	sendAll = false
	sendCnt = 0
	// construct a connection
	rAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:"+strconv.Itoa(ServerPort))
	conn, err := net.DialUDP("udp", nil, rAddr)
	if err != nil {
		log.Println("Some error", err)
		return
	}
	defer conn.Close()
	// Get client's local address
	lAddr := conn.LocalAddr().String()
	fmt.Println("Client's local address:", lAddr)
	// Send segment
	go c.send(conn)
	go c.listen(conn)

	<-cReturnCh
	log.Println("Send start at:", sendTime)
	log.Println("chunk:", chunkCnt, "send:", sendCnt, "Good put:", float64(chunkCnt)/float64(sendCnt))
	return
}

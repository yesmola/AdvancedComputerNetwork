package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"
)

type Client struct {
}

type TCPServer struct {
	IP        string // Server's IP address
	Port      int    // Server's port
	NextSeq   uint32 // Next sequence number received
	ExpectSeq uint32 // Sequence number expected
	WINSize   uint32 // Receive window size

	Buf       Queue           // Receive buffer
	SeqBuffer map[uint32]bool // Save sequence numbers received
	mu        sync.Mutex
	Clients   []Client // TODO
}

var rAddr *net.UDPAddr  // Remote address
var rcvFile []byte      // Save file data received
var sReturnCh chan bool // close the program

/*
 * Initial some variables for server
 */
func (s *TCPServer) Initialize(IP string, port int) {
	s.mu.Lock()
	s.IP = IP
	s.Port = port
	s.NextSeq = 0
	s.ExpectSeq = 0
	s.WINSize = BufSize * MSS
	s.Buf = *GetQueue()
	s.SeqBuffer = make(map[uint32]bool, BufSize)
	s.mu.Unlock()
}

/*
 * Keep listening from clients
 */
func (s *TCPServer) Listen(ser *net.UDPConn) {
	// Loop for listen
	p := make([]byte, 1024)
	for {
		_, remoteAddr, err := ser.ReadFromUDP(p)
		rAddr = remoteAddr
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
		// Receive a connection request
		if packet.Flag&SYN != 0 {
			if packet.Data == nil {
				fmt.Println("receive a connection request from", remoteAddr)
			} else {
				fmt.Println("Warning: Receive a connection request from", remoteAddr, "and the data is not null")
			}
			s.mu.Lock()
			err = s.sendSYN(ser, packet)
			if err != nil {
				log.Println("Some error", err)
			}
			s.mu.Unlock()
		}
		// Receive a data packet
		if packet.Flag == ACK && packet.Data != nil {
			// fmt.Println("receive a data packet from", remoteAddr)
			fmt.Println("expected seq:", s.ExpectSeq, "received seq:", packet.Seq)
			// Use random number to make a dropped packet
			rand.Seed(time.Now().UnixNano())
			if rand.Intn(100) > 90 {
				fmt.Println("drop the packet")
				continue
			}
			// if packet is the next packet server want to get
			if packet.Seq == s.ExpectSeq {
				rcvFile = append(rcvFile, packet.Data[:]...)
				s.mu.Lock()
				// modify the expect sequence number
				s.ExpectSeq = packet.Seq + uint32(len(packet.Data))
				err = s.sendACK(ser, s.NextSeq, s.ExpectSeq, packet.ConnectionID)
				if err != nil {
					log.Println("Some error", err)
				}
				s.NextSeq++
				// Read from buffer
				if s.Buf.Len() > 0 {
					front := s.Buf.Front().(Segment)
					for front.Seq == s.ExpectSeq {
						fmt.Println("read from buffer", front.Seq)
						/*for item := s.Buf.list.Front(); item != nil; item = item.Next() {
							fmt.Println("buffer:",item.Value.(Segment).Seq)
						}*/
						rcvFile = append(rcvFile, front.Data[:]...)
						s.ExpectSeq = front.Seq + uint32(len(front.Data))
						err = s.Buf.Pop()
						delete(s.SeqBuffer, front.Seq)
						if err != nil {
							log.Println("Some error", err)
						}
						if s.Buf.Len() == 0 {
							break
						}
						front = s.Buf.Front().(Segment)
					}
				}
				s.mu.Unlock()
			} else if packet.Seq > s.ExpectSeq {
				// Buffer the packet
				s.mu.Lock()
				if uint32(s.Buf.Len())*MSS > s.WINSize {
					// refuse the packet
				} else if !s.SeqBuffer[packet.Seq] {
					err = s.sendACK(ser, s.NextSeq, packet.Seq+uint32(len(packet.Data)), packet.ConnectionID)
					if err != nil {
						log.Println("Some error", err)
					}
					fmt.Println("buffer a seq", packet.Seq)
					s.SeqBuffer[packet.Seq] = true
					s.Buf.Push(packet)
				}
				s.mu.Unlock()
			}
		}
		// A ack packet for disconnection
		if packet.Flag == ACK && packet.Data == nil {
			fmt.Println("disconnect successfully")
			err = writeFile("out.txt", rcvFile, 0666)
			if err != nil {
				log.Println("Some Error", err)
			}
			sReturnCh <- true
			return
		}
		// A fin packet
		if packet.Flag == FIN {
			log.Println("receive a disconnection request from", remoteAddr)
			s.mu.Lock()
			s.ExpectSeq = packet.Seq + 1
			err = s.sendACK(ser, s.NextSeq, s.ExpectSeq, packet.ConnectionID)
			if err != nil {
				log.Println("Some error", err)
			}
			s.NextSeq++
			err = s.sendFIN(ser, packet)
			if err != nil {
				log.Println("Some error", err)
			}
			s.mu.Unlock()
		}
	}
}

/*
 * Send a SYN packet
 */
func (s *TCPServer) sendSYN(ser *net.UDPConn, rPacket Segment) error {
	fmt.Println("Server sends a SYN")
	sPacket := Segment{
		Seq:          s.NextSeq,
		Ack:          rPacket.Seq + 1,
		ConnectionID: rPacket.ConnectionID,
		Flag:         SYN | ACK,
		Data:         []byte{},
	}
	s.NextSeq++
	s.ExpectSeq = rPacket.Seq + 1
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(sPacket)
	if err != nil {
		return err
	}

	_, err = ser.WriteTo(buffer.Bytes(), rAddr)
	if err != nil {
		return err
	}
	return nil
}

/*
 * Send a ACK packet
 */
func (s *TCPServer) sendACK(ser *net.UDPConn, seq uint32, ack uint32, connectionId uint16) error {
	sPacket := Segment{
		Seq:          seq,
		Ack:          ack,
		ConnectionID: connectionId,
		Flag:         ACK,
		Data:         []byte{},
	}
	fmt.Println("Server sends a ACK", ack)
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(sPacket)
	if err != nil {
		return err
	}

	_, err = ser.WriteTo(buffer.Bytes(), rAddr)
	if err != nil {
		return err
	}
	return nil
}

/*
 * Send a FIN packet
 */
func (s *TCPServer) sendFIN(ser *net.UDPConn, rPacket Segment) error {
	fmt.Println("Server sends a FIN")
	sPacket := Segment{
		Seq:          s.NextSeq,
		Ack:          rPacket.Seq + 1,
		ConnectionID: rPacket.ConnectionID,
		Flag:         FIN,
		Data:         []byte{},
	}
	s.NextSeq++
	s.ExpectSeq = rPacket.Seq + 1
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(sPacket)
	if err != nil {
		return err
	}

	_, err = ser.WriteTo(buffer.Bytes(), rAddr)
	if err != nil {
		return err
	}
	return nil
}

/*
 * Save received data to a local file
 */
func writeFile(filename string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	n, err := f.Write(data)
	if err == nil && n < len(data) {
		err = io.ErrShortWrite
	}
	if err1 := f.Close(); err == nil {
		err = err1
	}
	return err
}

func main() {
	IP := ServerIP
	port := ServerPort

	var s TCPServer
	// Initialization
	s.Initialize(IP, port)
	sReturnCh = make(chan bool, 1)
	// define local address
	addr := net.UDPAddr{
		Port: s.Port,
		IP:   net.ParseIP(s.IP),
	}
	// build a PacketConn
	ser, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Println(err)
		return
	}
	defer ser.Close()

	go s.Listen(ser)

	<-sReturnCh
	log.Println("Receive end at:", time.Now().UnixNano()/1e6)
	return
}

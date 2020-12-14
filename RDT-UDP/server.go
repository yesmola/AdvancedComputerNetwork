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
	IP        string
	Port      int
	NextSeq   uint32
	ExpectSeq uint32

	Packet  Segment
	mu      sync.Mutex
	Clients []Client
}

var rAddr *net.UDPAddr
var rcvFile []byte
var sReturnCh chan bool

/*
 * Initial some variables for server
 */
func (s *TCPServer) Initialize(IP string, port int) {
	s.mu.Lock()
	s.IP = IP
	s.Port = port
	s.NextSeq = 0
	s.ExpectSeq = 0
	s.mu.Unlock()
}

/*
 * Keep listening from clients
 */
func (s *TCPServer) Listen(ser *net.UDPConn) {
	// loop for listen
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
		// SYN = 1
		if packet.Flag&SYN != 0 {
			if packet.Data == nil {
				fmt.Println("receive a connection request from", remoteAddr)
			} else {
				fmt.Println("Warning: Receive a connection request from", remoteAddr, "and the data is not null")
			}
			s.mu.Lock()
			s.sendSYN(ser, packet)
			s.mu.Unlock()
		}
		if packet.Flag == ACK && packet.Data != nil {
			fmt.Println("receive a data packet from", remoteAddr)
			fmt.Println("expected seq:", s.ExpectSeq, "received seq:", packet.Seq)
			rand.Seed(time.Now().UnixNano())
			if rand.Intn(100) > 95 {
				fmt.Println("drop the packet")
				continue
			}
			s.mu.Lock()
			if packet.Seq == s.ExpectSeq {
				s.sendACK(ser, packet)
			}
			s.mu.Unlock()
		}
		if packet.Flag == ACK && packet.Data == nil {
			fmt.Println("disconnect successfully")
			err = writeFile("out.txt", rcvFile, 0666)
			if err != nil {
				log.Println("Some Error", err)
			}
			sReturnCh <- true
			return
		}
		if packet.Flag == FIN {
			log.Println("receive a disconnection request from", remoteAddr)
			s.mu.Lock()
			s.sendACK(ser, packet)
			s.sendFIN(ser, packet)
			s.mu.Unlock()
		}
	}
}

func (s *TCPServer) sendSYN(ser *net.UDPConn, rPacket Segment) {
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
		log.Println("Some error", err)
	}

	_, err = ser.WriteTo(buffer.Bytes(), rAddr)
	if err != nil {
		log.Println("Some error", err)
	}
}

func (s *TCPServer) sendACK(ser *net.UDPConn, rPacket Segment) {
	rcvFile = append(rcvFile, rPacket.Data[:]...)
	if uint32(len(rPacket.Data)) == 0 {
		s.ExpectSeq = rPacket.Seq + 1
	} else {
		s.ExpectSeq = rPacket.Seq + uint32(len(rPacket.Data))
	}
	sPacket := Segment{
		Seq:          s.NextSeq,
		Ack:          s.ExpectSeq,
		ConnectionID: rPacket.ConnectionID,
		Flag:         ACK,
		Data:         []byte{},
	}
	s.NextSeq++
	fmt.Println("Server sends a ACK", s.ExpectSeq)
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(sPacket)
	if err != nil {
		log.Println("Some error", err)
	}

	_, err = ser.WriteTo(buffer.Bytes(), rAddr)
	if err != nil {
		log.Println("Some error", err)
	}
}

func (s *TCPServer) sendFIN(ser *net.UDPConn, rPacket Segment) {
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
		log.Println("Some error", err)
	}

	_, err = ser.WriteTo(buffer.Bytes(), rAddr)
	if err != nil {
		log.Println("Some error", err)
	}
}

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
	return
}

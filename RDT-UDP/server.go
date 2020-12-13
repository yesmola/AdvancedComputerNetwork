package main

import (
	"bytes"
	"encoding/gob"
	"log"
	"net"
)

type Client struct {
}

type TCPServer struct {
	IP   string
	Port int

	Packet  Segment
	Clients []Client

	synCh chan Segment
	adrCh chan *net.UDPAddr
}

var rAddr *net.UDPAddr

/*
 * Initial some variables for server
 */
func (s *TCPServer) Initialize(IP string, port int) {
	s.IP = IP
	s.Port = port
	s.synCh = make(chan Segment, 1)
	s.adrCh = make(chan *net.UDPAddr, 1)
}

/*
 * Keep listening from clients
 */
func (s *TCPServer) Listen(ser *net.UDPConn) {
	// loop for listen
	p := make([]byte, 512)
	for {
		_, remoteAddr, err := ser.ReadFromUDP(p)
		s.adrCh <- remoteAddr
		if err != nil {
			log.Println("Some error", err)
		}
		log.Println("receive a packet from", remoteAddr)

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
				log.Println("Receive a connection request")
			} else {
				log.Println("Warning: Receive a connection request and the data is not null")
			}
			s.synCh <- packet
		}

	}
}

func (s *TCPServer) sendSYN(ser *net.UDPConn, rPacket Segment) {
	sPacket := Segment{
		Seq:          0,
		Ack:          rPacket.Seq + 1,
		ConnectionID: rPacket.ConnectionID,
		Flag:         SYN | ACK,
		Data:         []byte{},
	}

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

func main() {
	IP := ServerIP
	port := ServerPort

	var s TCPServer
	// Initialization
	s.Initialize(IP, port)

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
	for {
		select {
		case rAddr = <-s.adrCh:
			log.Println("Remote address:", rAddr)
		case rPacket := <-s.synCh:
			s.sendSYN(ser, rPacket)
		}
	}
}

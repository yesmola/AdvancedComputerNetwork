package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"
)

const Unreachable = 16

type TableValue struct {
	Dist  int
	Route []string
}

type Router struct {
	Id          string "Router's id"
	Port        int    "Router's port"
	Neighbour   []int  "neighbour's port"
	RouterTable map[int]TableValue
}

func (r *Router) print() {
	fmt.Println("Id: ",r.Id)
	fmt.Println("Port: ", r.Port)
	fmt.Print("Neighbours: ")
	for _,v := range r.Neighbour {
		fmt.Print(v," ")
	}
	fmt.Print("\n")
}

func (r *Router) server() {
	p := make([]byte, 2048)
	addr := net.UDPAddr{
		Port: r.Port,
		IP:   net.ParseIP("127.0.0.1"),
	}
	ser, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Println("Some error when start a UDP server: ", err)
		return
	}
	for {
		_, remoteAddr, err := ser.ReadFromUDP(p)
		fmt.Printf("Read a message from %v %s \n", remoteAddr, p)
		if err != nil {
			fmt.Printf("Some error  %v", err)
			continue
		}
		//go sendResponse(ser, remoteAddr)
	}
}

func (r *Router) broadcast(port int) {
	//p := make([]byte, 2048)
	conn, err := net.Dial("udp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		fmt.Printf("Some error %v", err)
		return
	}
	_,_ = fmt.Fprintf(conn, "Hi UDP Server, How are you doing?")
}


func main() {
	argNum := len(os.Args)
	if argNum <= 1 {
		log.Println("That a isolated router!")
		return
	}
	var r Router
	// initialization
	r.Id = os.Args[1]
	r.Port, _ = strconv.Atoi(os.Args[2])
	// r.Neighbour = make([]int, argNum-3)
	r.RouterTable = make(map[int]TableValue, argNum-3)
	for i := 3; i < argNum; i++ {
		neighbourPort, _ := strconv.Atoi(os.Args[i])
		r.Neighbour = append(r.Neighbour, neighbourPort)
		r.RouterTable[neighbourPort] = TableValue{Unreachable, []string{}}
	}
	// print router state
	r.print()
	go r.server()
	for {
		for i:=0;i<len(r.Neighbour);i++ {
			go r.broadcast(r.Neighbour[i])
		}
		time.Sleep(3 * time.Second)
	}
	return
}

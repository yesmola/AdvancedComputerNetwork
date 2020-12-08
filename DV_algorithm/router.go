package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const Unreachable = 16

type TableValue struct {
	Dist int
	Route []string
}
type Router struct {
	Id           string "Router's id"
	Port         int    "Router's port"
	Neighbour    []int  "neighbour's port"
	RoutingTable map[string]TableValue
}

/*
 * print router's id and port and neighbour's port
 */
func (r *Router) print() {
	fmt.Println("Router Information:")
	fmt.Println("Id         ", r.Id)
	fmt.Println("Port       ", r.Port)
	fmt.Print("Neighbours  ")
	for _, v := range r.Neighbour {
		fmt.Print(v, " ")
	}
	fmt.Print("\n")
}

/*
 * run a udp server to listen the neighbour's router table.
 */
func (r *Router) server() {
	p := make([]byte, 4096)
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
		if err != nil {
			fmt.Println("Some error", err)
			continue
		}

		var Message Router
		buffer := bytes.NewBuffer(p)
		decoder := gob.NewDecoder(buffer)
		err = decoder.Decode(&Message)
		if err != nil {
			fmt.Println("Some error", err)
			continue
		}
		log.Println("Read a message from", remoteAddr.Port, Message)

		// update routing table
		remotePort := remoteAddr.Port - 1000
		remoteId := string(p)
		_, ok := r.RoutingTable[remoteId]
		// if that router is not in routing table
		if ok == false && r.isNeighbour(remotePort) {
			r.RoutingTable[remoteId] = TableValue{
				Dist: 1,
				Route: []string{remoteId},
			}
		}
		//go sendResponse(ser, remoteAddr)
	}
}

/*
 * broadcast current router table to neighbours
 */
func (r *Router) broadcast() {
	for {
		for i := 0; i < len(r.Neighbour); i++ {
			port := r.Neighbour[i]
			lAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:"+strconv.Itoa(r.Port+1000))
			rAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:"+strconv.Itoa(port))
			conn, err := net.DialUDP("udp", lAddr, rAddr)

			if err != nil {
				fmt.Println("Some error", err)
				return
			}

			var buffer bytes.Buffer
			encoder := gob.NewEncoder(&buffer)
			err = encoder.Encode(r)
			if err != nil {
				fmt.Println("Some error", err)
				return
			}

			_,err = conn.Write(buffer.Bytes())
			if err != nil {
				fmt.Println("Some error", err)
				return
			}

			_ = conn.Close()
			buffer.Reset()
		}
		time.Sleep(3 * time.Second)
	}
}

/*
 * check whether the router is neighbour or not
 * return true or false
 */
func (r *Router) isNeighbour(port int) bool {
	for _, v := range r.Neighbour {
		if v == port {
			return true
		}
	}
	return false
}

/*
 * print all neighbours who are active
 */
func (r *Router) listNBsAlive() {
	flag := 0
	for k, v := range r.RoutingTable {
		if v.Dist < Unreachable {
			flag = 1
			fmt.Print(k, " ")
		}
	}
	if flag == 0 {
		fmt.Print("Empty")
	}
	fmt.Print("\n")
}

/*
 * print current routing table
 */
func (r *Router) printRoutingTable() {
	fmt.Println("Destination Route")
	for k,v := range r.RoutingTable {
		fmt.Print("     ",k,"        ",v.Route[0])
		for i:=1;i<len(v.Route);i++ {
			fmt.Print(","+v.Route[i])
		}
		fmt.Println()
	}
}

func main() {
	argNum := len(os.Args)
	if argNum == 1 {
		fmt.Println("Usage: go run router.go [router's id],[router's port],[neighbour's port]")
	}
	info := strings.Split(os.Args[1], ",")
	if len(info) <= 2 {
		log.Println("That a isolated router!")
		return
	}
	var r Router
	// initialization and make all neighbours unreachable until receive their heartbeat
	r.Id = info[0]
	r.Port, _ = strconv.Atoi(info[1])
	for i := 2; i < len(info); i++ {
		neighbourPort,_ := strconv.Atoi(info[i])
		r.Neighbour = append(r.Neighbour, neighbourPort)
	}
	r.RoutingTable = make(map[string]TableValue,10)
	r.RoutingTable[r.Id] = TableValue{
		Dist: 0,
		Route: []string{},
	}
	// print router state
	r.print()

	go r.server()
	go r.broadcast()

	// get stdio from user
	for {
		buf := bufio.NewReader(os.Stdin)
		fmt.Print("> ")
		sentence, err := buf.ReadString('\n')
		// move the last '\n'
		sentence = strings.Replace(sentence, "\n", "", -1)
		command := strings.Split(sentence, " ")
		if err != nil {
			panic(err)
		}

		// run the command
		switch command[0] {
		case "N":
			/*
			 * Print activity’s adjacent list.
			 * Format: 4 5 (in one line, space separated)
			 * If having no neighbors，then print “Empty”
			 */
			r.listNBsAlive()
		case "RT":
			/*
			 * Export the routes that reach to every destination node.
			 * Each route occupies one line.
			 * Format:
			 * Destination route
			 * 1 4,3
			 * 4 5,4
			 */
			 r.printRoutingTable()
		default:
			fmt.Println("Unknown command")
		}
	}

	return
}

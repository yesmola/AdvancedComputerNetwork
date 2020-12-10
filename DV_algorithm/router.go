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
const DefaultTTL = 5
const BroadcastCycle = 5

/*
 * @Destination: the destination of this data packet
 * @TTL: time-to-live of this data packet
 */
type DataPacket struct {
	Destination string
	TTL         int
}

/*
 * @Dist: the distance to the node
 * @Route: the route to the node including the destination
 * @Route: whether refused to pass to the node or not
 */
type TableValue struct {
	Dist    int
	Route   []string
	Refused bool
}

/*
 * @Id: id of this router
 * @Port: port of the router's udp server to listen packet
 * @Neighbour: ports of the router's neighbour's udp server to send packet
 * @RoutingTable : routing table of this router
 */
type Router struct {
	Id           string
	Port         int
	Neighbour    []int
	RoutingTable map[string]TableValue
}

/*
 * UpdateBy: update routing table because of who
 * Log: all routing table in history
 */
type RouterLog struct {
	UpdateBy string
	Log      map[string]TableValue
}

/*
 * record all the update to routing table
 */
var Logs []RouterLog

/*
 * transfer route's id to its port
 */
var IdToPort map[string]int

/*
 * check if the neighbour is alive
 */
var LiveTimer map[int]int64

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
		remotePort := remoteAddr.Port
		// use port 400X to send routing table
		// use port 500X to send data packet
		if remotePort > 4000 && remotePort < 5000 {
			var Message Router
			buffer := bytes.NewBuffer(p)
			decoder := gob.NewDecoder(buffer)
			err = decoder.Decode(&Message)
			if err != nil {
				fmt.Println("Some error", err)
				continue
			}
			//log.Println("Read a router message from", remoteAddr.Port)
			//fmt.Print("> ")

			// reset neighbour's timer
			now := time.Now().Unix()
			LiveTimer[remotePort-1000] = now

			// update routing table
			remoteId := Message.Id
			IdToPort[remoteId] = remotePort - 1000
			isUpdated := false
			for k, v := range Message.RoutingTable {
				_, ok := r.RoutingTable[k]
				// if that router is not in routing table
				if ok == false {
					isUpdated = true
					r.RoutingTable[k] = TableValue{
						Dist:    v.Dist + 1,
						Route:   append([]string{remoteId}, v.Route[:]...),
						Refused: false,
					}
				} else {
					// if refused to pass k
					if r.RoutingTable[k].Refused {
						continue
					}
					// split horizon with poison reversed
					reBack := false
					for _, hop := range v.Route {
						if hop == r.Id {
							reBack = true
							//fmt.Println("find circle")
						}
					}
					if reBack {
						continue
					}
					// if next hop is not remote router and have a better router
					if (len(r.RoutingTable[k].Route) == 0 ||
						(len(r.RoutingTable[k].Route) > 0 && r.RoutingTable[k].Route[0] != remoteId)) &&
						r.RoutingTable[k].Dist > v.Dist+1 {
						isUpdated = true
						r.RoutingTable[k] = TableValue{
							Dist:    v.Dist + 1,
							Route:   append([]string{remoteId}, v.Route[:]...),
							Refused: false,
						}
					} else if len(r.RoutingTable[k].Route) > 0 && r.RoutingTable[k].Route[0] == remoteId {
						// next hop is remote router
						isUpdated = true
						if v.Dist == Unreachable || v.Dist+1 == Unreachable {
							r.RoutingTable[k] = TableValue{
								Dist:    Unreachable,
								Route:   append([]string{}, v.Route[:]...),
								Refused: false,
							}
						} else {
							r.RoutingTable[k] = TableValue{
								Dist:    v.Dist + 1,
								Route:   append([]string{remoteId}, v.Route[:]...),
								Refused: false,
							}
						}
					}
				}
			}
			// record the log of update
			if isUpdated {
				// cannot use just := to copy a map
				cloneRT := make(map[string]TableValue)
				for k, v := range r.RoutingTable {
					cloneRT[k] = v
				}
				newLog := RouterLog{
					UpdateBy: remoteId,
					Log:      cloneRT,
				}
				Logs = append(Logs, newLog)
			}
			continue
		}
		if remotePort > 5000 && remotePort < 6000 {
			var Message DataPacket
			buffer := bytes.NewBuffer(p)
			decoder := gob.NewDecoder(buffer)
			err = decoder.Decode(&Message)
			if err != nil {
				fmt.Println("Some error", err)
				fmt.Print("> ")
				continue
			}
			// if TTL=0, discard it, otherwise continue to transmit it.
			if Message.TTL == 0 {
				fmt.Println("time exceeded", Message.Destination)
				fmt.Print("> ")
				continue
			}
			if r.Id == Message.Destination {
				fmt.Println("Destination", Message.Destination)
				fmt.Print("> ")
				continue
			}
			_, hasRouteToDes := r.RoutingTable[Message.Destination]
			if !hasRouteToDes || r.RoutingTable[Message.Destination].Dist == Unreachable {
				fmt.Println("Dropped", Message.Destination)
				fmt.Print("> ")
				continue
			}
			// if refuse to pass that
			if r.RoutingTable[Message.Destination].Refused {
				fmt.Println("Refused to pass", Message.Destination)
				fmt.Print("> ")
				continue
			}
			fmt.Println("Forward to", Message.Destination)
			fmt.Print("> ")
			go r.sendPacket(Message.Destination, Message.TTL-1, false, remotePort-2000)
			continue
		}
	}
}

/*
 * send from lAddr to rAddr
 * use gob to encode packet
 */
func (r *Router) send(lAddr *net.UDPAddr, rAddr *net.UDPAddr, packet interface{}) error {
	conn, err := net.DialUDP("udp", lAddr, rAddr)
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err = encoder.Encode(packet)
	if err != nil {
		return err
	}

	_, err = conn.Write(buffer.Bytes())
	if err != nil {
		return err
	}

	_ = conn.Close()
	buffer.Reset()
	return nil
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
			err := r.send(lAddr, rAddr, r)

			if err != nil {
				fmt.Println("Some error", err)
				return
			}
		}
		time.Sleep(BroadcastCycle * time.Second)
	}
}

/*
 * check neighbour's state
 */
func (r *Router) checkNeighbour() {
	for {
		now := time.Now().Unix()
		for k, v := range r.RoutingTable {
			port := IdToPort[k]
			if r.isNeighbour(port) && now-LiveTimer[port] > 3*BroadcastCycle {
				if v.Dist == Unreachable {
					continue
				}
				r.RoutingTable[k] = TableValue{
					Dist:    Unreachable,
					Route:   []string{},
					Refused: v.Refused,
				}
				LiveTimer[port] = now
				log.Println(k, "is dead")
				fmt.Print("> ")
			}
		}
		time.Sleep(1 * time.Second)
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
		if v.Dist == 1 {
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
 * Send a data pack to destination
 */
func (r *Router) sendPacket(des string, ttl int, isSource bool, from int) {
	_, ok := r.RoutingTable[des]
	if !ok {
		fmt.Println("No route to", des)
		if !isSource {
			fmt.Println("> ")
		}
		return
	}
	if r.RoutingTable[des].Refused {
		fmt.Println("Refuse to pass", des)
		if !isSource {
			fmt.Println("> ")
		}
		return
	}
	isAdjacent := false
	hasAdjacent := false
	// check if the destination is adjacent node
	// or this router has no adjacent node
	for k, v := range r.RoutingTable {
		if v.Dist != Unreachable {
			hasAdjacent = true
			if k == des && v.Dist == 1 {
				isAdjacent = true
			}
		}
	}
	if hasAdjacent == false {
		fmt.Println("No route to", des)
		if !isSource {
			fmt.Println("> ")
		}
		return
	}
	if des == r.Id {
		fmt.Println("Destination", des)
		if !isSource {
			fmt.Println("> ")
		}
		return
	}
	if isAdjacent && isSource {
		fmt.Println("Direct to", des)
		if !isSource {
			fmt.Println("> ")
		}
	}
	// send to next hop
	data := DataPacket{
		Destination: des,
		TTL:         ttl,
	}
	dist := r.RoutingTable[des].Dist
	route := r.RoutingTable[des].Route
	if dist == Unreachable {
		fmt.Println("Dropped", des)
		if !isSource {
			fmt.Println("> ")
		}
		return
	}
	port := IdToPort[route[0]]
	// split horizon with poison reversed
	if port == from {
		fmt.Println("Dropped", des)
		if !isSource {
			fmt.Println("> ")
		}
		return
	}
	lAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:"+strconv.Itoa(r.Port+2000))
	rAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:"+strconv.Itoa(port))
	err := r.send(lAddr, rAddr, data)
	if err != nil {
		fmt.Println("Some error", err)
		if !isSource {
			fmt.Println("> ")
		}
		return
	}
}

/*
 * Refused to pass the node n.
 */
func (r *Router) setRefused(node string) {
	_, ok := r.RoutingTable[node]
	if ok == false {
		r.RoutingTable[node] = TableValue{
			Dist:    Unreachable,
			Route:   []string{},
			Refused: true,
		}
	} else {
		r.RoutingTable[node] = TableValue{
			Dist:    r.RoutingTable[node].Dist,
			Route:   r.RoutingTable[node].Route,
			Refused: true,
		}
	}
}

/*
 * Specified priority route.
 */
func (r *Router) setPriorityRoute(route []string) {
	des := route[len(route)-1]
	if r.RoutingTable[des].Refused {
		fmt.Println("Refused pass to", des)
	}
	r.RoutingTable[des] = TableValue{
		Dist:    len(route),
		Route:   route,
		Refused: false,
	}
	// record at Logs
	cloneRT := make(map[string]TableValue)
	for k, v := range r.RoutingTable {
		cloneRT[k] = v
	}
	newLog := RouterLog{
		UpdateBy: r.Id,
		Log:      cloneRT,
	}
	Logs = append(Logs, newLog)
}

/*
 * print current routing table
 *
 * Format:
 * Destination route
 * 1 4,3
 * 4 5,4
 */
func printRoutingTable(id string, routingTable map[string]TableValue) {
	fmt.Println("Destination Dis Route")
	for k, v := range routingTable {
		// don't print itself
		if k == id {
			continue
		}
		fmt.Print("     ", k, "        ", v.Dist, "   ")
		// don't print the destination node at route
		for i := 0; i < len(v.Route); i++ {
			fmt.Print(v.Route[i])
			if i < len(v.Route)-1 {
				fmt.Print(",")
			}
		}
		fmt.Println()
	}
}

/*
 * Display the statistics of route updates.
 */
func printStatistics(id string) {
	for _, v := range Logs {
		fmt.Println("Updated By", v.UpdateBy)
		//fmt.Println(v.Log)
		printRoutingTable(id, v.Log)
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
		neighbourPort, _ := strconv.Atoi(info[i])
		r.Neighbour = append(r.Neighbour, neighbourPort)
	}
	r.RoutingTable = make(map[string]TableValue, 10)
	r.RoutingTable[r.Id] = TableValue{
		Dist:    0,
		Route:   []string{},
		Refused: false,
	}

	IdToPort = make(map[string]int, 10)
	LiveTimer = make(map[int]int64, 10)
	for _, v := range r.Neighbour {
		LiveTimer[v] = 0
	}
	// print router's information
	r.print()

	go r.server()
	go r.broadcast()
	go r.checkNeighbour()

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
			 */
			printRoutingTable(r.Id, r.RoutingTable)
		case "D":
			/*
			 * Send a data packet with a specified or default TTL value
			 * to the destination that the number n represents.
			 * D n
			 */
			if len(command) <= 1 {
				fmt.Println("Not enough arguments to call function D")
				break
			}
			r.sendPacket(command[1], DefaultTTL, true, 0)
		case "P":
			/*
			 * Specified priority route.
			 * The route to the destination nk should include the specified intermediate nodes n1 n2 ... .
			 * n1 n2 ... nk : IDs of all K nodes and nk is the ID of destination node.
			 * Replace possible shortest route with possible priority route after the node receives this command.
			 */
			k, _ := strconv.Atoi(command[1])
			var priorityRoute []string
			for i := 2; i < 2+k; i++ {
				priorityRoute = append(priorityRoute, command[i])
			}
			r.setPriorityRoute(priorityRoute)
		case "R":
			/*
			 * Refused to pass the node n.
			 * After the node receives this command,
			 * the node ignores all of the updates that contains node n in routing update.
			 * R n
			 */
			if len(command) <= 1 {
				fmt.Println("Not enough arguments to call function R")
				break
			}
			r.setRefused(command[1])
		case "S":
			/*
			 * Display the statistics of route updates.
			 */
			printStatistics(r.Id)
		case "quit":
			/*
			 * close the router
			 */
			return
		default:
			fmt.Println("Unknown command")
		}
	}

	return
}

# Advanced computer network 
Project Code
## Project A: Distance-Vector-Algorithm
A Policy-Based Routing (PBR) Router based on Distance-Vector Algorithm
### Distance vector algorithm
Distance vector algorithm was described in **RFC 2453**.

> To summarize, here is the basic distance vector algorithm as it has
    been developed so far. 
>- Keep a table with an entry for every possible destination in the
      system.  The entry contains the distance D to the destination, and
      the first router G on the route to that network.  Conceptually,
      there should be an entry for the entity itself, with metric 0, but
      this is not actually included.
>- Periodically, send a routing update to every neighbor.  The update
      is a set of messages that contain all of the information from the
      routing table.  It contains an entry for each destination, with the
      distance shown to that destination.
>- When a routing update arrives from a neighbor G', add the cost
      associated with the network that is shared with G'.  (This should
      be the network over which the update arrived.)  Call the resulting
>  distance D'.  Compare the resulting distances with the current
      routing table entries.  If the new distance D' for N is smaller
      than the existing value D, adopt the new route.  That is, change
      the table entry for N to have metric D' and router G'.  If G' is
      the router from which the existing route came, i.e., G' = G, then
      use the new metric even if it is larger than the old one.

In practise, there are some problems when implement that such as 
changes in topology, instability and so on.We can find all answers in **RFC 2453**.

### Run
Run like ``` go run ./DV_algorithm/router.go 3,3003,3004```

### Issues
Maybe some bugs.
Set priority route maybe some faults.

## Project B: Reliable Data Transfer over UDP (RDT-UDP)
Build a reliable transport protocol “RDT-UDP” over unreliable UDP. 
Also, the protocol provide in-order, reliable delivery of UDP datagrams, 
and must do so in the presence of packet loss, delay, corruption, duplication, and re-ordering.

### Requirements
+ Use TCP-style connection maintenance, including establishment, maintains and
  release.
+ The header includes the following fields:
    1. Sequence Number (32 bits)
    2. Acknowledgement Number (32 bits)
    3. ConnectionID(16bits)(if exists)
    4. A (ACK, 1 bit)
    5. S (SYN, 1 bit)
    6. F (FIN, 1 bit)
+ Avoid losing messages.
+ In case of lost message, detect and recover it.
+ Handle messages reordering.
+ (Optional) Add Congestion Control. The control algorithm is AIMD (like TCP)
+ The maximum UDP packet size is 524 bytes including a header (512 bytes for the
  payload).
+ The maximum sequence and acknowledgment number should be 102400 and be reset
  to zero whenever it reaches the maximum value.
  

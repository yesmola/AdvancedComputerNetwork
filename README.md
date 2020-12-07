#Advanced computer network 
Project Code
## Project A: Distance-Vector-Algorithm
A Policy-Based Routing (PBR) Router based on Distance-Vector Algorithm
### Distance vector algorithm
Distance vector algorithm was described in **RFC 2453**.I select some information from that as below.
That may be different from the real implementations.
>Distance vector algorithms are based on the exchange of only a small amount of information.
>Each entity keeps a routing database with one entry for every possible destination in the system.
>An actual implementation is likely to need to keep the following information about each destination:
> 
>- address: in IP implementations of these algorithms, this will be
       the IP address of the host or network.
>- router: the first router along the route to the destination.
>- interface: the physical network which must be used to reach the
      first router.
>- metric: a number, indicating the distance to the destination.
>- timer: the amount of time since the entry was last updated.
> 
>In addition, various flags and other internal information will
     probably be included.  This database is initialized with a
     description of the entities that are directly connected to the
     system.  It is updated according to information received in messages
     from neighboring routers.
>
>The purpose of routing is to find a way to get
    datagrams to their ultimate destinations.  Distance vector algorithms
    are based on a table in each router listing the best route to every
    destination in the system.
>
>Let D(i,j) represent the metric of the best route from entity i to
    entity j.it is easy to show that the best metric must be described by
>
>- D(i,i) = 0,                      all i 
>
>- D(i,j) = min [d(i,k) + D(k,j)],  otherwise
>
> Because
    we don't have to make assumptions about when updates are sent, it is
    safe to run the algorithm asynchronously.  That is, each entity can
    send updates according to its own clock. 
>
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

### Distance vector algorithm
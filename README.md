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
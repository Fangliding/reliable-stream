package reliablestream

import (
	"errors"
	"io"
	"sync"

	"github.com/Fangliding/reliable-stream/utils"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"
)

type peerTransport struct {
	mu        sync.Mutex
	transport io.ReadWriteCloser
}

// serverEndpoint is a LinkEndpoint that routes outgoing packets
// to the correct peer transport by destination IPv6 address.
type serverEndpoint struct {
	routes     utils.TypedSyncMap[tcpip.Address, *peerTransport]
	mtu        uint32
	dispatcher stack.NetworkDispatcher
}

func (e *serverEndpoint) MTU() uint32 {
	return e.mtu
}

func (e *serverEndpoint) Attach(d stack.NetworkDispatcher) {
	if d == nil {
		e.dispatcher = noopDispatcher{}
		return
	}
	e.dispatcher = d
}
func (e *serverEndpoint) IsAttached() bool {
	if _, ok := e.dispatcher.(noopDispatcher); ok {
		return false
	}
	return e.dispatcher != nil
}

func (e *serverEndpoint) WritePackets(pkts stack.PacketBufferList) (int, tcpip.Error) {
	n := 0
	for _, pkt := range pkts.AsSlice() {
		netHdr := pkt.NetworkHeader().Slice()
		if len(netHdr) < header.IPv6MinimumSize {
			continue
		}
		dst := header.IPv6(netHdr).DestinationAddress()

		peer, ok := e.routes.Load(dst)
		if !ok {
			continue
		}

		view := pkt.ToView()
		peer.mu.Lock()
		_, err := peer.transport.Write(view.AsSlice())
		peer.mu.Unlock()
		view.Release()
		if err != nil {
			return n, &tcpip.ErrAborted{}
		}
		n++
	}
	return n, nil
}

func (e *serverEndpoint) Close() {
	e.routes.Range(func(_ tcpip.Address, peer *peerTransport) bool {
		peer.transport.Close()
		return true
	})
}

func (e *serverEndpoint) addRoute(addr tcpip.Address, peer *peerTransport) {
	e.routes.Store(addr, peer)
}

func (e *serverEndpoint) removeRoutes(peer *peerTransport) {
	e.routes.Range(func(addr tcpip.Address, p *peerTransport) bool {
		if p == peer {
			e.routes.Delete(addr)
		}
		return true
	})
}

// Server accepts reliable streams over unreliable transports.
// A single gvisor stack is shared across all peer transports.
type Server struct {
	s        *stack.Stack
	endpoint *serverEndpoint
	streams  chan Stream
	done     *utils.Done
}

// NewServer creates a Server to handle incoming connections from peers.
// It can handle multiple peers concurrently.
func NewServer(mtu uint32) (*Server, error) {
	ep := &serverEndpoint{mtu: mtu}

	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocolCUBIC},
	})

	if err := s.CreateNIC(nicID, ep); err != nil {
		return nil, errors.New(err.String())
	}
	if err := s.SetPromiscuousMode(nicID, true); err != nil {
		return nil, errors.New(err.String())
	}
	if err := s.SetSpoofing(nicID, true); err != nil {
		return nil, errors.New(err.String())
	}

	s.SetRouteTable([]tcpip.Route{{
		Destination: header.IPv6EmptySubnet,
		NIC:         nicID,
	}})

	srv := &Server{
		s:        s,
		endpoint: ep,
		streams:  make(chan Stream, 256),
		done:     utils.NewDone(),
	}

	fwd := tcp.NewForwarder(s, 0, 65535, srv.handleRequest)
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, fwd.HandlePacket)

	return srv, nil
}

// AddTransport registers a new peer connection.
func (srv *Server) AddTransport(transport io.ReadWriteCloser) {
	peer := &peerTransport{transport: transport}
	go srv.readLoopForPeer(peer)
}

func (srv *Server) readLoopForPeer(peer *peerTransport) {
	buf := make([]byte, srv.endpoint.mtu)
	for {
		n, err := peer.transport.Read(buf)
		if err != nil {
			srv.endpoint.removeRoutes(peer)
			return
		}
		data := buf[:n]
		if len(data) >= header.IPv6MinimumSize {
			src := header.IPv6(data).SourceAddress()
			srv.endpoint.addRoute(src, peer)
		}
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(data),
		})
		srv.endpoint.dispatcher.DeliverNetworkPacket(ipv6.ProtocolNumber, pkt)
		pkt.DecRef()
	}
}

func (srv *Server) handleRequest(r *tcp.ForwarderRequest) {
	var wq waiter.Queue
	ep, err := r.CreateEndpoint(&wq)
	if err != nil {
		r.Complete(true)
		return
	}
	r.Complete(false)

	conn := gonet.NewTCPConn(&wq, ep)
	select {
	case srv.streams <- conn:
	case <-srv.done.Done():
		conn.Close()
	}
}

// Accept blocks until a new stream arrives or the server is closed.
func (srv *Server) Accept() (Stream, error) {
	select {
	case stream := <-srv.streams:
		return stream, nil
	case <-srv.done.Done():
		return nil, errors.New("server closed")
	}
}

func (srv *Server) Close() {
	srv.done.Close()
	srv.s.Close()
}

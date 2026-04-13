package reliablestream

import (
	"crypto/rand"
	"errors"
	"io"
	"sync"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
)

const nicID = 1
const streamPort = 14514

// Stream is a reliable bidirectional stream with half-close support.
type Stream interface {
	io.ReadWriteCloser
	CloseRead() error
	CloseWrite() error
}

// fd00::/64
var prefix = [8]byte{0xfd}

func randAddrInPrefix() tcpip.Address {
	var addr [16]byte
	copy(addr[:8], prefix[:])
	rand.Read(addr[8:])
	return tcpip.AddrFromSlice(addr[:])
}

type transportEndpoint struct {
	mu         sync.Mutex
	transport  io.ReadWriteCloser
	mtu        uint32
	dispatcher stack.NetworkDispatcher
}

func (e *transportEndpoint) MTU() uint32 {
	return e.mtu
}

func (e *transportEndpoint) Attach(d stack.NetworkDispatcher) {
	if d == nil {
		e.dispatcher = noopDispatcher{}
		return
	}
	e.dispatcher = d
}

func (e *transportEndpoint) IsAttached() bool {
	if _, ok := e.dispatcher.(noopDispatcher); ok {
		return false
	}
	return e.dispatcher != nil
}

func (e *transportEndpoint) Close() {
	e.transport.Close()
}

func (e *transportEndpoint) WritePackets(pkts stack.PacketBufferList) (int, tcpip.Error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, pkt := range pkts.AsSlice() {
		view := pkt.ToView()
		_, err := e.transport.Write(view.AsSlice())
		view.Release()
		if err != nil {
			return n, &tcpip.ErrAborted{}
		}
		n++
	}
	return n, nil
}

// Client wraps a gvisor userspace TCP/IP stack over an unreliable transport.
// Call OpenStream to create reliable streams
type Client struct {
	transport io.ReadWriteCloser
	s         *stack.Stack
	endpoint  *transportEndpoint
}

// NewClient creates a Client over the given unreliable transport
func NewClient(transport io.ReadWriteCloser, mtu uint32) (*Client, error) {
	ep := &transportEndpoint{transport: transport, mtu: mtu}

	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocolCUBIC},
	})

	if err := s.CreateNIC(nicID, ep); err != nil {
		return nil, errors.New(err.String())
	}

	localAddr := randAddrInPrefix()
	protoAddr := tcpip.ProtocolAddress{
		Protocol: ipv6.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   localAddr,
			PrefixLen: 64,
		},
	}
	if err := s.AddProtocolAddress(nicID, protoAddr, stack.AddressProperties{}); err != nil {
		return nil, errors.New(err.String())
	}

	s.SetRouteTable([]tcpip.Route{{
		Destination: header.IPv6EmptySubnet,
		NIC:         nicID,
	}})

	c := &Client{transport: transport, s: s, endpoint: ep}
	go c.readLoop()
	return c, nil
}

func (c *Client) readLoop() {
	buf := make([]byte, c.endpoint.mtu)
	for {
		n, err := c.transport.Read(buf)
		if err != nil {
			return
		}
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(buf[:n]),
		})
		c.endpoint.dispatcher.DeliverNetworkPacket(ipv6.ProtocolNumber, pkt)
		pkt.DecRef()
	}
}

// OpenStream creates a reliable stream over the unreliable transport
func (c *Client) OpenStream() (Stream, error) {
	addr := tcpip.FullAddress{
		NIC:  nicID,
		Addr: randAddrInPrefix(),
		Port: streamPort,
	}
	return gonet.DialTCP(c.s, addr, ipv6.ProtocolNumber)
}

func (c *Client) Close() {
	c.s.Close()
}

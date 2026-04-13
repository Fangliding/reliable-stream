package reliablestream

import (
	"bytes"
	"io"
	"net"
	"sync"

	"github.com/Fangliding/reliable-stream/utils"
)

// StartDemoServer listens on listenAddr (UDP), accepts packets from any peer,
// and for each accepted stream, proxies data to proxyTarget (TCP).
func StartDemoServer(listenAddr string, proxyTarget string, mtu uint32) error {
	udpAddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}

	srv, err := NewServer(mtu)
	if err != nil {
		conn.Close()
		return err
	}

	go func() {
		peers := make(map[string]*peerUDP)
		buf := make([]byte, mtu)
		for {
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			key := remoteAddr.String()
			// should clean in production, but this is just a demo
			peer, ok := peers[key]
			if !ok {
				peer = newPeerUDP(conn, remoteAddr)
				peers[key] = peer
				srv.AddTransport(peer)
			}
			peer.deliver(buf[:n])
		}
	}()

	go func() {
		for {
			stream, err := srv.Accept()
			if err != nil {
				return
			}
			go func() {
				defer stream.Close()
				target, err := net.Dial("tcp", proxyTarget)
				if err != nil {
					return
				}
				defer target.Close()
				go io.Copy(target, stream)
				io.Copy(stream, target)
			}()
		}
	}()

	return nil
}

// StartDemoClient connects to serverAddr (UDP) using udpLocalAddr as the
// local UDP address, listens on tcpListenAddr (TCP), and for each accepted
// TCP connection, opens a reliable stream and proxies data.
func StartDemoClient(serverAddr string, udpLocalAddr string, tcpListenAddr string, mtu uint32) error {
	raddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return err
	}
	laddr, err := net.ResolveUDPAddr("udp", udpLocalAddr)
	if err != nil {
		return err
	}
	udp, err := net.DialUDP("udp", laddr, raddr)
	if err != nil {
		return err
	}

	client, err := NewClient(udp, mtu)
	if err != nil {
		udp.Close()
		return err
	}

	tcpLn, err := net.Listen("tcp", tcpListenAddr)
	if err != nil {
		client.Close()
		return err
	}

	go func() {
		for {
			conn, err := tcpLn.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				stream, err := client.OpenStream()
				if err != nil {
					return
				}
				defer stream.Close()
				go io.Copy(stream, conn)
				io.Copy(conn, stream)
			}()
		}
	}()

	return nil
}

var packetPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

type peerUDP struct {
	conn   *net.UDPConn
	remote *net.UDPAddr
	ch     chan *bytes.Buffer
	done   *utils.Done
}

func newPeerUDP(conn *net.UDPConn, remote *net.UDPAddr) *peerUDP {
	return &peerUDP{
		conn:   conn,
		remote: remote,
		ch:     make(chan *bytes.Buffer, 256),
		done:   utils.NewDone(),
	}
}

func (p *peerUDP) deliver(data []byte) {
	buf := packetPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Write(data)
	select {
	case p.ch <- buf:
	case <-p.done.Done():
		packetPool.Put(buf)
	}
}

func (p *peerUDP) Read(b []byte) (int, error) {
	select {
	case buf := <-p.ch:
		n := copy(b, buf.Bytes())
		packetPool.Put(buf)
		return n, nil
	case <-p.done.Done():
		return 0, io.EOF
	}
}

func (p *peerUDP) Write(b []byte) (int, error) {
	return p.conn.WriteToUDP(b, p.remote)
}

func (p *peerUDP) Close() error {
	p.done.Close()
	return nil
}

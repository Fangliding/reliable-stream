package reliablestream

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPipe(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	srv, err := NewServer(1500)
	if err != nil {
		t.Fatal(err)
	}
	srv.AddTransport(serverConn)

	client, err := NewClient(clientConn, 1500)
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		stream, err := srv.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer stream.Close()
		// Echo back whatever we receive.
		_, err = io.Copy(stream, stream)
		errCh <- err
	}()

	stream, err := client.OpenStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	msg := []byte("hello reliable stream")
	if _, err := stream.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	stream.CloseWrite()

	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(msg) {
		t.Fatalf("got %q, want %q", got, msg)
	}
	t.Logf("echo OK: %q", got)
}

func TestDemo(t *testing.T) {
	serverUDP := "127.0.0.1:19000"
	clientUDP := "127.0.0.1:19001"
	clientTCP := "127.0.0.1:19002"
	proxyTarget := "cloudflare.com:80"
	mtu := uint32(1500)

	if err := StartDemoServer(serverUDP, proxyTarget, mtu); err != nil {
		t.Fatalf("start server: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if err := StartDemoClient(serverUDP, clientUDP, clientTCP, mtu); err != nil {
		t.Fatalf("start client: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	req, _ := http.NewRequest("GET", "http://"+clientTCP+"/cdn-cgi/trace", nil)
	req.Host = "cloudflare.com"
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	text := string(body)
	t.Logf("response:\n%s", text)

	if !strings.Contains(text, "fl=") || !strings.Contains(text, "ip=") {
		t.Fatalf("unexpected response body:\n%s", text)
	}
}

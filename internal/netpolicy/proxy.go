package netpolicy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type Proxy struct {
	Plan   Plan
	ln     net.Listener
	closed chan struct{}
	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	wg     sync.WaitGroup
	Audit  func(host string, decision Decision)
}

func ListenProxy(plan Plan) (*Proxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen network policy proxy: %w", err)
	}
	return &Proxy{Plan: plan, ln: ln, closed: make(chan struct{}), conns: make(map[net.Conn]struct{})}, nil
}

func (p *Proxy) URL() string {
	if p == nil || p.ln == nil {
		return ""
	}
	return "http://" + p.ln.Addr().String()
}

func (p *Proxy) Serve(ctx context.Context) error {
	if p == nil || p.ln == nil {
		return fmt.Errorf("network policy proxy is not initialized")
	}
	go func() { <-ctx.Done(); _ = p.Close() }()
	for {
		conn, err := p.ln.Accept()
		if err != nil {
			select {
			case <-p.closed:
				return nil
			default:
			}
			return fmt.Errorf("accept network policy proxy: %w", err)
		}
		if !p.track(conn) {
			continue
		}
		go func() {
			defer p.wg.Done()
			defer p.untrack(conn)
			p.handle(conn)
		}()
	}
}

func (p *Proxy) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	select {
	case <-p.closed:
		p.mu.Unlock()
		return nil
	default:
		close(p.closed)
	}
	listenerErr := p.ln.Close()
	// Closing the listener does not close connections accepted before it. A
	// keep-alive client or CONNECT tunnel must not outlive the protected run,
	// otherwise cleanup can race an active proxy handler and filesystem
	// unmount. Close the bounded set of active connections, then wait for every
	// handler to leave before returning to the run coordinator.
	connections := make([]net.Conn, 0, len(p.conns))
	for conn := range p.conns {
		connections = append(connections, conn)
	}
	p.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
	p.wg.Wait()
	return listenerErr
}

func (p *Proxy) track(conn net.Conn) bool {
	p.mu.Lock()
	select {
	case <-p.closed:
		p.mu.Unlock()
		_ = conn.Close()
		return false
	default:
	}
	p.conns[conn] = struct{}{}
	p.wg.Add(1)
	p.mu.Unlock()
	return true
}

func (p *Proxy) untrack(conn net.Conn) {
	p.mu.Lock()
	delete(p.conns, conn)
	p.mu.Unlock()
}

func (p *Proxy) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	host := request.Host
	if request.Method != http.MethodConnect {
		if parsed, parseErr := url.Parse(request.URL.String()); parseErr == nil && parsed.Host != "" {
			host = parsed.Host
		}
	}
	host = strings.Split(host, ":")[0]
	decision := p.Plan.Explain(host)
	if p.Audit != nil {
		p.Audit(host, decision)
	}
	if decision == Deny {
		_, _ = io.WriteString(conn, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n")
		return
	}
	if request.Method == http.MethodConnect {
		p.tunnel(conn, request.Host)
		return
	}
	p.forwardHTTP(conn, request)
}

func (p *Proxy) tunnel(client net.Conn, address string) {
	upstream, err := net.Dial("tcp", address)
	if err != nil {
		_, _ = io.WriteString(client, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer upstream.Close()
	_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
	go io.Copy(upstream, client)
	_, _ = io.Copy(client, upstream)
}

func (p *Proxy) forwardHTTP(client net.Conn, request *http.Request) {
	if request.URL.Host == "" {
		_, _ = io.WriteString(client, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
		return
	}
	request.RequestURI = ""
	transport := &http.Transport{Proxy: nil}
	response, err := transport.RoundTrip(request)
	if err != nil {
		_, _ = io.WriteString(client, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer response.Body.Close()
	_ = response.Write(client)
}

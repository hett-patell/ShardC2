package agent

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

type SOCKS5Server struct {
	listener net.Listener
	port     int
	done     chan struct{}
}

func StartSOCKS5(port int) (*SOCKS5Server, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("socks5 listen: %w", err)
	}

	s := &SOCKS5Server{listener: ln, port: port, done: make(chan struct{})}
	go s.serve()
	return s, nil
}

func (s *SOCKS5Server) Stop() {
	close(s.done)
	s.listener.Close()
}

func (s *SOCKS5Server) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		go s.handleConn(conn)
	}
}

func (s *SOCKS5Server) handleConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// SOCKS5 greeting
	buf := make([]byte, 258)
	n, err := conn.Read(buf)
	if err != nil || n < 2 || buf[0] != 0x05 {
		return
	}

	// No auth required
	conn.Write([]byte{0x05, 0x00})

	// Read connect request
	n, err = conn.Read(buf)
	if err != nil || n < 7 || buf[0] != 0x05 || buf[1] != 0x01 {
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	var targetAddr string
	var targetPort uint16

	switch buf[3] {
	case 0x01: // IPv4
		if n < 10 {
			return
		}
		targetAddr = net.IP(buf[4:8]).String()
		targetPort = binary.BigEndian.Uint16(buf[8:10])
	case 0x03: // Domain
		domLen := int(buf[4])
		if n < 5+domLen+2 {
			return
		}
		targetAddr = string(buf[5 : 5+domLen])
		targetPort = binary.BigEndian.Uint16(buf[5+domLen : 7+domLen])
	case 0x04: // IPv6
		if n < 22 {
			return
		}
		targetAddr = net.IP(buf[4:20]).String()
		targetPort = binary.BigEndian.Uint16(buf[20:22])
	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	target := socksTarget(targetAddr, targetPort)
	remote, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer remote.Close()

	// Success reply
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// Clear deadlines for relay
	conn.SetDeadline(time.Time{})
	remote.SetDeadline(time.Time{})

	relay(conn, remote)
}

func socksTarget(host string, port uint16) string {
	return net.JoinHostPort(host, strconv.Itoa(int(port)))
}

func relay(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	cp := func(dst, src net.Conn) {
		defer wg.Done()
		io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
	}

	go cp(a, b)
	go cp(b, a)
	wg.Wait()
}

func (a *Agent) HandleProxy(payload string) (string, error) {
	var req struct {
		Action string `json:"action"`
		Port   int    `json:"port"`
	}

	if err := parseProxyPayload(payload, &req); err != nil {
		return "", err
	}

	switch req.Action {
	case "start":
		port := req.Port
		if port == 0 {
			port = 1080
		}
		srv, err := StartSOCKS5(port)
		if err != nil {
			return "", err
		}
		a.proxySrv = srv
		return fmt.Sprintf("SOCKS5 proxy started on 127.0.0.1:%d", port), nil

	case "stop":
		if a.proxySrv != nil {
			a.proxySrv.Stop()
			a.proxySrv = nil
			return "SOCKS5 proxy stopped", nil
		}
		return "no proxy running", nil

	case "status":
		if a.proxySrv != nil {
			return fmt.Sprintf("SOCKS5 proxy running on port %d", a.proxySrv.port), nil
		}
		return "no proxy running", nil

	default:
		return "", fmt.Errorf("unknown proxy action: %s", req.Action)
	}
}

func parseProxyPayload(payload string, v interface{}) error {
	return json.Unmarshal([]byte(payload), v)
}

package proxy

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/migandhi/tunnel-software/internal/bandwidth"
	"github.com/migandhi/tunnel-software/internal/config"
	"github.com/migandhi/tunnel-software/internal/control"
)

type TCPManager struct {
	cfg       *config.Config
	reg       *control.Registry
	acct      *bandwidth.Accountant
	log       *slog.Logger
	mu        sync.Mutex
	listeners map[int]net.Listener
}

func NewTCPManager(cfg *config.Config, reg *control.Registry, acct *bandwidth.Accountant, log *slog.Logger) *TCPManager {
	return &TCPManager{cfg: cfg, reg: reg, acct: acct, log: log, listeners: map[int]net.Listener{}}
}
func (m *TCPManager) EnsurePort(port int) error {
	if port < m.cfg.TCPPortMin || port > m.cfg.TCPPortMax {
		return fmt.Errorf("port %d outside configured range", port)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.listeners[port]; ok {
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	m.listeners[port] = ln
	m.log.Info("public tcp port bound", "port", port)
	go m.serve(ln, port)
	return nil
}
func (m *TCPManager) ClosePort(port int) {
	m.mu.Lock()
	ln, ok := m.listeners[port]
	if ok {
		delete(m.listeners, port)
	}
	m.mu.Unlock()
	if ok {
		_ = ln.Close()
		m.log.Info("public tcp port released", "port", port)
	}
}
func (m *TCPManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for p, ln := range m.listeners {
		_ = ln.Close()
		delete(m.listeners, p)
	}
}
func (m *TCPManager) serve(ln net.Listener, port int) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go m.bridge(conn, port)
	}
}
func (m *TCPManager) bridge(pub net.Conn, port int) {
	defer pub.Close()
	t := m.reg.GetTCP(port)
	if t == nil {
		return
	}
	stream, err := t.OpenStream()
	if err != nil {
		return
	}
	defer stream.Close()
	tracked := m.acct.Track(stream, t.UserID)
	idlePub := &idleConn{Conn: pub, timeout: m.cfg.TCPIdleTimeout}
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(tracked, idlePub); done <- struct{}{} }()
	go func() { _, _ = io.Copy(idlePub, tracked); done <- struct{}{} }()
	<-done
	_ = pub.Close()
	_ = stream.Close()
	<-done
}

type idleConn struct {
	net.Conn
	timeout time.Duration
}

func (c *idleConn) Read(b []byte) (int, error) {
	_ = c.Conn.SetDeadline(time.Now().Add(c.timeout))
	return c.Conn.Read(b)
}
func (c *idleConn) Write(b []byte) (int, error) {
	_ = c.Conn.SetDeadline(time.Now().Add(c.timeout))
	return c.Conn.Write(b)
}

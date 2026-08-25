package control

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
)

var ErrTooManyStreams = errors.New("per-tunnel concurrent stream limit reached")
var ErrTunnelOffline = errors.New("tunnel offline")

type Tunnel struct {
	UserID      int64
	Subdomain   string
	Port        int
	Protocol    string
	Session     *yamux.Session
	ConnectedAt time.Time
	RemoteAddr  string
	maxStreams  int64
	streams     atomic.Int64
}

func (t *Tunnel) OpenStream() (net.Conn, error) {
	if t.streams.Load() >= t.maxStreams {
		return nil, ErrTooManyStreams
	}
	s, err := t.Session.Open()
	if err != nil {
		return nil, err
	}
	t.streams.Add(1)
	return &countedConn{Conn: s, t: t}, nil
}
func (t *Tunnel) ActiveStreams() int64 { return t.streams.Load() }

type countedConn struct {
	net.Conn
	t    *Tunnel
	once sync.Once
}

func (c *countedConn) Close() error { c.once.Do(func() { c.t.streams.Add(-1) }); return c.Conn.Close() }

type Registry struct {
	mu   sync.RWMutex
	http map[string]*Tunnel
	tcp  map[int]*Tunnel
}

func NewRegistry() *Registry { return &Registry{http: map[string]*Tunnel{}, tcp: map[int]*Tunnel{}} }
func (r *Registry) RegisterHTTP(t *Tunnel) {
	r.mu.Lock()
	old := r.http[t.Subdomain]
	r.http[t.Subdomain] = t
	r.mu.Unlock()
	if old != nil && old != t {
		old.Session.Close()
	}
}
func (r *Registry) RegisterTCP(t *Tunnel) {
	r.mu.Lock()
	old := r.tcp[t.Port]
	r.tcp[t.Port] = t
	r.mu.Unlock()
	if old != nil && old != t {
		old.Session.Close()
	}
}
func (r *Registry) Unregister(t *Tunnel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.Protocol == "tcp" {
		if r.tcp[t.Port] == t {
			delete(r.tcp, t.Port)
		}
	} else {
		if r.http[t.Subdomain] == t {
			delete(r.http, t.Subdomain)
		}
	}
}
func (r *Registry) GetHTTP(sub string) *Tunnel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := r.http[sub]
	if t == nil || t.Session.IsClosed() {
		return nil
	}
	return t
}
func (r *Registry) GetTCP(port int) *Tunnel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := r.tcp[port]
	if t == nil || t.Session.IsClosed() {
		return nil
	}
	return t
}
func (r *Registry) CloseUser(id int64) int {
	r.mu.RLock()
	var v []*Tunnel
	for _, t := range r.http {
		if t.UserID == id {
			v = append(v, t)
		}
	}
	for _, t := range r.tcp {
		if t.UserID == id {
			v = append(v, t)
		}
	}
	r.mu.RUnlock()
	for _, t := range v {
		t.Session.Close()
	}
	return len(v)
}

type Snapshot struct {
	HTTPTunnels int
	TCPTunnels  int
	Tunnels     []SnapshotTunnel
}
type SnapshotTunnel struct {
	UserID    int64
	Subdomain string
	Protocol  string
	Port      int
	Streams   int64
	Since     time.Time
}

func (r *Registry) Snapshot() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := Snapshot{HTTPTunnels: len(r.http), TCPTunnels: len(r.tcp)}
	for _, t := range r.http {
		s.Tunnels = append(s.Tunnels, SnapshotTunnel{t.UserID, t.Subdomain, "http", 0, t.ActiveStreams(), t.ConnectedAt})
	}
	for _, t := range r.tcp {
		s.Tunnels = append(s.Tunnels, SnapshotTunnel{t.UserID, t.Subdomain, "tcp", t.Port, t.ActiveStreams(), t.ConnectedAt})
	}
	return s
}

package control

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/migandhi/tunnel-software/internal/auth"
	"github.com/migandhi/tunnel-software/internal/config"
	"github.com/migandhi/tunnel-software/internal/security"
	"github.com/migandhi/tunnel-software/internal/store"
	"github.com/migandhi/tunnel-software/internal/version"
)

type Handshake struct {
	Proto         int    `json:"proto"`
	Token         string `json:"token"`
	Protocol      string `json:"protocol"`
	ClientVersion string `json:"client_version"`
}
type HandshakeResponse struct {
	OK        bool   `json:"ok"`
	ErrorCode string `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
	Subdomain string `json:"subdomain,omitempty"`
	URL       string `json:"url,omitempty"`
	TCPAddr   string `json:"tcp_addr,omitempty"`
	Warning   string `json:"warning,omitempty"`
}
type PortOpener interface{ EnsurePort(int) error }

type Server struct {
	cfg     *config.Config
	st      *store.Store
	reg     *Registry
	ports   PortOpener
	log     *slog.Logger
	limiter *security.Limiter
	tlsCfg  *tls.Config
}

func NewServer(cfg *config.Config, st *store.Store, reg *Registry, ports PortOpener, log *slog.Logger, tlsCfg *tls.Config) *Server {
	return &Server{cfg: cfg, st: st, reg: reg, ports: ports, log: log, limiter: security.NewLimiter(cfg.MaxConnsPerIP, time.Minute), tlsCfg: tlsCfg}
}
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.cfg.ControlAddr)
	if err != nil {
		return fmt.Errorf("control listen %s: %w", s.cfg.ControlAddr, err)
	}
	if s.tlsCfg != nil {
		ln = tls.NewListener(ln, s.tlsCfg)
	}
	s.log.Info("control server listening", "addr", s.cfg.ControlAddr, "tls", s.tlsCfg != nil)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			return err
		}
		go s.handle(conn)
	}
}
func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	if ip == "" {
		ip = conn.RemoteAddr().String()
	}
	if !s.limiter.Allow(ip) {
		return
	}
	_ = conn.SetDeadline(time.Now().Add(s.cfg.HandshakeTimeout))
	br := bufio.NewReaderSize(conn, 8192)
	line, err := br.ReadSlice('\n')
	if err != nil {
		return
	}
	var hs Handshake
	if err := json.Unmarshal(line, &hs); err != nil {
		s.reject(conn, "bad_request", "malformed handshake")
		return
	}
	if hs.Proto != version.ProtocolVersion {
		s.reject(conn, "unsupported_proto", fmt.Sprintf("server speaks protocol v%d; please upgrade your client", version.ProtocolVersion))
		return
	}
	if hs.Protocol != "http" && hs.Protocol != "tcp" {
		s.reject(conn, "bad_request", "protocol must be http or tcp")
		return
	}
	u, err := s.st.GetUserByTokenHash(auth.HashToken(hs.Token))
	if err != nil {
		s.st.Audit(ip, "auth_failed", "invalid token")
		s.reject(conn, "invalid_token", "authentication failed: unknown token")
		return
	}
	now := time.Now()
	switch {
	case !u.Active:
		s.reject(conn, "disabled", "account is disabled; contact the administrator")
		return
	case u.Expired(now):
		s.reject(conn, "expired", fmt.Sprintf("subscription expired on %s; contact the administrator to renew", u.ExpiresAt.Format("2006-01-02")))
		return
	case u.OverLimit():
		s.reject(conn, "bandwidth_exceeded", "bandwidth limit reached; contact the administrator to renew or increase the limit")
		return
	case hs.Protocol == "tcp" && u.TCPPort == 0:
		s.reject(conn, "no_tcp_port", "your plan has no TCP port assigned")
		return
	}
	resp := HandshakeResponse{OK: true, Subdomain: u.Subdomain}
	if hs.Protocol == "http" {
		resp.URL = fmt.Sprintf("%s://%s.%s", s.cfg.PublicScheme(), u.Subdomain, s.cfg.Domain)
	} else {
		resp.TCPAddr = fmt.Sprintf("%s:%d", s.cfg.Domain, u.TCPPort)
	}
	if d := int(time.Until(u.ExpiresAt).Hours() / 24); d <= 7 {
		if d < 0 {
			d = 0
		}
		resp.Warning = fmt.Sprintf("subscription expires in %d day(s) on %s", d, u.ExpiresAt.Format("2006-01-02"))
	}
	if u.BandwidthLimit > 0 && u.BandwidthUsed*100 >= u.BandwidthLimit*80 {
		if resp.Warning != "" {
			resp.Warning += "; "
		}
		resp.Warning += fmt.Sprintf("bandwidth %d%% used", u.BandwidthUsed*100/u.BandwidthLimit)
	}
	if err := writeJSONLine(conn, resp); err != nil {
		return
	}
	_ = conn.SetDeadline(time.Time{})
	ycfg := yamux.DefaultConfig()
	ycfg.KeepAliveInterval = 20 * time.Second
	ycfg.ConnectionWriteTimeout = 30 * time.Second
	ycfg.MaxStreamWindowSize = uint32(s.cfg.StreamWindowKB) * 1024
	ycfg.LogOutput = io.Discard
	session, err := yamux.Server(conn, ycfg)
	if err != nil {
		return
	}
	t := &Tunnel{UserID: u.ID, Subdomain: u.Subdomain, Protocol: hs.Protocol, Session: session, ConnectedAt: time.Now(), RemoteAddr: ip, maxStreams: int64(s.cfg.MaxStreamsPerTunnel)}
	if hs.Protocol == "tcp" {
		t.Port = u.TCPPort
		if err := s.ports.EnsurePort(u.TCPPort); err != nil {
			s.log.Error("failed to open public port", "port", u.TCPPort, "err", err)
			session.Close()
			return
		}
		s.reg.RegisterTCP(t)
	} else {
		s.reg.RegisterHTTP(t)
	}
	s.st.Audit(u.Subdomain, "tunnel_connect", fmt.Sprintf("%s from %s", hs.Protocol, ip))
	<-session.CloseChan()
	s.reg.Unregister(t)
	s.log.Info("tunnel offline", "user", u.Subdomain, "protocol", hs.Protocol)
	s.st.Audit(u.Subdomain, "tunnel_disconnect", hs.Protocol)
}
func (s *Server) reject(conn net.Conn, code, msg string) {
	_ = writeJSONLine(conn, HandshakeResponse{OK: false, ErrorCode: code, Error: msg})
}
func writeJSONLine(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/migandhi/tunnel-software/internal/version"
)

type handshake struct {
	Proto         int    `json:"proto"`
	Token         string `json:"token"`
	Protocol      string `json:"protocol"`
	ClientVersion string `json:"client_version"`
}
type handshakeResponse struct {
	OK        bool   `json:"ok"`
	ErrorCode string `json:"error_code"`
	Error     string `json:"error"`
	Subdomain string `json:"subdomain"`
	URL       string `json:"url"`
	TCPAddr   string `json:"tcp_addr"`
	Warning   string `json:"warning"`
}

var fatalCodes = map[string]bool{"invalid_token": true, "disabled": true, "expired": true, "bandwidth_exceeded": true, "unsupported_proto": true, "bad_request": true, "no_tcp_port": true}

type options struct {
	protocol                 string
	localPort                int
	localHost, server, token string
	insecure, noTLS          bool
}

func usage() {
	fmt.Fprintf(os.Stderr, `tunnel-client %s (protocol v%d)
Usage:
  tunnel-client http <local-port> [flags]
  tunnel-client tcp  <local-port> [flags]

Flags:
  --server host:port
  --token tk_...
  --local-host addr
  --insecure       skip TLS verification (testing only)
  --no-tls         plaintext control channel (development only)

Config file: ~/.tunnel/config
  server = tun.example.com:7000
  token  = tk_xxxxx
`, version.Version, version.ProtocolVersion)
	os.Exit(2)
}

func loadConfigFile(o *options) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(home, ".tunnel", "config"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "server":
			if o.server == "" {
				o.server = v
			}
		case "token":
			if o.token == "" {
				o.token = v
			}
		}
	}
}
func parseArgs() *options {
	if len(os.Args) < 2 {
		usage()
	}
	if os.Args[1] == "version" {
		fmt.Println(version.Version)
		os.Exit(0)
	}
	o := &options{localHost: "127.0.0.1"}
	switch os.Args[1] {
	case "http", "tcp":
		o.protocol = os.Args[1]
	default:
		usage()
	}
	if len(os.Args) < 3 {
		usage()
	}
	p, err := strconv.Atoi(os.Args[2])
	if err != nil || p < 1 || p > 65535 {
		fmt.Fprintln(os.Stderr, "invalid local port")
		os.Exit(2)
	}
	o.localPort = p
	args := os.Args[3:]
	for i := 0; i < len(args); i++ {
		next := func() string {
			i++
			if i >= len(args) {
				usage()
			}
			return args[i]
		}
		switch args[i] {
		case "--server":
			o.server = next()
		case "--token":
			o.token = next()
		case "--local-host":
			o.localHost = next()
		case "--insecure":
			o.insecure = true
		case "--no-tls":
			o.noTLS = true
		default:
			usage()
		}
	}
	if o.server == "" {
		o.server = os.Getenv("TUNNEL_SERVER")
	}
	if o.token == "" {
		o.token = os.Getenv("TUNNEL_TOKEN")
	}
	loadConfigFile(o)
	if os.Getenv("TUNNEL_NO_TLS") == "1" {
		o.noTLS = true
	}
	if o.server == "" || o.token == "" {
		fmt.Fprintln(os.Stderr, "--server and --token are required")
		os.Exit(2)
	}
	return o
}

type fatalError struct{ msg string }

func (e *fatalError) Error() string { return e.msg }

func main() {
	o := parseArgs()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-stop; fmt.Println("\nShutting down."); os.Exit(0) }()
	backoff := 2 * time.Second
	for {
		start := time.Now()
		err := runSession(o)
		if fe, ok := err.(*fatalError); ok {
			fmt.Fprintf(os.Stderr, "Fatal: %s\n", fe.msg)
			os.Exit(1)
		}
		if time.Since(start) > 60*time.Second {
			backoff = 2 * time.Second
		}
		fmt.Printf("Disconnected: %v — reconnecting in %s...\n", err, backoff)
		time.Sleep(backoff)
		if backoff < 60*time.Second {
			backoff *= 2
		}
	}
}
func runSession(o *options) error {
	fmt.Printf("Connecting to %s...\n", o.server)
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var err error
	if o.noTLS {
		conn, err = dialer.Dial("tcp", o.server)
	} else {
		host, _, herr := net.SplitHostPort(o.server)
		if herr != nil {
			return &fatalError{"invalid --server address; expected host:port"}
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", o.server, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: o.insecure})
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	hs, _ := json.Marshal(handshake{Proto: version.ProtocolVersion, Token: o.token, Protocol: o.protocol, ClientVersion: version.Version})
	if _, err := conn.Write(append(hs, '\n')); err != nil {
		return err
	}
	line, err := bufio.NewReaderSize(conn, 8192).ReadSlice('\n')
	if err != nil {
		return fmt.Errorf("no handshake response: %w", err)
	}
	var resp handshakeResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("bad handshake response: %w", err)
	}
	if !resp.OK {
		if fatalCodes[resp.ErrorCode] {
			return &fatalError{resp.Error}
		}
		return fmt.Errorf("server rejected connection: %s", resp.Error)
	}
	_ = conn.SetDeadline(time.Time{})
	fmt.Println("Connected")
	fmt.Printf("Tunnel: %s\nProtocol: %s\nLocal service: %s:%d\n", resp.Subdomain, strings.ToUpper(o.protocol), o.localHost, o.localPort)
	if o.protocol == "http" {
		fmt.Printf("Public URL: %s\n", resp.URL)
	} else {
		fmt.Printf("Public endpoint: %s\n", resp.TCPAddr)
	}
	if resp.Warning != "" {
		fmt.Printf("Warning: %s\n", resp.Warning)
	}
	fmt.Println("Status: Online")
	ycfg := yamux.DefaultConfig()
	ycfg.KeepAliveInterval = 20 * time.Second
	ycfg.ConnectionWriteTimeout = 30 * time.Second
	ycfg.LogOutput = io.Discard
	session, err := yamux.Client(conn, ycfg)
	if err != nil {
		return err
	}
	defer session.Close()
	local := net.JoinHostPort(o.localHost, strconv.Itoa(o.localPort))
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return fmt.Errorf("session closed: %w", err)
		}
		go proxyStream(stream, local)
	}
}
func proxyStream(stream net.Conn, local string) {
	defer stream.Close()
	app, err := net.DialTimeout("tcp", local, 5*time.Second)
	if err != nil {
		fmt.Printf("cannot reach local service %s: %v\n", local, err)
		return
	}
	defer app.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(app, stream); done <- struct{}{} }()
	go func() { _, _ = io.Copy(stream, app); done <- struct{}{} }()
	<-done
	_ = stream.Close()
	_ = app.Close()
	<-done
}

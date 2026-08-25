package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Domain                 string
	ControlAddr            string
	HTTPAddr               string
	HTTPSAddr              string
	AdminAddr              string
	AdminUser              string
	AdminPassHash          string
	DataDir                string
	TLSMode                string
	TLSCertFile            string
	TLSKeyFile             string
	ACMEEmail              string
	TCPPortMin             int
	TCPPortMax             int
	MaxStreamsPerTunnel    int
	StreamWindowKB         int
	MaxConnsPerIP          int
	BandwidthFlushInterval time.Duration
	EnforceInterval        time.Duration
	HandshakeTimeout       time.Duration
	TCPIdleTimeout         time.Duration
	Dev                    bool
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
func getenvDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func Load() (*Config, error) {
	dev := os.Getenv("TUNNEL_DEV") == "1" || os.Getenv("TUNNEL_DEV") == "true"
	c := &Config{
		Domain:                 strings.ToLower(getenv("TUNNEL_DOMAIN", "")),
		ControlAddr:            getenv("TUNNEL_CONTROL_ADDR", ":7000"),
		HTTPAddr:               getenv("TUNNEL_HTTP_ADDR", ":80"),
		HTTPSAddr:              getenv("TUNNEL_HTTPS_ADDR", ":443"),
		AdminAddr:              getenv("TUNNEL_ADMIN_ADDR", "127.0.0.1:9800"),
		AdminUser:              getenv("TUNNEL_ADMIN_USER", "admin"),
		AdminPassHash:          getenv("TUNNEL_ADMIN_PASS_HASH", ""),
		DataDir:                getenv("TUNNEL_DATA_DIR", "/var/lib/tunnel"),
		TLSMode:                getenv("TUNNEL_TLS_MODE", "auto"),
		TLSCertFile:            getenv("TUNNEL_TLS_CERT", ""),
		TLSKeyFile:             getenv("TUNNEL_TLS_KEY", ""),
		ACMEEmail:              getenv("TUNNEL_ACME_EMAIL", ""),
		TCPPortMin:             getenvInt("TUNNEL_TCP_PORT_MIN", 20000),
		TCPPortMax:             getenvInt("TUNNEL_TCP_PORT_MAX", 20249),
		MaxStreamsPerTunnel:    getenvInt("TUNNEL_MAX_STREAMS", 128),
		StreamWindowKB:         getenvInt("TUNNEL_STREAM_WINDOW_KB", 1024),
		MaxConnsPerIP:          getenvInt("TUNNEL_MAX_HANDSHAKES_PER_IP", 12),
		BandwidthFlushInterval: getenvDur("TUNNEL_BW_FLUSH", 15*time.Second),
		EnforceInterval:        getenvDur("TUNNEL_ENFORCE_INTERVAL", 30*time.Second),
		HandshakeTimeout:       getenvDur("TUNNEL_HANDSHAKE_TIMEOUT", 10*time.Second),
		TCPIdleTimeout:         getenvDur("TUNNEL_TCP_IDLE_TIMEOUT", 5*time.Minute),
		Dev:                    dev,
	}
	if dev {
		if c.Domain == "" {
			c.Domain = "localhost"
		}
		if os.Getenv("TUNNEL_TLS_MODE") == "" {
			c.TLSMode = "off"
		}
		if os.Getenv("TUNNEL_HTTP_ADDR") == "" {
			c.HTTPAddr = ":8080"
		}
		if os.Getenv("TUNNEL_DATA_DIR") == "" {
			c.DataDir = "./data"
		}
	}
	return c, c.Validate()
}

func (c *Config) Validate() error {
	if c.Domain == "" {
		return errors.New("TUNNEL_DOMAIN is required")
	}
	switch c.TLSMode {
	case "auto":
	case "static":
		if c.TLSCertFile == "" || c.TLSKeyFile == "" {
			return errors.New("static TLS requires certificate and key")
		}
	case "off":
		if !c.Dev {
			return errors.New("TLS off is only permitted with TUNNEL_DEV=1")
		}
	default:
		return fmt.Errorf("invalid TUNNEL_TLS_MODE %q", c.TLSMode)
	}
	if c.AdminPassHash == "" {
		return errors.New("TUNNEL_ADMIN_PASS_HASH is required")
	}
	if c.TCPPortMin < 1024 || c.TCPPortMax > 65535 || c.TCPPortMin > c.TCPPortMax {
		return errors.New("invalid TCP port range")
	}
	if c.MaxStreamsPerTunnel < 1 {
		return errors.New("TUNNEL_MAX_STREAMS must be positive")
	}
	if c.StreamWindowKB < 64 || c.StreamWindowKB > 8192 {
		return errors.New("TUNNEL_STREAM_WINDOW_KB must be 64-8192")
	}
	return nil
}

func (c *Config) PublicScheme() string {
	if c.TLSMode == "off" {
		return "http"
	}
	return "https"
}

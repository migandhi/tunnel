package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	"github.com/migandhi/tunnel-software/internal/admin"
	"github.com/migandhi/tunnel-software/internal/bandwidth"
	"github.com/migandhi/tunnel-software/internal/config"
	"github.com/migandhi/tunnel-software/internal/control"
	"github.com/migandhi/tunnel-software/internal/proxy"
	"github.com/migandhi/tunnel-software/internal/store"
	"github.com/migandhi/tunnel-software/internal/version"
)

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "run":
		run()
	case "hash-password":
		hashPassword()
	case "version":
		fmt.Println(version.Version)
	default:
		fmt.Fprintln(os.Stderr, "usage: tunnel-server [run|hash-password|version]")
		os.Exit(2)
	}
}
func hashPassword() {
	fmt.Fprint(os.Stderr, "Enter admin password: ")
	pw, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil || len(pw) < 12 {
		fmt.Fprintln(os.Stderr, "error: password must be at least 12 characters")
		os.Exit(1)
	}
	h, err := bcrypt.GenerateFromPassword(pw, 12)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bcrypt error:", err)
		os.Exit(1)
	}
	fmt.Println(string(h))
}

type controller struct {
	reg *control.Registry
	tcp *proxy.TCPManager
}

func (c controller) CloseUser(id int64) int { return c.reg.CloseUser(id) }
func (c controller) ClosePort(p int)        { c.tcp.ClosePort(p) }

func run() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	cfg, err := config.Load()
	if err != nil {
		log.Error("config error", "err", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		log.Error("cannot create data dir", "dir", cfg.DataDir, "err", err)
		os.Exit(1)
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "tunnel.db"))
	if err != nil {
		log.Error("db open failed", "err", err)
		os.Exit(1)
	}
	defer st.Close()
	acct := bandwidth.New(st, log)
	reg := control.NewRegistry()
	tcpMgr := proxy.NewTCPManager(cfg, reg, acct, log)
	httpProxy := proxy.NewHTTPProxy(cfg, reg, acct, log)
	var publicTLS, controlTLS *tls.Config
	var acmeMgr *autocert.Manager
	switch cfg.TLSMode {
	case "auto":
		acmeMgr = &autocert.Manager{Prompt: autocert.AcceptTOS, Email: cfg.ACMEEmail, Cache: autocert.DirCache(filepath.Join(cfg.DataDir, "certs")),
			HostPolicy: func(_ context.Context, host string) error {
				host = strings.ToLower(host)
				if host == cfg.Domain {
					return nil
				}
				sub, ok := proxy.SubdomainOf(host, cfg.Domain)
				if !ok {
					return fmt.Errorf("host %q not under %q", host, cfg.Domain)
				}
				exists, err := st.SubdomainExists(sub)
				if err != nil {
					return err
				}
				if !exists {
					return fmt.Errorf("unknown subdomain %q", sub)
				}
				return nil
			}}
		publicTLS = &tls.Config{GetCertificate: acmeMgr.GetCertificate, MinVersion: tls.VersionTLS12, NextProtos: []string{"h2", "http/1.1", acme.ALPNProto}}
		controlTLS = &tls.Config{GetCertificate: acmeMgr.GetCertificate, MinVersion: tls.VersionTLS12}
	case "static":
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			log.Error("cannot load certificate", "err", err)
			os.Exit(1)
		}
		publicTLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12, NextProtos: []string{"h2", "http/1.1"}}
		controlTLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	case "off":
		log.Warn("TLS disabled; development mode only")
	}
	ctrl := control.NewServer(cfg, st, reg, tcpMgr, log, controlTLS)
	adminSrv, err := admin.New(cfg, st, reg, acct, controller{reg, tcpMgr}, log)
	if err != nil {
		log.Error("admin init failed", "err", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := ctrl.ListenAndServe(); err != nil {
			log.Error("control server failed", "err", err)
		}
	}()
	go func() {
		srv := &http.Server{Addr: cfg.AdminAddr, Handler: adminSrv.Handler(), ReadHeaderTimeout: 10 * time.Second}
		log.Info("admin ui listening", "addr", cfg.AdminAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("admin server failed", "err", err)
		}
	}()
	if cfg.TLSMode == "off" {
		go func() {
			srv := &http.Server{Addr: cfg.HTTPAddr, Handler: httpProxy, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10}
			log.Info("http proxy listening", "addr", cfg.HTTPAddr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error("http proxy failed", "err", err)
			}
		}()
	} else {
		go func() {
			var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				target := "https://" + r.Host + r.URL.RequestURI()
				http.Redirect(w, r, target, http.StatusMovedPermanently)
			})
			if acmeMgr != nil {
				h = acmeMgr.HTTPHandler(h)
			}
			srv := &http.Server{Addr: cfg.HTTPAddr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
			log.Info("http redirect/acme listening", "addr", cfg.HTTPAddr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error("http server failed", "err", err)
			}
		}()
		go func() {
			srv := &http.Server{Addr: cfg.HTTPSAddr, Handler: httpProxy, TLSConfig: publicTLS, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10}
			log.Info("https proxy listening", "addr", cfg.HTTPSAddr)
			if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				log.Error("https proxy failed", "err", err)
			}
		}()
	}
	go acct.Run(ctx, cfg.BandwidthFlushInterval)
	go enforcer(ctx, cfg, st, reg, acct, log)
	log.Info("tunnel-server started", "version", version.Version, "domain", cfg.Domain)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cancel()
	tcpMgr.CloseAll()
	acct.FlushNow()
}
func enforcer(ctx context.Context, cfg *config.Config, st *store.Store, reg *control.Registry, acct *bandwidth.Accountant, log *slog.Logger) {
	t := time.NewTicker(cfg.EnforceInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		acct.FlushNow()
		users, err := st.ListUsers()
		if err != nil {
			log.Error("enforcer list failed", "err", err)
			continue
		}
		now := time.Now()
		for _, u := range users {
			reason := ""
			switch {
			case !u.Active:
				reason = "account disabled"
			case u.Expired(now):
				reason = "subscription expired"
			case u.OverLimit():
				reason = "bandwidth limit reached"
			}
			if reason != "" {
				if n := reg.CloseUser(u.ID); n > 0 {
					log.Info("enforcer disconnected user", "user", u.Subdomain, "reason", reason)
					st.Audit("enforcer", "tunnel_terminated", u.Subdomain+": "+reason)
				}
			}
		}
	}
}

package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/migandhi/tunnel-software/internal/bandwidth"
	"github.com/migandhi/tunnel-software/internal/config"
	"github.com/migandhi/tunnel-software/internal/control"
)

type ctxKey int

const subdomainKey ctxKey = 0

type HTTPProxy struct {
	cfg  *config.Config
	reg  *control.Registry
	acct *bandwidth.Accountant
	log  *slog.Logger
	rp   *httputil.ReverseProxy
	tr   *http.Transport
}

func NewHTTPProxy(cfg *config.Config, reg *control.Registry, acct *bandwidth.Accountant, log *slog.Logger) *HTTPProxy {
	p := &HTTPProxy{cfg: cfg, reg: reg, acct: acct, log: log}
	p.tr = &http.Transport{DialContext: p.dial, MaxIdleConnsPerHost: 4, IdleConnTimeout: 60 * time.Second, ResponseHeaderTimeout: 60 * time.Second, DisableCompression: true}
	p.rp = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			sub, _ := pr.In.Context().Value(subdomainKey).(string)
			pr.SetXForwarded()
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = sub
			pr.Out.Host = pr.In.Host
		},
		Transport: p.tr, FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.Canceled) {
				return
			}
			p.log.Debug("proxy error", "host", r.Host, "err", err)
			writeErrPage(w, http.StatusBadGateway, "Tunnel error", "The tunnel is connected but the local service did not respond correctly.")
		},
	}
	return p
}
func (p *HTTPProxy) dial(_ context.Context, _, addr string) (net.Conn, error) {
	sub, _, err := net.SplitHostPort(addr)
	if err != nil {
		sub = addr
	}
	t := p.reg.GetHTTP(sub)
	if t == nil {
		return nil, control.ErrTunnelOffline
	}
	stream, err := t.OpenStream()
	if err != nil {
		return nil, err
	}
	return p.acct.Track(stream, t.UserID), nil
}
func (p *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := hostOnly(strings.ToLower(r.Host))
	if host == strings.ToLower(p.cfg.Domain) {
		p.serveStatusPage(w, r)
		return
	}
	sub, ok := SubdomainOf(host, strings.ToLower(p.cfg.Domain))
	if !ok {
		writeErrPage(w, http.StatusNotFound, "Not found", "Unknown host.")
		return
	}
	if p.reg.GetHTTP(sub) == nil {
		writeErrPage(w, http.StatusServiceUnavailable, "Tunnel offline", "This tunnel exists but its client is not currently connected.")
		return
	}
	ctx := context.WithValue(r.Context(), subdomainKey, sub)
	p.rp.ServeHTTP(w, r.WithContext(ctx))
}
func (p *HTTPProxy) serveStatusPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html><head><title>Tunnel server</title></head><body style="font-family:sans-serif;max-width:40em;margin:4em auto"><h1>Tunnel server</h1><p>This host serves developer tunnels under <code>*.%s</code>.</p></body></html>`, p.cfg.Domain)
}
func SubdomainOf(host, domain string) (string, bool) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	domain = strings.TrimSuffix(strings.ToLower(domain), ".")
	if !strings.HasSuffix(host, "."+domain) {
		return "", false
	}
	sub := strings.TrimSuffix(host, "."+domain)
	if sub == "" || strings.Contains(sub, ".") {
		return "", false
	}
	return sub, true
}
func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return strings.TrimSuffix(hostport, ":")
}
func writeErrPage(w http.ResponseWriter, code int, title, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `<!doctype html><html><head><title>%s</title></head><body style="font-family:sans-serif;max-width:40em;margin:4em auto"><h1>%s</h1><p>%s</p></body></html>`, title, title, msg)
}

package admin

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/migandhi/tunnel-software/internal/auth"
	"github.com/migandhi/tunnel-software/internal/bandwidth"
	"github.com/migandhi/tunnel-software/internal/config"
	"github.com/migandhi/tunnel-software/internal/control"
	"github.com/migandhi/tunnel-software/internal/security"
	"github.com/migandhi/tunnel-software/internal/store"
	"github.com/migandhi/tunnel-software/internal/version"
)

//go:embed templates/*.html
var tplFS embed.FS

type Controller interface {
	CloseUser(int64) int
	ClosePort(int)
}
type Server struct {
	cfg   *config.Config
	st    *store.Store
	reg   *control.Registry
	acct  *bandwidth.Accountant
	ctl   Controller
	log   *slog.Logger
	tpl   *template.Template
	csrf  string
	start time.Time
}

func New(cfg *config.Config, st *store.Store, reg *control.Registry, acct *bandwidth.Accountant, ctl Controller, log *slog.Logger) (*Server, error) {
	tpl, err := template.ParseFS(tplFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, st: st, reg: reg, acct: acct, ctl: ctl, log: log, tpl: tpl, csrf: hex.EncodeToString(b), start: time.Now()}, nil
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	authed := http.NewServeMux()
	authed.HandleFunc("GET /{$}", s.dashboard)
	authed.HandleFunc("GET /metrics", s.metrics)
	authed.HandleFunc("GET /audit", s.audit)
	authed.HandleFunc("POST /users", s.createUser)
	authed.HandleFunc("POST /users/renew", s.userAction(s.doRenew))
	authed.HandleFunc("POST /users/disable", s.userAction(s.doDisable))
	authed.HandleFunc("POST /users/enable", s.userAction(s.doEnable))
	authed.HandleFunc("POST /users/delete", s.userAction(s.doDelete))
	authed.HandleFunc("POST /users/reset-token", s.userAction(s.doResetToken))
	authed.HandleFunc("POST /users/reset-bandwidth", s.userAction(s.doResetBandwidth))
	authed.HandleFunc("POST /users/set-limit", s.userAction(s.doSetLimit))
	limiter := security.NewLimiter(20, time.Minute)
	mux.Handle("/", auth.BasicAuth(s.cfg.AdminUser, s.cfg.AdminPassHash, limiter, s.csrfGuard(authed)))
	return mux
}
func (s *Server) csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.FormValue("csrf") != s.csrf {
			http.Error(w, "invalid csrf token; reload the dashboard", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type userView struct {
	ID                                                                     int64
	Email, Subdomain, URL, Plan, Used, Limit, Expires, Status, StatusClass string
	TCPPort, UsedPct, DaysLeft                                             int
}
type dashboardView struct {
	Users                      []userView
	Domain, CSRF, Version, Msg string
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	s.acct.FlushNow()
	users, err := s.st.ListUsers()
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	snap := s.reg.Snapshot()
	online := map[int64]bool{}
	for _, t := range snap.Tunnels {
		online[t.UserID] = true
	}
	v := dashboardView{Domain: s.cfg.Domain, CSRF: s.csrf, Version: version.Version, Msg: r.URL.Query().Get("msg")}
	now := time.Now()
	for _, u := range users {
		uv := userView{ID: u.ID, Email: u.Email, Subdomain: u.Subdomain, Plan: u.Plan, TCPPort: u.TCPPort, URL: fmt.Sprintf("%s://%s.%s", s.cfg.PublicScheme(), u.Subdomain, s.cfg.Domain), Used: formatBytes(u.BandwidthUsed), Expires: u.ExpiresAt.Format("2006-01-02")}
		uv.DaysLeft = int(time.Until(u.ExpiresAt).Hours() / 24)
		if u.BandwidthLimit == 0 {
			uv.Limit = "unlimited"
		} else {
			uv.Limit = formatBytes(u.BandwidthLimit)
			uv.UsedPct = int(u.BandwidthUsed * 100 / u.BandwidthLimit)
			if uv.UsedPct > 100 {
				uv.UsedPct = 100
			}
		}
		switch {
		case !u.Active:
			uv.Status, uv.StatusClass = "Disabled", "bad"
		case u.Expired(now):
			uv.Status, uv.StatusClass = "Expired", "bad"
		case u.OverLimit():
			uv.Status, uv.StatusClass = "Over limit", "warn"
		case online[u.ID]:
			uv.Status, uv.StatusClass = "Online", "good"
		default:
			uv.Status, uv.StatusClass = "Offline", "muted"
		}
		v.Users = append(v.Users, uv)
	}
	if err := s.tpl.ExecuteTemplate(w, "dashboard.html", v); err != nil {
		s.log.Error("dashboard template failed", "err", err)
	}
}
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	sub := r.FormValue("subdomain")
	plan := r.FormValue("plan")
	if plan == "" {
		plan = "free"
	}
	days, _ := strconv.Atoi(r.FormValue("days"))
	if days <= 0 {
		days = 30
	}
	var limit int64
	if gb, err := strconv.Atoi(r.FormValue("limit_gb")); err == nil && gb > 0 {
		limit = int64(gb) << 30
	}
	tcpPort := 0
	switch tp := r.FormValue("tcp"); tp {
	case "", "none":
	case "auto":
		p, err := s.st.AllocateTCPPort(s.cfg.TCPPortMin, s.cfg.TCPPortMax)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		tcpPort = p
	default:
		p, err := strconv.Atoi(tp)
		if err != nil || p < s.cfg.TCPPortMin || p > s.cfg.TCPPortMax {
			http.Error(w, "invalid TCP port", 400)
			return
		}
		used, _ := s.st.UsedTCPPorts()
		if used[p] {
			http.Error(w, "TCP port already assigned", 409)
			return
		}
		tcpPort = p
	}
	token, err := auth.GenerateToken()
	if err != nil {
		http.Error(w, "token generation failed", 500)
		return
	}
	u, err := s.st.CreateUser(email, sub, plan, auth.HashToken(token), tcpPort, limit, days)
	if err != nil {
		http.Error(w, "create failed: "+err.Error(), 400)
		return
	}
	s.st.Audit("admin", "user_create", fmt.Sprintf("%s plan=%s port=%d days=%d", sub, plan, tcpPort, days))
	s.showToken(w, u, token)
}
func (s *Server) userAction(fn func(http.ResponseWriter, *http.Request, *store.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", 400)
			return
		}
		u, err := s.st.GetUserByID(id)
		if err != nil {
			http.Error(w, "user not found", 404)
			return
		}
		fn(w, r, u)
	}
}
func (s *Server) doRenew(w http.ResponseWriter, r *http.Request, u *store.User) {
	days, _ := strconv.Atoi(r.FormValue("days"))
	if days <= 0 {
		days = 30
	}
	reset := r.FormValue("reset_bandwidth") == "on"
	if err := s.st.Renew(u.ID, days, reset); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.st.Audit("admin", "user_renew", fmt.Sprintf("%s days=%d reset=%v", u.Subdomain, days, reset))
	redirectMsg(w, r, "renewed "+u.Subdomain)
}
func (s *Server) doDisable(w http.ResponseWriter, r *http.Request, u *store.User) {
	if err := s.st.SetActive(u.ID, false); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	n := s.ctl.CloseUser(u.ID)
	s.st.Audit("admin", "user_disable", fmt.Sprintf("%s sessions_closed=%d", u.Subdomain, n))
	redirectMsg(w, r, "disabled "+u.Subdomain)
}
func (s *Server) doEnable(w http.ResponseWriter, r *http.Request, u *store.User) {
	if err := s.st.SetActive(u.ID, true); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.st.Audit("admin", "user_enable", u.Subdomain)
	redirectMsg(w, r, "enabled "+u.Subdomain)
}
func (s *Server) doDelete(w http.ResponseWriter, r *http.Request, u *store.User) {
	s.ctl.CloseUser(u.ID)
	if u.TCPPort > 0 {
		s.ctl.ClosePort(u.TCPPort)
	}
	if err := s.st.DeleteUser(u.ID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.st.Audit("admin", "user_delete", u.Subdomain)
	redirectMsg(w, r, "deleted "+u.Subdomain)
}
func (s *Server) doResetToken(w http.ResponseWriter, r *http.Request, u *store.User) {
	token, err := auth.GenerateToken()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := s.st.SetToken(u.ID, auth.HashToken(token)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.ctl.CloseUser(u.ID)
	s.st.Audit("admin", "token_reset", u.Subdomain)
	s.showToken(w, u, token)
}
func (s *Server) doResetBandwidth(w http.ResponseWriter, r *http.Request, u *store.User) {
	if err := s.st.ResetBandwidth(u.ID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.st.Audit("admin", "bandwidth_reset", u.Subdomain)
	redirectMsg(w, r, "bandwidth reset for "+u.Subdomain)
}
func (s *Server) doSetLimit(w http.ResponseWriter, r *http.Request, u *store.User) {
	gb, err := strconv.Atoi(r.FormValue("limit_gb"))
	if err != nil || gb < 0 {
		http.Error(w, "bad limit", 400)
		return
	}
	if err := s.st.SetBandwidthLimit(u.ID, int64(gb)<<30); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.st.Audit("admin", "limit_set", fmt.Sprintf("%s limit=%dGB", u.Subdomain, gb))
	redirectMsg(w, r, "limit updated for "+u.Subdomain)
}
func (s *Server) showToken(w http.ResponseWriter, u *store.User, token string) {
	_ = s.tpl.ExecuteTemplate(w, "token.html", map[string]any{"User": u, "Token": token, "Domain": s.cfg.Domain, "Scheme": s.cfg.PublicScheme()})
}
func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	snap := s.reg.Snapshot()
	users, _ := s.st.ListUsers()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"version": version.Version, "uptime_seconds": int(time.Since(s.start).Seconds()), "goroutines": runtime.NumGoroutine(), "heap_bytes": mem.HeapAlloc, "users_total": len(users), "tunnels_http": snap.HTTPTunnels, "tunnels_tcp": snap.TCPTunnels, "tunnels": snap.Tunnels})
}
func (s *Server) audit(w http.ResponseWriter, _ *http.Request) {
	lines, err := s.st.RecentAudit(200)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
}
func redirectMsg(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, fmt.Sprintf("/?msg=%s", template.URLQueryEscaper(msg)), http.StatusSeeOther)
}
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

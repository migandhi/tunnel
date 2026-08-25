package bandwidth

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/migandhi/tunnel-software/internal/store"
)

type Accountant struct {
	mu      sync.Mutex
	pending map[int64]int64
	st      *store.Store
	log     *slog.Logger
}

func New(st *store.Store, log *slog.Logger) *Accountant {
	return &Accountant{pending: make(map[int64]int64), st: st, log: log}
}
func (a *Accountant) Add(userID, n int64) {
	if n <= 0 {
		return
	}
	a.mu.Lock()
	a.pending[userID] += n
	a.mu.Unlock()
}
func (a *Accountant) Pending(userID int64) int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pending[userID]
}
func (a *Accountant) FlushNow() {
	a.mu.Lock()
	batch := a.pending
	a.pending = make(map[int64]int64)
	a.mu.Unlock()
	for id, n := range batch {
		if err := a.st.AddBandwidth(id, n); err != nil {
			a.log.Error("bandwidth flush failed", "user", id, "bytes", n, "err", err)
			a.Add(id, n)
		}
	}
}
func (a *Accountant) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			a.FlushNow()
			return
		case <-t.C:
			a.FlushNow()
		}
	}
}
func (a *Accountant) Track(c net.Conn, userID int64) net.Conn {
	return &trackedConn{Conn: c, a: a, id: userID}
}

type trackedConn struct {
	net.Conn
	a  *Accountant
	id int64
}

func (t *trackedConn) Read(b []byte) (int, error) {
	n, err := t.Conn.Read(b)
	t.a.Add(t.id, int64(n))
	return n, err
}
func (t *trackedConn) Write(b []byte) (int, error) {
	n, err := t.Conn.Write(b)
	t.a.Add(t.id, int64(n))
	return n, err
}

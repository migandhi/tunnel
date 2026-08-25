package bandwidth

import (
	"github.com/migandhi/tunnel-software/internal/store"
	"log/slog"
	"path/filepath"
	"testing"
)

func TestAccumulateAndFlush(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	u, _ := st.CreateUser("", "bwtest", "basic", "h1", 0, 0, 30)
	a := New(st, slog.Default())
	a.Add(u.ID, 100)
	a.Add(u.ID, 250)
	if a.Pending(u.ID) != 350 {
		t.Fatal("pending count wrong")
	}
	a.FlushNow()
	got, _ := st.GetUserByID(u.ID)
	if got.BandwidthUsed != 350 {
		t.Fatalf("want 350 got %d", got.BandwidthUsed)
	}
}

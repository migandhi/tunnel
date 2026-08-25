package store

import (
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestValidateSubdomain(t *testing.T) {
	for _, s := range []string{"a", "myapp", "my-app", "abc123", "a1-b2"} {
		if err := ValidateSubdomain(s); err != nil {
			t.Errorf("valid %q: %v", s, err)
		}
	}
	for _, s := range []string{"", "-x", "x-", "UPPER", "has.dot", "admin", "www", "sp ace", "under_score"} {
		if err := ValidateSubdomain(s); err == nil {
			t.Errorf("expected invalid %q", s)
		}
	}
}
func TestCreateLookupRenewExpiry(t *testing.T) {
	s := testStore(t)
	u, err := s.CreateUser("a@b.c", "myapp", "basic", "hash1", 0, 50<<30, 30)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetUserByTokenHash("hash1")
	if err != nil || got.ID != u.ID {
		t.Fatalf("lookup failed: %v", err)
	}
	if got.Expired(time.Now()) {
		t.Fatal("new user should not be expired")
	}
	if !got.Expired(time.Now().AddDate(0, 0, 31)) {
		t.Fatal("should expire after 31 days")
	}
	if err := s.Renew(u.ID, 60, true); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetUserByID(u.ID)
	if got.Expired(time.Now().AddDate(0, 0, 31)) {
		t.Fatal("renewed too early")
	}
	if got.BandwidthUsed != 0 {
		t.Fatal("bandwidth not reset")
	}
}
func TestSubdomainUniqueness(t *testing.T) {
	s := testStore(t)
	if _, err := s.CreateUser("", "dup", "free", "h1", 0, 0, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("", "dup", "free", "h2", 0, 0, 30); err == nil {
		t.Fatal("expected uniqueness error")
	}
}
func TestPortAllocation(t *testing.T) {
	s := testStore(t)
	p, _ := s.AllocateTCPPort(20000, 20002)
	if p != 20000 {
		t.Fatalf("want 20000 got %d", p)
	}
	s.CreateUser("", "u1", "pro", "h1", p, 0, 30)
	p, _ = s.AllocateTCPPort(20000, 20002)
	if p != 20001 {
		t.Fatalf("want 20001 got %d", p)
	}
	s.CreateUser("", "u2", "pro", "h2", p, 0, 30)
	s.CreateUser("", "u3", "pro", "h3", 20002, 0, 30)
	if _, err := s.AllocateTCPPort(20000, 20002); err != ErrNoFreePorts {
		t.Fatalf("expected no free ports: %v", err)
	}
}
func TestBandwidthAccounting(t *testing.T) {
	s := testStore(t)
	u, _ := s.CreateUser("", "bw", "basic", "h1", 0, 100, 30)
	s.AddBandwidth(u.ID, 60)
	s.AddBandwidth(u.ID, 50)
	got, _ := s.GetUserByID(u.ID)
	if got.BandwidthUsed != 110 || !got.OverLimit() {
		t.Fatalf("bandwidth accounting failed: %+v", got)
	}
}

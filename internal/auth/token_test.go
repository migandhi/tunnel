package auth

import (
	"strings"
	"testing"
)

func TestGenerateTokenFormat(t *testing.T) {
	tok, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, TokenPrefix) {
		t.Fatalf("missing prefix: %s", tok)
	}
	if len(tok) != len(TokenPrefix)+48 {
		t.Fatalf("unexpected length %d", len(tok))
	}
}

func TestGenerateTokenUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		tok, err := GenerateToken()
		if err != nil {
			t.Fatal(err)
		}
		if seen[tok] {
			t.Fatal("duplicate token generated")
		}
		seen[tok] = true
	}
}

func TestHashToken(t *testing.T) {
	a := HashToken("tk_aaa")
	if a != HashToken("tk_aaa") {
		t.Fatal("hash not deterministic")
	}
	if a == HashToken("tk_aab") {
		t.Fatal("unexpected collision")
	}
	if len(a) != 64 {
		t.Fatalf("unexpected hash length %d", len(a))
	}
}

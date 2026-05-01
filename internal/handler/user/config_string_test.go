package user

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestBuildConfigString_A_NME_B(t *testing.T) {
	a := buildConfigString("a", "https://nme.local", map[string]string{"ak": "ak1", "whsec": "ws1"})
	b := buildConfigString("b", "https://nme.local", map[string]string{"bk": "bk1", "bsk": "bs1"})

	for _, cs := range []string{a, b} {
		parts := strings.Split(cs, ".")
		if len(parts) != 3 || parts[0] != "NME1" {
			t.Fatalf("invalid config format: %s", cs)
		}
		mac := hmac.New(sha256.New, []byte("nme-config-v1"))
		_, _ = mac.Write([]byte(parts[1]))
		wantSig := hex.EncodeToString(mac.Sum(nil))
		if parts[2] != wantSig {
			t.Fatalf("invalid config signature: %s", cs)
		}
	}
}


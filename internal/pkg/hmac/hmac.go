package hmacutil

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// Sign computes HMAC-SHA256 of an arbitrary string payload.
func Sign(secret, payload string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(payload))
	return hex.EncodeToString(m.Sum(nil))
}

// Verify checks a simple string-based HMAC (legacy, used by inbound v9 simple sig).
func Verify(secret, payload, got string) bool {
	return hmac.Equal([]byte(Sign(secret, payload)), []byte(got))
}

// ── Body-hash HMAC (v8-compatible, used for B-station and A-station calls) ──

// hashBody returns hex(sha256(body)).
func hashBody(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// buildPayload constructs the canonical signing string:
//
//	apiKey + "\n" + timestamp + "\n" + sha256(body)
func buildPayload(apiKey string, ts int64, body []byte) string {
	return fmt.Sprintf("%s\n%d\n%s", apiKey, ts, hashBody(body))
}

// BuildHeaders generates outbound request headers for SS→B-station or SS→A-station calls.
// Returns a map with X-Api-Key, X-Timestamp, X-Signature.
func BuildHeaders(apiKey, secret string, body []byte) map[string]string {
	ts := time.Now().Unix()
	sig := Sign(secret, buildPayload(apiKey, ts, body))
	return map[string]string{
		"X-Api-Key":   apiKey,
		"X-Timestamp": strconv.FormatInt(ts, 10),
		"X-Signature": sig,
	}
}

// VerifyBodyRequest validates an inbound request that uses the body-hash scheme.
// windowSeconds is the allowed clock skew (e.g. 300).
func VerifyBodyRequest(apiKey string, ts int64, sig string, body []byte, secret string, windowSeconds int64) bool {
	delta := time.Now().Unix() - ts
	if delta < 0 {
		delta = -delta
	}
	if delta > windowSeconds {
		return false
	}
	expected := Sign(secret, buildPayload(apiKey, ts, body))
	return hmac.Equal([]byte(expected), []byte(sig))
}

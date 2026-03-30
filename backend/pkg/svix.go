package pkg

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// VerifySvixSignature verifies a Svix webhook signature.
// Returns true if signature is valid, false otherwise.
func VerifySvixSignature(payload, signatureHeader, timestamp, secret string) (bool, error) {
	if signatureHeader == "" {
		return false, fmt.Errorf("missing signature header")
	}
	if timestamp == "" {
		return false, fmt.Errorf("missing timestamp")
	}

	// Parse signature header: "v1,signature1 v2,signature2 ..."
	signatures := strings.Split(signatureHeader, " ")
	if len(signatures) == 0 {
		return false, fmt.Errorf("invalid signature format")
	}

	// Expected signature format: "v1,digest"
	expectedPrefix := "v1,"

	// Check for v1 signature
	for _, sig := range signatures {
		if !strings.HasPrefix(sig, expectedPrefix) {
			continue
		}

		// Create HMAC: sha256(timestamp + payload)
		h := hmac.New(sha256.New, []byte(secret))
		h.Write([]byte(timestamp))
		h.Write([]byte(payload))
		expectedDigest := hex.EncodeToString(h.Sum(nil))

		// Compare with provided signature (without "v1," prefix)
		providedDigest := sig[len(expectedPrefix):]

		// Constant-time comparison to prevent timing attacks
		return hmac.Equal([]byte(expectedDigest), []byte(providedDigest)), nil
	}

	return false, fmt.Errorf("no v1 signature found")
}

// VerifySvixTimestamp verifies the timestamp is within tolerance (in seconds).
// Prevents replay attacks by rejecting timestamps too far in the past or future.
// The default tolerance should be 60 seconds for security.
func VerifySvixTimestamp(timestampStr string, toleranceSeconds int) error {
	// Enforce maximum tolerance to prevent accepting old timestamps
	maxTolerance := 60
	if toleranceSeconds > maxTolerance {
		toleranceSeconds = maxTolerance
	}

	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	now := time.Now().Unix()
	diff := now - timestamp

	// Reject future timestamps - they should not be accepted even within tolerance
	if diff < 0 {
		return fmt.Errorf("timestamp is in the future: diff=%d", diff)
	}

	// Only check for timestamps too old
	if diff > int64(toleranceSeconds) {
		return fmt.Errorf("timestamp too old: diff=%d seconds", diff)
	}

	return nil
}

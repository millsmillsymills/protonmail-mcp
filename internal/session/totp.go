package session

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// generateTOTP returns the current 6-digit TOTP for a base32 secret. Accepts
// either the raw seed or an otpauth:// URI.
func generateTOTP(secret string) (string, error) {
	seed := secret
	if strings.HasPrefix(secret, "otpauth://") {
		i := strings.Index(secret, "secret=")
		if i < 0 {
			return "", fmt.Errorf("otpauth URI missing secret")
		}
		rest := secret[i+len("secret="):]
		if amp := strings.IndexByte(rest, '&'); amp >= 0 {
			rest = rest[:amp]
		}
		seed = rest
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.ReplaceAll(seed, " ", "")))
	if err != nil {
		return "", fmt.Errorf("base32 decode: %w", err)
	}
	counter := uint64(time.Now().Unix() / 30)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0F
	bin := (uint32(sum[off])&0x7F)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	return fmt.Sprintf("%06d", bin%1_000_000), nil
}

// GenerateTOTPForTest is exported only for unit tests.
var GenerateTOTPForTest = generateTOTP

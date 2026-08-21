package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TOTP implementation based on RFC 6238 (Google Authenticator compatible).
// Compatible with Alibaba Cloud APP, Tencent Cloud APP, Huawei Cloud APP,
// WeChat identity verification mini program, and all other authenticators
// that support the Google Authenticator protocol.

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret generates a new TOTP secret (base32, no padding, length 32).
func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, 20) // 160-bit, Google Authenticator default
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return b32.EncodeToString(buf), nil
}

// BuildOTPAuthURL builds an otpauth:// URI for the frontend to generate a QR code for binding.
func BuildOTPAuthURL(secret, account, issuer string) string {
	if issuer == "" {
		issuer = "OpsMini"
	}
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	return fmt.Sprintf("otpauth://totp/%s?%s", label, q.Encode())
}

// ValidateTOTP validates a TOTP code, tolerating ±1 time step (30s) of clock skew.
func ValidateTOTP(secret, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	if _, err := strconv.Atoi(code); err != nil {
		return false
	}
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(key) == 0 {
		return false
	}
	now := time.Now().Unix()
	for _, step := range []int64{0, -1, 1} {
		if totpAt(key, now/30+step) == code {
			return true
		}
	}
	return false
}

// totpAt computes the 6-digit TOTP code for the given time step.
func totpAt(key []byte, counter int64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[offset])&0x7f)<<24 |
		(uint32(sum[offset+1]))<<16 |
		(uint32(sum[offset+2]))<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%06d", bin%1000000)
}

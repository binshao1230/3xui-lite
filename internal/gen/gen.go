package gen

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/curve25519"
)

// FreePort picks an unused TCP port starting from prefer, falling back to random.
func FreePort(prefer int, used map[int]bool) int {
	try := func(p int) bool {
		if p <= 0 || p > 65535 || used[p] {
			return false
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", p))
		if err != nil {
			return false
		}
		_ = ln.Close()
		return true
	}
	if prefer > 0 && try(prefer) {
		return prefer
	}
	for _, p := range []int{8443, 10443, 20443, 2053, 2083, 2087, 2096, 10888, 20000, 30000, 40000, 443} {
		if try(p) {
			return p
		}
	}
	for p := 10000; p < 50000; p++ {
		if try(p) {
			return p
		}
	}
	return 0
}

// RandomBase64 returns n random bytes as standard base64 (for SS2022 keys).
func RandomBase64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// RandomHex returns n random bytes as hex.
func RandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	const hexdigits = "0123456789abcdef"
	out := make([]byte, n*2)
	for i, v := range b {
		out[i*2] = hexdigits[v>>4]
		out[i*2+1] = hexdigits[v&0x0f]
	}
	return string(out)
}

// ShortID returns a random Reality shortId (8 hex chars).
func ShortID() string {
	return RandomHex(4)
}

// RealityKeyPair generates X25519 keys in base64.RawURLEncoding (Xray/sing-box style).
func RealityKeyPair() (privateKey, publicKey string, err error) {
	var priv [32]byte
	if _, err = rand.Read(priv[:]); err != nil {
		return "", "", err
	}
	// X25519 clamp
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)

	// Xray REALITY uses RawURLEncoding (no padding, - and _)
	privateKey = base64.RawURLEncoding.EncodeToString(priv[:])
	publicKey = base64.RawURLEncoding.EncodeToString(pub[:])
	return privateKey, publicKey, nil
}

// RealityKeyPairFromXray prefers `xray x25519` for exact compatibility.
// Newer Xray prints:
//
//	PrivateKey: xxx
//	Password (PublicKey): yyy
//
// Older builds may print PublicKey: yyy
func RealityKeyPairFromXray(xrayBin string) (privateKey, publicKey string, err error) {
	if xrayBin == "" {
		return RealityKeyPair()
	}
	if st, e := exec.LookPath(xrayBin); e == nil {
		xrayBin = st
	}
	cmd := exec.Command(xrayBin, "x25519")
	if dir := filepath.Dir(xrayBin); dir != "" && dir != "." {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return RealityKeyPair()
	}
	var priv, pub string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// split on first ':'
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		low := strings.ToLower(key)
		switch {
		case strings.Contains(low, "private"):
			priv = val
		case strings.Contains(low, "public"):
			// matches "PublicKey" and "Password (PublicKey)"
			pub = val
		}
	}
	if priv == "" || pub == "" {
		return RealityKeyPair()
	}
	return priv, pub, nil
}

// RandomPath for WS.
func RandomPath() string {
	return "/" + RandomHex(8)
}

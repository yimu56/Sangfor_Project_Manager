package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const iterations = 120000

func pbkdf2(password, salt []byte, iter, keyLen int) []byte {
	h := sha256.New
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	var buf [4]byte
	dk := make([]byte, 0, numBlocks*hashLen)
	u := make([]byte, hashLen)
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)
		prf.Write(buf[:4])
		dk = prf.Sum(dk)
		t := dk[len(dk)-hashLen:]
		copy(u, t)
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = u[:0]
			u = prf.Sum(u)
			for x := range u {
				t[x] ^= u[x]
			}
		}
	}
	return dk[:keyLen]
}

// Hash 生成 pbkdf2$iter$salt$hash 格式串
func Hash(password string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	dk := pbkdf2([]byte(password), salt, iterations, 32)
	return fmt.Sprintf("pbkdf2$%d$%s$%s", iterations, hex.EncodeToString(salt), hex.EncodeToString(dk))
}

func Verify(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	var iter int
	if _, err := fmt.Sscanf(parts[1], "%d", &iter); err != nil || iter <= 0 {
		return false
	}
	salt, err1 := hex.DecodeString(parts[2])
	want, err2 := hex.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got := pbkdf2([]byte(password), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ---------- 内存会话 ----------

type Sessions struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func NewSessions() *Sessions { return &Sessions{m: map[string]time.Time{}} }

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (s *Sessions) Create() string {
	t := randomToken(32)
	s.mu.Lock()
	s.m[t] = time.Now().Add(30 * 24 * time.Hour)
	s.mu.Unlock()
	return t
}

func (s *Sessions) Valid(t string) bool {
	if t == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.m[t]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.m, t)
		return false
	}
	return true
}

func (s *Sessions) Drop(t string) {
	s.mu.Lock()
	delete(s.m, t)
	s.mu.Unlock()
}

func (s *Sessions) DropAll() {
	s.mu.Lock()
	s.m = map[string]time.Time{}
	s.mu.Unlock()
}

func NewResetToken() string { return strings.ToUpper(randomToken(8)) }

var ErrInvalid = errors.New("invalid credentials")

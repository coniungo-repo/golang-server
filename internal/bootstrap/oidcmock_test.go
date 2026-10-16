package bootstrap

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const mockKeyID = "test-key"

// mockOIDC is a minimal OpenID Connect provider: discovery, JWKS, and a token endpoint that returns the ID token registered for each code.
type mockOIDC struct {
	*httptest.Server
	key   *rsa.PrivateKey
	mu    sync.Mutex
	codes map[string]jwt.MapClaims
}

func newMockOIDC(t *testing.T) *mockOIDC {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := &mockOIDC{key: key, codes: map[string]jwt.MapClaims{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{
			"issuer":                 m.URL,
			"authorization_endpoint": m.URL + "/authorize",
			"token_endpoint":         m.URL + "/token",
			"userinfo_endpoint":      m.URL + "/userinfo",
			"jwks_uri":               m.URL + "/jwks",
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": mockKeyID,
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		m.mu.Lock()
		claims, ok := m.codes[r.Form.Get("code")]
		m.mu.Unlock()
		if !ok {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"access_token": "mock-access", "token_type": "Bearer", "id_token": m.sign(t, claims)})
	})
	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{})
	})
	m.Server = httptest.NewServer(mux)
	t.Cleanup(m.Close)
	return m
}

// code registers claims and returns a one-off authorisation code for them. Defaults make a valid token.
func (m *mockOIDC) code(clientID, sub, addr string, overrides jwt.MapClaims) string {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": m.URL, "aud": clientID, "sub": sub, "email": addr, "email_verified": true,
		"hd": "coniungolabs.com", "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	}
	for k, v := range overrides {
		if v == nil {
			delete(claims, k)
		} else {
			claims[k] = v
		}
	}
	code := base64.RawURLEncoding.EncodeToString(randomBytes(12))
	m.mu.Lock()
	m.codes[code] = claims
	m.mu.Unlock()
	return code
}

func (m *mockOIDC) sign(t *testing.T, claims jwt.MapClaims) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = mockKeyID
	s, err := tok.SignedString(m.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

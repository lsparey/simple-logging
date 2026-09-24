package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func writeHTPasswd(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "htpasswd")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func hashOf(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}

func TestLoadHTPasswd_AcceptsEveryBcryptPrefix(t *testing.T) {
	hash := hashOf(t, "secret") // $2a$
	content := strings.Join([]string{
		"# comment",
		"",
		"alice:" + hash,
		"bob:" + strings.Replace(hash, "$2a$", "$2y$", 1), // what `htpasswd -B` writes
		"carol:" + strings.Replace(hash, "$2a$", "$2b$", 1),
	}, "\n")
	b, err := LoadHTPasswd(writeHTPasswd(t, content))
	if err != nil {
		t.Fatalf("LoadHTPasswd: %v", err)
	}
	if b.Users() != 3 {
		t.Fatalf("Users: got %d, want 3", b.Users())
	}
	for _, user := range []string{"alice", "bob", "carol"} {
		if !b.check(user, "secret") {
			t.Errorf("%s: correct password rejected", user)
		}
	}
}

func TestLoadHTPasswd_RejectsNonBcrypt(t *testing.T) {
	for name, content := range map[string]string{
		"apr1":     "alice:$apr1$abc$def",
		"sha":      "alice:{SHA}abcdef",
		"no colon": "alice",
		"empty":    "# nobody\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadHTPasswd(writeHTPasswd(t, content)); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	b, err := LoadHTPasswd(writeHTPasswd(t, "alice:"+hashOf(t, "secret")))
	if err != nil {
		t.Fatal(err)
	}
	handler := b.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "/healthz")

	for _, tc := range []struct {
		name       string
		path       string
		user, pass string
		want       int
	}{
		{"open path without credentials", "/healthz", "", "", http.StatusOK},
		{"no credentials", "/", "", "", http.StatusUnauthorized},
		{"wrong password", "/", "alice", "nope", http.StatusUnauthorized},
		{"unknown user", "/", "mallory", "secret", http.StatusUnauthorized},
		{"valid", "/", "alice", "secret", http.StatusOK},
		{"valid again (cached)", "/simplelog.v1.LogService/ListNamespaces", "alice", "secret", http.StatusOK},
		{"open path is exact, not a prefix", "/healthz/x", "", "", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.user != "" {
				req.SetBasicAuth(tc.user, tc.pass)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status: got %d, want %d", rec.Code, tc.want)
			}
			if tc.want == http.StatusUnauthorized && !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Basic ") {
				t.Errorf("missing Basic challenge, got %q", rec.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

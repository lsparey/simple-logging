// Package auth provides the optional built-in HTTP basic authentication.
//
// It is deliberately minimal, meant for small installs without an
// authenticating proxy in front: users and bcrypt password hashes come from
// an htpasswd file (as written by `htpasswd -B`), loaded once at startup.
// Anything more (OIDC, groups, per-namespace access) belongs in a proxy at
// the edge, such as oauth2-proxy.
package auth

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// realm is sent in WWW-Authenticate so browsers show their login prompt.
const realm = "simple-logging"

// maxCachedCredentials bounds the verified-credentials cache (see Basic).
const maxCachedCredentials = 256

// Basic checks HTTP basic credentials against bcrypt hashes from an htpasswd
// file.
//
// bcrypt is slow by design (tens of milliseconds per check), and a browser
// resends credentials with every request — each static asset, API call and
// dashboard poll — so successful checks are cached by a SHA-256 of the
// username and password. Only credentials that already verified against
// bcrypt are cached, so the cache can't be used to guess a password any
// faster than bcrypt allows.
type Basic struct {
	users map[string][]byte // username -> bcrypt hash

	mu       sync.Mutex
	verified map[[sha256.Size]byte]struct{}
}

// LoadHTPasswd reads an htpasswd file of "user:hash" lines. Blank lines and
// lines starting with # are ignored. Only bcrypt hashes ($2a$, $2b$, $2y$)
// are accepted; any other scheme is an error rather than being silently
// unusable.
func LoadHTPasswd(path string) (*Basic, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	users := make(map[string][]byte)
	scanner := bufio.NewScanner(f)
	for lineNum := 1; scanner.Scan(); lineNum++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		user, hash, ok := strings.Cut(line, ":")
		if !ok || user == "" || hash == "" {
			return nil, fmt.Errorf("%s:%d: expected user:hash", path, lineNum)
		}
		if !isBcrypt(hash) {
			return nil, fmt.Errorf("%s:%d: user %q does not have a bcrypt hash (create it with `htpasswd -B`)", path, lineNum, user)
		}
		users[user] = []byte(hash)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, errors.New(path + ": no users")
	}
	return &Basic{users: users, verified: make(map[[sha256.Size]byte]struct{})}, nil
}

func isBcrypt(hash string) bool {
	for _, prefix := range []string{"$2a$", "$2b$", "$2y$"} {
		if strings.HasPrefix(hash, prefix) {
			return true
		}
	}
	return false
}

// Users returns how many users were loaded.
func (b *Basic) Users() int {
	return len(b.users)
}

// Middleware requires valid credentials on every request except those whose
// path is in open (e.g. health probes), answering 401 with a
// WWW-Authenticate challenge otherwise.
func (b *Basic) Middleware(next http.Handler, open ...string) http.Handler {
	openPaths := make(map[string]bool, len(open))
	for _, p := range open {
		openPaths[p] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if openPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		user, password, ok := r.BasicAuth()
		if !ok || !b.check(user, password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// check reports whether password is valid for user.
func (b *Basic) check(user, password string) bool {
	key := sha256.Sum256([]byte(user + "\x00" + password))

	b.mu.Lock()
	_, cached := b.verified[key]
	b.mu.Unlock()
	if cached {
		return true
	}

	hash, known := b.users[user]
	if !known {
		// Compare against a fixed hash anyway so an unknown user takes as
		// long as a wrong password, and usernames can't be probed by timing.
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
		return false
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return false
	}

	b.mu.Lock()
	if len(b.verified) >= maxCachedCredentials {
		clear(b.verified)
	}
	b.verified[key] = struct{}{}
	b.mu.Unlock()
	return true
}

// dummyHash is a bcrypt hash at the default cost, used to equalise timing
// for unknown usernames. Built on first use so it costs nothing when basic
// auth is off.
var dummyHash = sync.OnceValue(func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("simple-logging-unknown-user"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return h
})

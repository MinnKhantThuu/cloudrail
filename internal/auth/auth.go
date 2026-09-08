package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var ErrCredentials = errors.New("Email or password is incorrect")
var ErrSetup = errors.New("Owner has already been configured")

type Auth struct {
	DB     *pgxpool.Pool
	Secure bool
}

func (a *Auth) Configured(ctx context.Context) (bool, error) {
	var configured bool
	err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM owners)`).Scan(&configured)
	return configured, err
}
func (a *Auth) Setup(ctx context.Context, email, password string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 {
		return errors.New("Enter a valid email address")
	}
	if len(password) < 12 || len(password) > 72 {
		return errors.New("Password must be 12–72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	tag, err := a.DB.Exec(ctx, `INSERT INTO owners(id,email,password_hash) VALUES(true,$1,$2) ON CONFLICT(id) DO NOTHING`, email, string(hash))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrSetup
	}
	return nil
}
func hashToken(token string) string {
	v := sha256.Sum256([]byte(token))
	return hex.EncodeToString(v[:])
}
func (a *Auth) Login(ctx context.Context, email, password string) (string, error) {
	if len(password) > 72 {
		return "", ErrCredentials
	}
	var stored, ownerEmail string
	err := a.DB.QueryRow(ctx, `SELECT email,password_hash FROM owners WHERE id=true`).Scan(&ownerEmail, &stored)
	// Use a fixed-cost comparison even for a missing account/email.
	if err != nil {
		stored = "$2a$12$eRPE7oNVl5M6sLM6L/Le2.87g2t9w6FCPbM0RpFqxjQ18grR1MIPa"
	}
	passwordErr := bcrypt.CompareHashAndPassword([]byte(stored), []byte(password))
	if err != nil || passwordErr != nil || ownerEmail != strings.ToLower(strings.TrimSpace(email)) {
		return "", ErrCredentials
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	_, err = a.DB.Exec(ctx, `INSERT INTO sessions(token_hash,expires_at) VALUES($1,now()+interval '24 hours')`, hashToken(token))
	return token, err
}
func (a *Auth) Session(ctx context.Context, r *http.Request) (string, error) {
	c, err := r.Cookie("cloudrail_session")
	if err != nil {
		return "", ErrCredentials
	}
	var email string
	err = a.DB.QueryRow(ctx, `SELECT email FROM owners WHERE EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1 AND expires_at>now())`, hashToken(c.Value)).Scan(&email)
	return email, err
}
func (a *Auth) Cookie(w http.ResponseWriter, token string) {
	maxAge := 86400
	if token == "" {
		maxAge = -1
	}
	http.SetCookie(w, &http.Cookie{Name: "cloudrail_session", Value: token, Path: "/", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}
func (a *Auth) Logout(ctx context.Context, r *http.Request) error {
	c, err := r.Cookie("cloudrail_session")
	if err != nil {
		return nil
	}
	_, err = a.DB.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, hashToken(c.Value))
	return err
}
func (a *Auth) AllowAttempt(ctx context.Context, r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	var count int
	err = a.DB.QueryRow(ctx, `INSERT INTO auth_attempts(key,attempts) VALUES($1,1) ON CONFLICT(key) DO UPDATE SET attempts=CASE WHEN auth_attempts.window_start<now()-interval '15 minutes' THEN 1 ELSE auth_attempts.attempts+1 END,window_start=CASE WHEN auth_attempts.window_start<now()-interval '15 minutes' THEN now() ELSE auth_attempts.window_start END RETURNING attempts`, hashToken(ip)).Scan(&count)
	return err == nil && count <= 20
}
func SameOrigin(r *http.Request) bool {
	if r.Method == "GET" || r.Method == "HEAD" {
		return true
	}
	if r.Header.Get("X-Cloudrail-Request") != "1" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host != r.Host {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
func (a *Auth) Cleanup(ctx context.Context) {
	_, _ = a.DB.Exec(ctx, `DELETE FROM sessions WHERE expires_at<now(); DELETE FROM auth_attempts WHERE window_start<now()-interval '1 day'`)
}

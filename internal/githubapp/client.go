package githubapp

import (
	"bytes"
	"cloudrail/internal/secrets"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Client struct {
	DB      *pgxpool.Pool
	Cipher  *secrets.Cipher
	HTTP    *http.Client
	BaseURL string
}
type Configuration struct {
	AppID         string `json:"appId"`
	Slug          string `json:"slug"`
	PrivateKey    string `json:"privateKey"`
	WebhookSecret string `json:"webhookSecret"`
}

func New(db *pgxpool.Pool, cipher *secrets.Cipher) *Client {
	return &Client{DB: db, Cipher: cipher, BaseURL: "https://api.github.com", HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || (req.URL.Host != "api.github.com" && req.URL.Host != "codeload.github.com") {
			return errors.New("archive redirect rejected")
		}
		req.Header.Del("Authorization")
		return nil
	}}}
}
func parseKey(key string) (*rsa.PrivateKey, error) {
	p, _ := pem.Decode([]byte(key))
	if p == nil {
		return nil, errors.New("invalid PEM private key")
	}
	if k, e := x509.ParsePKCS1PrivateKey(p.Bytes); e == nil {
		return k, nil
	}
	k, e := x509.ParsePKCS8PrivateKey(p.Bytes)
	if e != nil {
		return nil, e
	}
	r, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("RSA private key required")
	}
	return r, nil
}
func (c *Client) Save(ctx context.Context, v Configuration) error {
	if id, e := strconv.ParseInt(v.AppID, 10, 64); e != nil || id <= 0 {
		return errors.New("invalid app ID")
	}
	if v.Slug == "" || len(v.Slug) > 100 || len(v.WebhookSecret) < 32 {
		return errors.New("app slug and webhook secret (32+ characters) required")
	}
	if _, e := parseKey(v.PrivateKey); e != nil {
		return e
	}
	k, e := c.Cipher.Seal(v.PrivateKey, "github:private-key")
	if e != nil {
		return e
	}
	s, e := c.Cipher.Seal(v.WebhookSecret, "github:webhook")
	if e != nil {
		return e
	}
	_, e = c.DB.Exec(ctx, `INSERT INTO github_app(id,app_id,slug,private_key,webhook_secret) VALUES(true,$1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET app_id=$1,slug=$2,private_key=$3,webhook_secret=$4`, v.AppID, v.Slug, k, s)
	return e
}
func (c *Client) config(ctx context.Context) (Configuration, error) {
	var v Configuration
	var k, s []byte
	e := c.DB.QueryRow(ctx, `SELECT app_id,slug,private_key,webhook_secret FROM github_app`).Scan(&v.AppID, &v.Slug, &k, &s)
	if e != nil {
		return v, e
	}
	v.PrivateKey, e = c.Cipher.Open(k, "github:private-key")
	if e != nil {
		return v, e
	}
	v.WebhookSecret, e = c.Cipher.Open(s, "github:webhook")
	return v, e
}
func JWT(id, key string) (string, error) {
	k, e := parseKey(key)
	if e != nil {
		return "", e
	}
	claims, _ := json.Marshal(map[string]any{"iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(8 * time.Minute).Unix(), "iss": id})
	body := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(claims)
	h := sha256.Sum256([]byte(body))
	sig, e := rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, h[:])
	if e != nil {
		return "", e
	}
	return body + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
func (c *Client) request(ctx context.Context, method, path, token string, body any) (*http.Response, error) {
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	req, e := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "Cloudrail")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r, e := c.HTTP.Do(req)
	if e != nil {
		return nil, errors.New("GitHub connection failed; retry shortly")
	}
	if r.StatusCode >= 300 {
		r.Body.Close()
		return nil, fmt.Errorf("GitHub returned HTTP %d", r.StatusCode)
	}
	return r, nil
}
func (c *Client) get(ctx context.Context, path, token string, out any) error {
	r, e := c.request(ctx, "GET", path, token, nil)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(out)
}
func (c *Client) Token(ctx context.Context, installation int64) (string, error) {
	if installation == 0 {
		return "", nil
	}
	v, e := c.config(ctx)
	if e != nil {
		return "", errors.New("configure GitHub App first")
	}
	jwt, e := JWT(v.AppID, v.PrivateKey)
	if e != nil {
		return "", e
	}
	r, e := c.request(ctx, "POST", "/app/installations/"+strconv.FormatInt(installation, 10)+"/access_tokens", jwt, map[string]any{"permissions": map[string]string{"contents": "read", "metadata": "read"}})
	if e != nil {
		return "", e
	}
	defer r.Body.Close()
	var b struct {
		Token string `json:"token"`
	}
	e = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&b)
	if e == nil && b.Token == "" {
		e = errors.New("GitHub returned no installation token")
	}
	return b.Token, e
}
func (c *Client) Commit(ctx context.Context, installation int64, repository, branch string) (string, error) {
	token, e := c.Token(ctx, installation)
	if e != nil {
		return "", e
	}
	var v struct {
		SHA string `json:"sha"`
	}
	e = c.get(ctx, "/repos/"+repository+"/commits/"+url.PathEscape(branch), token, &v)
	return v.SHA, e
}
func (c *Client) Archive(ctx context.Context, installation int64, repo, sha string) (*http.Response, error) {
	token, e := c.Token(ctx, installation)
	if e != nil {
		return nil, e
	}
	return c.request(ctx, "GET", "/repos/"+repo+"/tarball/"+sha, token, nil)
}
func (c *Client) List(ctx context.Context, kind string, installation int64, repo string, page int) (any, error) {
	if page < 1 || page > 100 {
		return nil, errors.New("invalid page")
	}
	var token, path string
	var e error
	switch kind {
	case "installations":
		v, err := c.config(ctx)
		if err != nil {
			return nil, err
		}
		token, e = JWT(v.AppID, v.PrivateKey)
		path = "/app/installations"
	case "repositories":
		token, e = c.Token(ctx, installation)
		if installation == 0 {
			return nil, errors.New("installation required")
		}
		path = "/installation/repositories"
	case "branches":
		token, e = c.Token(ctx, installation)
		path = "/repos/" + repo + "/branches"
	default:
		return nil, errors.New("invalid listing")
	}
	if e != nil {
		return nil, e
	}
	var out any
	e = c.get(ctx, path+"?per_page=100&page="+strconv.Itoa(page), token, &out)
	return out, e
}
func Verify(secret string, body []byte, signature string) bool {
	if len(secret) < 1 {
		return false
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	expected := "sha256=" + hex.EncodeToString(m.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}
func (c *Client) VerifyWebhook(ctx context.Context, body []byte, signature string) bool {
	v, e := c.config(ctx)
	return e == nil && Verify(v.WebhookSecret, body, signature)
}

package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"
)

// S3Storage persists uploads in any S3-compatible object store: Utho Object
// Storage, Cloudflare R2, MinIO, or AWS itself. Production uses this so photos
// never sit on the app server's disk — they are served straight from object
// storage, which is what lets a client cache them instead of re-fetching the
// image on every poll.
type S3Storage struct {
	endpoint   string // e.g. https://innoida.utho.io
	region     string
	bucket     string
	accessKey  string
	secretKey  string
	publicBase string // optional CDN / custom domain in front of the bucket
	client     *http.Client
}

// emptySHA256 is the SHA-256 of a zero-length body.
const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func NewS3Storage(endpoint, region, bucket, accessKey, secretKey, publicBase string) (*S3Storage, error) {
	if endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("s3 storage requires S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY and S3_SECRET_KEY")
	}
	if region == "" {
		region = "us-east-1"
	}

	return &S3Storage{
		endpoint:   strings.TrimRight(endpoint, "/"),
		region:     region,
		bucket:     bucket,
		accessKey:  accessKey,
		secretKey:  secretKey,
		publicBase: strings.TrimRight(publicBase, "/"),
		// Object uploads are small (a few hundred KB); a tight timeout keeps a
		// stalled object store from holding a request open indefinitely.
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (s *S3Storage) objectURL(key string) string {
	return s.endpoint + "/" + s.bucket + "/" + key
}

// publicURL is what gets stored in the database and handed to clients.
func (s *S3Storage) publicURL(key string) string {
	if s.publicBase != "" {
		return s.publicBase + "/" + key
	}
	return s.objectURL(key)
}

func (s *S3Storage) Save(ctx context.Context, data []byte, contentType string) (string, error) {
	key := GenerateFilename(contentType)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.objectURL(key), bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to build object upload request: %w", err)
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", contentType)

	sum := sha256.Sum256(data)
	s.sign(req, hex.EncodeToString(sum[:]), time.Now())

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("object upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return "", fmt.Errorf("object upload rejected (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return s.publicURL(key), nil
}

func (s *S3Storage) Delete(ctx context.Context, fileURL string) error {
	key := path.Base(fileURL)
	if key == "" || key == "." || key == "/" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.objectURL(key), nil)
	if err != nil {
		return fmt.Errorf("failed to build object delete request: %w", err)
	}
	s.sign(req, emptySHA256, time.Now())

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("object delete request failed: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (s *S3Storage) sign(req *http.Request, payloadHash string, now time.Time) {
	signRequest(req, payloadHash, s.accessKey, s.secretKey, s.region, "s3", now)
}

// signRequest applies AWS Signature Version 4 to req and returns the canonical
// request it signed (returned so tests can pin the exact wire format).
//
// This is a deliberately small implementation covering the two calls we make —
// PUT and DELETE, with no query string and unadorned object keys. It is
// verified against the official AWS signing-key test vector in s3_test.go.
//
// ponytail: no presigning, multipart, or chunked payloads. Add aws-sdk-go-v2 if
// uploads ever need those.
func signRequest(req *http.Request, payloadHash, accessKey, secretKey, region, service string, now time.Time) string {
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// Signed headers are the host plus whatever content-type / x-amz-* headers
	// the caller set, in sorted order. Nothing else is signed, so transport
	// headers (user-agent, content-length) can vary without breaking the
	// signature.
	type header struct{ name, value string }
	headers := []header{{"host", req.URL.Host}}
	for name, values := range req.Header {
		lower := strings.ToLower(name)
		if lower == "content-type" || strings.HasPrefix(lower, "x-amz-") {
			headers = append(headers, header{lower, strings.TrimSpace(strings.Join(values, ","))})
		}
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].name < headers[j].name })

	var canonicalHeaders strings.Builder
	signedNames := make([]string, 0, len(headers))
	for _, h := range headers {
		canonicalHeaders.WriteString(h.name + ":" + h.value + "\n")
		signedNames = append(signedNames, h.name)
	}
	signedHeaders := strings.Join(signedNames, ";")

	// req.URL.Path (not EscapedPath) because the keys this driver generates are
	// UUIDs plus an extension, i.e. already RFC3986-unreserved characters.
	canonicalRequest := strings.Join([]string{
		req.Method,
		uriEncodePath(req.URL.Path),
		canonicalQueryString(req.URL.RawQuery),
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	digest := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(digest[:]),
	}, "\n")

	signature := hex.EncodeToString(hmacSHA256(deriveSigningKey(secretKey, dateStamp, region, service), []byte(stringToSign)))

	req.Header.Set("Authorization",
		"AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+
			", SignedHeaders="+signedHeaders+
			", Signature="+signature)

	return canonicalRequest
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// deriveSigningKey walks the AWS4 key derivation chain: date, then region,
// then service, then the fixed terminator.
func deriveSigningKey(secretKey, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

// uriEncodePath percent-encodes everything outside the RFC3986 unreserved set,
// leaving path separators alone.
func uriEncodePath(p string) string {
	if p == "" {
		return "/"
	}
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~', c == '/':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// canonicalQueryString encodes query parameters the way SigV4 requires: keys
// and values percent-encoded and sorted.
//
// ponytail: we never actually sign a query (uploads are plain PUT/DELETE), so
// this stays simple. It is needed if presigned URLs are added later.
func canonicalQueryString(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}

	keys := []string{}
	values := map[string][]string{}
	for _, pair := range strings.Split(rawQuery, "&") {
		if pair == "" {
			continue
		}
		key, value, _ := strings.Cut(pair, "=")
		if _, seen := values[key]; !seen {
			keys = append(keys, key)
		}
		values[key] = append(values[key], value)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		vs := values[key]
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, uriEncode(key)+"="+uriEncode(v))
		}
	}
	return strings.Join(parts, "&")
}

func uriEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

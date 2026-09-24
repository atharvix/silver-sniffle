package middleware

import "net/http"

// BodySizeLimit caps how many bytes a handler will read from a request body.
// Without this an oversized payload (e.g. a huge base64 photo) is buffered in
// full while decoding, which is a cheap memory-exhaustion vector.
func BodySizeLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

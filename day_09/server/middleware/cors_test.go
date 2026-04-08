package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS(t *testing.T) {
	// Create a simple handler that we'll wrap
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	handler := CORS(baseHandler)

	tests := []struct {
		name            string
		method          string
		origin          string
		wantAllowOrigin string
		wantHeaders     map[string]string
	}{
		{
			name:            "GET request with origin",
			method:          "GET",
			origin:          "https://example.com",
			wantAllowOrigin: "https://example.com",
			wantHeaders: map[string]string{
				"Access-Control-Allow-Methods":     "GET, POST, OPTIONS",
				"Access-Control-Allow-Headers":     "Content-Type, Authorization",
				"Access-Control-Allow-Credentials": "true",
			},
		},
		{
			name:            "POST request without origin",
			method:          "POST",
			origin:          "",
			wantAllowOrigin: "*",
			wantHeaders: map[string]string{
				"Access-Control-Allow-Methods":     "GET, POST, OPTIONS",
				"Access-Control-Allow-Headers":     "Content-Type, Authorization",
				"Access-Control-Allow-Credentials": "true",
			},
		},
		{
			name:            "OPTIONS preflight request",
			method:          "OPTIONS",
			origin:          "https://test.com",
			wantAllowOrigin: "https://test.com",
			wantHeaders: map[string]string{
				"Access-Control-Allow-Methods":     "GET, POST, OPTIONS",
				"Access-Control-Allow-Headers":     "Content-Type, Authorization",
				"Access-Control-Allow-Credentials": "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			// Check Access-Control-Allow-Origin
			allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
			if allowOrigin != tt.wantAllowOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", allowOrigin, tt.wantAllowOrigin)
			}

			// Check other CORS headers
			for key, wantValue := range tt.wantHeaders {
				gotValue := resp.Header.Get(key)
				if gotValue != wantValue {
					t.Errorf("%s = %q, want %q", key, gotValue, wantValue)
				}
			}

			// For OPTIONS request, middleware should respond with 200 and not call the inner handler
			if tt.method == "OPTIONS" {
				if resp.StatusCode != http.StatusOK {
					t.Errorf("OPTIONS request status = %d, want %d", resp.StatusCode, http.StatusOK)
				}
				if w.Body.String() != "" {
					t.Errorf("OPTIONS request body should be empty, got %q", w.Body.String())
				}
			} else {
				if resp.StatusCode != http.StatusOK {
					t.Errorf("request status = %d, want %d", resp.StatusCode, http.StatusOK)
				}
				if w.Body.String() != "OK" {
					t.Errorf("request body = %q, want %q", w.Body.String(), "OK")
				}
			}
		})
	}
}

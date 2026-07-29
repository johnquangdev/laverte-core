package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
)

func TestHealthEndpoint(t *testing.T) {
	srv := NewServer(config.Config{FrontendURL: "http://localhost:3000"}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != `{"ok":true}`+"\n" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestSplitOrigins(t *testing.T) {
	got := splitOrigins("http://a.com, http://b.com,,http://c.com")
	want := []string{"http://a.com", "http://b.com", "http://c.com"}
	if len(got) != len(want) {
		t.Fatalf("splitOrigins() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitOrigins()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

package internal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadPreservesCacheOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "rules.json")
	if err := os.WriteFile(target, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if err := DownloadFile(server.URL, target, true); err == nil {
		t.Fatal("expected HTTP error")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "previous" {
		t.Fatalf("cache replaced: %q, %v", data, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file leaked: %v, %v", entries, err)
	}
}

func TestDownloadPublishesCompleteFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("complete"))
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "nested", "rules.json")
	if err := DownloadFile(server.URL, target, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "complete" {
		t.Fatalf("download: %q, %v", data, err)
	}
	if BoxCreateHttpClient(nil).Timeout <= 0 {
		t.Fatal("unbounded client")
	}
}

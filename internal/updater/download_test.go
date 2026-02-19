package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDownloaderDownloadToFile(t *testing.T) {
	t.Run("successful download", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "13")
			w.Write([]byte("file contents"))
		}))
		defer server.Close()

		d := NewDownloader(server.Client())
		dest := filepath.Join(t.TempDir(), "downloaded")

		err := d.DownloadToFile(context.Background(), server.URL, dest, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("reading file: %v", err)
		}

		if string(data) != "file contents" {
			t.Errorf("content = %q, want %q", string(data), "file contents")
		}
	})

	t.Run("progress callback", func(t *testing.T) {
		body := strings.Repeat("x", 1000)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "1000")
			w.Write([]byte(body))
		}))
		defer server.Close()

		d := NewDownloader(server.Client())
		dest := filepath.Join(t.TempDir(), "downloaded")

		var callCount atomic.Int32

		err := d.DownloadToFile(context.Background(), server.URL, dest, func(_, total int64) {
			callCount.Add(1)

			if total != 1000 {
				t.Errorf("total = %d, want 1000", total)
			}
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if callCount.Load() == 0 {
			t.Error("progress callback was never called")
		}
	})

	t.Run("HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		d := NewDownloader(server.Client())
		dest := filepath.Join(t.TempDir(), "downloaded")

		err := d.DownloadToFile(context.Background(), server.URL, dest, nil)
		if err == nil {
			t.Fatal("expected error for HTTP 404")
		}

		if !strings.Contains(err.Error(), "404") {
			t.Errorf("error = %q, want to contain '404'", err.Error())
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("content"))
		}))
		defer server.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		d := NewDownloader(server.Client())
		dest := filepath.Join(t.TempDir(), "downloaded")

		err := d.DownloadToFile(ctx, server.URL, dest, nil)
		if err == nil {
			t.Fatal("expected error for cancelled context")
		}
	})
}

func TestDownloaderDownloadToString(t *testing.T) {
	t.Run("successful download", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("string content"))
		}))
		defer server.Close()

		d := NewDownloader(server.Client())

		got, err := d.DownloadToString(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != "string content" {
			t.Errorf("content = %q, want %q", got, "string content")
		}
	})

	t.Run("HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		d := NewDownloader(server.Client())

		_, err := d.DownloadToString(context.Background(), server.URL)
		if err == nil {
			t.Fatal("expected error for HTTP 500")
		}
	})
}

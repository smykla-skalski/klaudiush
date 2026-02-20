package updater_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/updater"
)

var _ = Describe("Downloader", func() {
	Describe("DownloadToFile", func() {
		It("downloads file successfully", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Length", "13")
					_, _ = w.Write([]byte("file contents"))
				}),
			)
			defer server.Close()

			d := updater.NewDownloader(server.Client())
			dest := filepath.Join(GinkgoT().TempDir(), "downloaded")

			Expect(d.DownloadToFile(context.Background(), server.URL, dest, nil)).To(Succeed())

			data, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal("file contents"))
		})

		It("invokes progress callback", func() {
			body := strings.Repeat("x", 1000)

			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Length", "1000")
					_, _ = w.Write([]byte(body))
				}),
			)
			defer server.Close()

			d := updater.NewDownloader(server.Client())
			dest := filepath.Join(GinkgoT().TempDir(), "downloaded")

			var callCount atomic.Int32

			err := d.DownloadToFile(context.Background(), server.URL, dest, func(_, total int64) {
				callCount.Add(1)
				Expect(total).To(Equal(int64(1000)))
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(callCount.Load()).To(BeNumerically(">", 0))
		})

		It("returns error on HTTP 404", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}),
			)
			defer server.Close()

			d := updater.NewDownloader(server.Client())
			dest := filepath.Join(GinkgoT().TempDir(), "downloaded")

			err := d.DownloadToFile(context.Background(), server.URL, dest, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("404"))
		})

		It("returns error on context cancellation", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte("content"))
				}),
			)
			defer server.Close()

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			d := updater.NewDownloader(server.Client())
			dest := filepath.Join(GinkgoT().TempDir(), "downloaded")

			err := d.DownloadToFile(ctx, server.URL, dest, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("DownloadToString", func() {
		It("downloads content as string", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte("string content"))
				}),
			)
			defer server.Close()

			d := updater.NewDownloader(server.Client())

			got, err := d.DownloadToString(context.Background(), server.URL)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal("string content"))
		})

		It("returns error on HTTP 500", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}),
			)
			defer server.Close()

			d := updater.NewDownloader(server.Client())

			_, err := d.DownloadToString(context.Background(), server.URL)
			Expect(err).To(HaveOccurred())
		})
	})
})

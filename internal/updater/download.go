package updater

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/cockroachdb/errors"
)

// ProgressFunc is called during download with bytes received and total bytes.
// Total may be -1 if the server doesn't send Content-Length.
type ProgressFunc func(received, total int64)

// Downloader handles HTTP downloads.
type Downloader struct {
	client *http.Client
}

// NewDownloader creates a new Downloader with the given HTTP client.
func NewDownloader(client *http.Client) *Downloader {
	if client == nil {
		client = http.DefaultClient
	}

	return &Downloader{client: client}
}

// DownloadToFile downloads a URL to a local file path.
//
//nolint:gosec // G304/G704: URL and destPath are constructed internally by the updater, not user-controlled
func (d *Downloader) DownloadToFile(
	ctx context.Context,
	url, destPath string,
	progress ProgressFunc,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(err, "creating request")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "downloading file")
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on response body

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return errors.Wrap(err, "creating destination file")
	}

	var reader io.Reader = resp.Body

	if progress != nil {
		reader = &progressReader{
			reader:   resp.Body,
			total:    resp.ContentLength,
			callback: progress,
		}
	}

	if _, copyErr := io.Copy(out, reader); copyErr != nil {
		_ = out.Close()

		return errors.Wrap(copyErr, "writing download to file")
	}

	return out.Close()
}

// DownloadToString downloads a URL and returns the body as a string.
//
//nolint:gosec // G704: URL is constructed internally by the updater, not user-controlled
func (d *Downloader) DownloadToString(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", errors.Wrap(err, "creating request")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "downloading content")
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on response body

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "reading response body")
	}

	return string(data), nil
}

// progressReader wraps an io.Reader and reports progress.
type progressReader struct {
	reader   io.Reader
	total    int64
	received int64
	callback ProgressFunc
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.received += int64(n)

	if r.callback != nil {
		r.callback(r.received, r.total)
	}

	return n, err
}

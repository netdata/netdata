// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const acceptHeader = `text/plain;version=0.0.4;q=1,*/*;q=0.1`

// fetcher writes the raw exposition text for one scrape into w. A single
// prometheus instance owns one fetcher and reuses it across scrapes.
type fetcher interface {
	fetch(ctx context.Context, w io.Writer) error
}

// fileFetcher reads the exposition text from a local file (file:// URLs).
type fileFetcher struct {
	path string
}

func (f *fileFetcher) fetch(_ context.Context, w io.Writer) error {
	file, err := os.Open(f.path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(w, file)

	return err
}

// httpFetcher scrapes the exposition text over HTTP, transparently decompressing
// gzip responses. The gzip reader and its buffered source are reused across scrapes.
type httpFetcher struct {
	client  *http.Client
	request web.RequestConfig

	gzipr   *gzip.Reader
	bodyBuf *bufio.Reader
}

func (f *httpFetcher) fetch(ctx context.Context, w io.Writer) error {
	req, err := web.NewHTTPRequest(f.request)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	req.Header.Add("Accept", acceptHeader)
	req.Header.Add("Accept-Encoding", "gzip")

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}

	defer web.CloseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server '%s' returned HTTP status code %d (%s)", req.URL, resp.StatusCode, resp.Status)
	}

	if resp.Header.Get("Content-Encoding") != "gzip" {
		_, err = io.Copy(w, resp.Body)
		return err
	}

	if f.gzipr == nil {
		f.bodyBuf = bufio.NewReader(resp.Body)
		f.gzipr, err = gzip.NewReader(f.bodyBuf)
		if err != nil {
			return err
		}
	} else {
		f.bodyBuf.Reset(resp.Body)
		_ = f.gzipr.Reset(f.bodyBuf)
	}

	_, err = io.Copy(w, f.gzipr)
	_ = f.gzipr.Close()

	return err
}

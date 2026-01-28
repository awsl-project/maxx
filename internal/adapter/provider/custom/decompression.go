package custom

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// nopCloser wraps a reader and makes Close() a no-op
// Used to prevent double-closing when decompressResponse returns resp.Body directly
type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

// decompressResponse returns a reader that decompresses the response body
// based on the Content-Encoding header.
// The returned reader's Close() does NOT close the underlying resp.Body -
// the caller is responsible for closing resp.Body separately.
func decompressResponse(resp *http.Response) (io.ReadCloser, error) {
	encoding := resp.Header.Get("Content-Encoding")
	if encoding == "" {
		// Wrap in nopCloser to prevent double-close when caller defers reader.Close()
		// while resp.Body.Close() is also deferred elsewhere
		return nopCloser{resp.Body}, nil
	}

	for _, enc := range strings.Split(encoding, ",") {
		enc = strings.TrimSpace(strings.ToLower(enc))
		switch enc {
		case "gzip":
			// gzip.Reader.Close() does NOT close the underlying reader
			return gzip.NewReader(resp.Body)
		case "deflate":
			// flate.Reader.Close() does NOT close the underlying reader
			return flate.NewReader(resp.Body), nil
		case "br":
			// brotli.Reader has no Close method, wrap with nopCloser
			return nopCloser{brotli.NewReader(resp.Body)}, nil
		case "zstd":
			// zstd decoder.Close() does NOT close the underlying reader
			decoder, err := zstd.NewReader(resp.Body)
			if err != nil {
				return nil, err
			}
			return decoder.IOReadCloser(), nil
		}
	}
	// Unknown encoding, wrap in nopCloser
	return nopCloser{resp.Body}, nil
}

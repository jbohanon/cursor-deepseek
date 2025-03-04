package deepseek

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"

	"github.com/andybalholm/brotli"
	"github.com/pkg/errors"
)

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func copyHeaders(dst, src http.Header) {
	// Headers to skip
	skipHeaders := map[string]bool{
		"Content-Length":    true,
		"Content-Encoding":  true,
		"Transfer-Encoding": true,
		"Connection":        true,
	}

	for k, vv := range src {
		if !skipHeaders[k] {
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}

func readResponse(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body

	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, errors.Wrap(err, "error creating gzip reader")
		}
		defer gzReader.Close()
		reader = gzReader
	case "br":
		reader = brotli.NewReader(resp.Body)
	case "deflate":
		reader = flate.NewReader(resp.Body)
	}

	return io.ReadAll(reader)
}

package httputil

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
)

const (
	httpRequestKey = "_http_request"
)

func getRequest(ctx context.Context) *http.Request {
	r, _ := ctx.Value(httpRequestKey).(*http.Request)
	return r
}

func readRequestBody(r *http.Request) (string, error) {
	if r.Body == nil {
		return "", nil
	}

	// Check Content-Type for JSON
	contentType := r.Header.Get("Content-Type")
	contentType = strings.TrimSpace(contentType)
	switch contentType {
	case "application/json":
	case "application/json; charset=utf-8":
	default:
		return "", nil
	}

	// Read the body
	var buf bytes.Buffer
	tee := io.TeeReader(r.Body, &buf)
	body, err := io.ReadAll(tee)
	if err != nil {
		return "", err
	}

	// Replace the original body with a new ReadCloser for further reading
	r.Body = io.NopCloser(&buf)
	return string(body), nil
}

func readResBody(r *http.Response) (string, error) {
	if r.Body == nil {
		return "", nil
	}

	// Check Content-Type for JSON
	contentType := r.Header.Get("Content-Type")
	contentType = strings.TrimSpace(contentType)
	switch contentType {
	case "application/json":
	case "application/json; charset=utf-8":
	default:
		if r.StatusCode == http.StatusOK {
			return "", nil
		}
	}

	// Read the body
	var buf bytes.Buffer
	tee := io.TeeReader(r.Body, &buf)
	body, err := io.ReadAll(tee)
	if err != nil {
		return "", err
	}

	// Replace the original body with a new ReadCloser for further reading
	r.Body = io.NopCloser(&buf)
	return string(body), nil
}

package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
)

func wrapResponseWriter(w http.ResponseWriter) *responseWriterWrapper {
	return &responseWriterWrapper{ResponseWriter: w}
}

// responseWriterWrapper is a minimal wrapper for http.ResponseWriter that allows the
// written HTTP status code to be captured for logging.
// adapted from: https://github.com/elithrar/admission-control/blob/df0c4bf37a96d159d9181a71cee6e5485d5a50a9/request_logger.go#L11-L13
type responseWriterWrapper struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// Status provides access to the wrapped http.ResponseWriter's status
func (rw *responseWriterWrapper) Status() int {
	return rw.status
}

// Header provides access to the wrapped http.ResponseWriter's header
// allowing handlers to set HTTP headers on the wrapped response
func (rw *responseWriterWrapper) Header() http.Header {
	return rw.ResponseWriter.Header()
}

// WriteHeader intercepts the written status code and caches it
// so that we can access it later
func (rw *responseWriterWrapper) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}

	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
	rw.wroteHeader = true

	return
}

// Flush implements http.Flusher
func (rw *responseWriterWrapper) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rw *responseWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("http.Hijacker not implemented")
}

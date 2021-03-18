package middleware

import "net/http"

func wrapResponseWriter(w http.ResponseWriter) *responseWriterWrapper {
	return &responseWriterWrapper{ResponseWriter: w}
}

// responseWriterWrapper is a minimal wrapper for http.ResponseWriter that allows the
// written HTTP status code to be captured for logging.
// adapted from: https://github.com/elithrar/admission-control/blob/df0c4bf37a96d159d9181a71cee6e5485d5a50a9/request_logger.go#L11-L13
type responseWriterWrapper struct {
	http.ResponseWriter
	status       int
	wroteHeader  bool
	bytesWritten []byte
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

func (rw *responseWriterWrapper) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}

	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
	rw.wroteHeader = true

	return
}


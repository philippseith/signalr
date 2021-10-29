package mux

import (
	"net/http"

	"github.com/philippseith/signalr"
)

func WithHttpServeMux(serveMux *http.ServeMux) func() signalr.MappableRouter {
	return func() signalr.MappableRouter {
		return serveMux
	}
}

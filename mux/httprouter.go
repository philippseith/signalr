package mux

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/philippseith/signalr"
)

func WithHttpRouter(router *httprouter.Router) func() signalr.MappableRouter {
	return func() signalr.MappableRouter {
		return &julienRouter{router: router}
	}
}

type julienRouter struct {
	router *httprouter.Router
}

func (j *julienRouter) HandleFunc(path string, handler func(w http.ResponseWriter, r *http.Request)) {
	j.router.HandlerFunc("POST", path, handler)
}

func (j *julienRouter) Handle(pattern string, handler http.Handler) {
	j.router.Handler("POST", pattern, handler)
	j.router.Handler("GET", pattern, handler)
}

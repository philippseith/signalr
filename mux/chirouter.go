package mux

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/philippseith/signalr"
)

func WithChiRouter(router chi.Router) func() signalr.MappableRouter {
	return func() signalr.MappableRouter {
		return &chiRouter{router: router}
	}
}

type chiRouter struct {
	router chi.Router
}

func (j *chiRouter) HandleFunc(path string, handler func(w http.ResponseWriter, r *http.Request)) {
	j.router.HandleFunc(path, handler)
}

func (j *chiRouter) Handle(pattern string, handler http.Handler) {
	j.router.Handle(pattern, handler)
}

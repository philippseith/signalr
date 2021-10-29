package mux

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/philippseith/signalr"
)

func WithGorillaRouter(router *mux.Router) func() signalr.MappableRouter {
	return func() signalr.MappableRouter {
		return &gorillaMappableRouter{router: router}
	}
}

type gorillaMappableRouter struct {
	router *mux.Router
}

func (g *gorillaMappableRouter) Handle(path string, handler http.Handler) {
	g.router.Handle(path, handler)
}

func (g *gorillaMappableRouter) HandleFunc(path string, handleFunc func(w http.ResponseWriter, r *http.Request)) {
	g.router.HandleFunc(path, handleFunc)
}

package router

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/philippseith/signalr"
)

// WithGorillaRouter is a signalr.MappableRouter factory for signalr.Server.MapHTTP
// which converts a mux.Router to a signalr.MappableRouter.
func WithGorillaRouter(r *mux.Router) func() signalr.MappableRouter {
	return func() signalr.MappableRouter {
		return &gorillaMappableRouter{r: r}
	}
}

type gorillaMappableRouter struct {
	r *mux.Router
}

func (g *gorillaMappableRouter) Handle(path string, handler http.Handler) {
	g.r.Handle(path, handler)
}

func (g *gorillaMappableRouter) HandleFunc(path string, handleFunc func(w http.ResponseWriter, r *http.Request)) {
	g.r.HandleFunc(path, handleFunc)
}

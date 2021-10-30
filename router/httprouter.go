package router

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/philippseith/signalr"
)

// WithHttpRouter is a signalr.MappableRouter factory for signalr.Server.MapHTTP
// which converts a httprouter.Router to a signalr.MappableRouter.
func WithHttpRouter(r *httprouter.Router) func() signalr.MappableRouter {
	return func() signalr.MappableRouter {
		return &julienRouter{r: r}
	}
}

type julienRouter struct {
	r *httprouter.Router
}

func (j *julienRouter) HandleFunc(path string, handler func(w http.ResponseWriter, r *http.Request)) {
	j.r.HandlerFunc("POST", path, handler)
}

func (j *julienRouter) Handle(pattern string, handler http.Handler) {
	j.r.Handler("POST", pattern, handler)
	j.r.Handler("GET", pattern, handler)
}

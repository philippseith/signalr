package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/philippseith/signalr"
)

// WithChiRouter is a signalr.MappableRouter factory for signalr.Server.MapHTTP
// which converts a chi.Router to a signalr.MappableRouter.
func WithChiRouter(r chi.Router) func() signalr.MappableRouter {
	return func() signalr.MappableRouter {
		return &chiRouter{r: r}
	}
}

type chiRouter struct {
	r chi.Router
}

func (j *chiRouter) HandleFunc(path string, handler func(w http.ResponseWriter, r *http.Request)) {
	j.r.HandleFunc(path, handler)
}

func (j *chiRouter) Handle(pattern string, handler http.Handler) {
	j.r.Handle(pattern, handler)
}

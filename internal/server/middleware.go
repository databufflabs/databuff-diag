package server

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// errorLogger logs HTTP requests that return 5xx status codes.
func errorLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		if status < http.StatusInternalServerError {
			return
		}
		log.Printf(
			`[%s] "%s %s %s" from %s - %d %dB in %s`,
			middleware.GetReqID(r.Context()),
			r.Method,
			r.URL.String(),
			r.Proto,
			r.RemoteAddr,
			status,
			ww.BytesWritten(),
			time.Since(start),
		)
	})
}

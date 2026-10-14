package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/coniungo-repo/golang-server.git/internal/platform/logger"
	"github.com/coniungo-repo/golang-server.git/internal/platform/respond"
)

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &recorder{ResponseWriter: w}

		defer func() {
			if p := recover(); p != nil {
				if err, ok := p.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(p)
				}

				logger.From(r.Context()).Error("panic recovered",
					"error", fmt.Sprint(p),
					"stack", string(debug.Stack()),
				)

				if rec.status == 0 {
					respond.ServerError(w, r, fmt.Errorf("panic: %v", p))
				}
			}
		}()

		next.ServeHTTP(rec, r)
	})
}

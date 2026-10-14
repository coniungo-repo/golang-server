package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/coniungo-repo/golang-server/internal/platform/logger"
	"github.com/coniungo-repo/golang-server/internal/platform/respond"
)

// Recover turns a handler panic into a 500. If the response has already started, it aborts the connection so the client never sees a truncated success.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &recorder{ResponseWriter: w}

		defer func() {
			p := recover()
			if p == nil {
				return
			}
			if err, ok := p.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(p)
			}

			logger.From(r.Context()).Error("panic recovered",
				"panic", fmt.Sprint(p),
				"stack", string(debug.Stack()),
			)

			if rec.status != 0 {
				panic(http.ErrAbortHandler)
			}
			respond.Error(w, r, http.StatusInternalServerError, respond.ServerErrorMessage)
		}()

		next.ServeHTTP(rec, r)
	})
}

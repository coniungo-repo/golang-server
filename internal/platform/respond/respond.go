package respond

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/coniungo-repo/golang-server/internal/platform/logger"
	"github.com/coniungo-repo/golang-server/internal/platform/requestid"
)

const (
	ContentTypeJSON           = "application/json; charset=utf-8"
	ServerErrorMessage        = "Something went wrong on our side. Try again in a moment."
	TimeoutMessage            = "That took too long. Try again in a moment."
	StatusClientClosedRequest = 499
)

type errorBody struct {
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// JSON encodes v before writing headers, so an encoding failure becomes a clean 500.
func JSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if status == http.StatusNoContent || status == http.StatusNotModified {
		w.WriteHeader(status)
		return
	}

	buf, err := json.Marshal(v)
	if err != nil {
		ServerError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

func Error(w http.ResponseWriter, r *http.Request, status int, message string) {
	JSON(w, r, status, errorBody{
		Message:   message,
		RequestID: requestid.From(r.Context()),
	})
}

// ServerError logs err and sends a generic message. Deadlines become 503 and client disconnects are not treated as failures.
func ServerError(w http.ResponseWriter, r *http.Request, err error) {
	log := logger.From(r.Context())

	switch {
	case errors.Is(err, context.Canceled) && r.Context().Err() != nil:
		log.Info("client closed request", "err", err)
		w.WriteHeader(StatusClientClosedRequest)
	case errors.Is(err, context.DeadlineExceeded):
		log.Warn("request timed out", "err", err)
		w.Header().Set("Retry-After", "1")
		Error(w, r, http.StatusServiceUnavailable, TimeoutMessage)
	default:
		log.Error("server error", "err", err)
		Error(w, r, http.StatusInternalServerError, ServerErrorMessage)
	}
}

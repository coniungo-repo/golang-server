package respond

import (
	"encoding/json"
	"net/http"

	"github.com/coniungo-repo/golang-server.git/internal/platform/logger"
)

const (
	RequestIDHeader    = "X-Request-ID"
	ServerErrorMessage = "Something went wrong on our side. Try again in a moment."
	ContentTypeJSON    = "application/json; charset=utf-8"
)

type errorBody struct {
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// JSON writes a Go value to the response stream as JSON.
func JSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}

	// Marshal the JSON first to a byte slice.
	// This prevents "Partial Writes" where we send a 200 OK but then
	// the JSON encoding fails, resulting in a broken response.
	buf, err := json.Marshal(v)
	if err != nil {
		ServerError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

// Error sends a JSON-formatted error message to the client.
func Error(w http.ResponseWriter, r *http.Request, status int, message string) {
	requestID := r.Header.Get(RequestIDHeader)

	JSON(w, r, status, errorBody{
		Message:   message,
		RequestID: requestID,
	})
}

// ServerError handles internal failures (500) by logging the real error
// and sending a safe, generic message to the client.
func ServerError(w http.ResponseWriter, r *http.Request, err error) {
	logger.From(r.Context()).Error("server error", "err", err)

	Error(w, r, http.StatusInternalServerError, ServerErrorMessage)
}

// Status sends a response with just a status code and no body.
func Status(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
}

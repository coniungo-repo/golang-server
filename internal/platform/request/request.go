package request

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// MaxBodyBytes is the largest request body the API accepts.
const MaxBodyBytes = 1 << 20

// Error is a client mistake in the request body, with the status to answer with.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

func badRequest(format string, args ...any) *Error {
	return &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf(format, args...)}
}

// Decode reads a single JSON value into dst, rejecting unknown fields, trailing data, wrong media types and oversized bodies. Failures are *Error.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mt, _, err := mime.ParseMediaType(ct)
		if err != nil || mt != "application/json" {
			return &Error{Status: http.StatusUnsupportedMediaType, Message: "body must be application/json"}
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return badRequest("body must only contain a single JSON value")
	}
	return nil
}

func decodeError(err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var maxErr *http.MaxBytesError
	var invalidErr *json.InvalidUnmarshalError

	switch {
	case errors.As(err, &invalidErr):
		panic(err)
	case errors.As(err, &syntaxErr):
		return badRequest("body contains badly-formed JSON (at character %d)", syntaxErr.Offset)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return badRequest("body contains badly-formed JSON")
	case errors.As(err, &typeErr):
		if typeErr.Field != "" {
			return badRequest("body contains the wrong type for field %q", typeErr.Field)
		}
		return badRequest("body contains the wrong JSON type (at character %d)", typeErr.Offset)
	case errors.Is(err, io.EOF):
		return badRequest("body must not be empty")
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return badRequest("body contains unknown key %s", strings.TrimPrefix(err.Error(), "json: unknown field "))
	case errors.As(err, &maxErr):
		return &Error{Status: http.StatusRequestEntityTooLarge, Message: fmt.Sprintf("body must not be larger than %d bytes", MaxBodyBytes)}
	default:
		return err
	}
}

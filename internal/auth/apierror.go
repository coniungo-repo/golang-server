package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/coniungo-repo/golang-server/internal/platform/request"
	"github.com/coniungo-repo/golang-server/internal/platform/respond"
	"github.com/supertokens/supertokens-golang/supertokens"
)

// onAPIError answers failed SuperTokens routes: client mistakes get a 4xx with a safe message, anything else is logged and becomes a 500.
func (s *Service) onAPIError(err error, r *http.Request, w http.ResponseWriter) {
	if reqErr := clientError(r, err); reqErr != nil {
		s.deps.Log.InfoContext(r.Context(), "auth request rejected", "status", reqErr.Status, "err", err)
		respond.Error(w, r, reqErr.Status, reqErr.Message)
		return
	}
	s.deps.Log.ErrorContext(r.Context(), "auth request failed", "err", err)
	respond.Error(w, r, http.StatusInternalServerError, respond.ServerErrorMessage)
}

// clientError returns the response for an error the caller caused, or nil. JSON errors only count when the request body is at fault, because the SDK also parses core responses.
func clientError(r *http.Request, err error) *request.Error {
	var badInput supertokens.BadInputError
	var maxErr *http.MaxBytesError
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	switch {
	case errors.As(err, &badInput):
		return &request.Error{Status: http.StatusBadRequest, Message: badInput.Msg}
	case errors.As(err, &maxErr):
		return asRequestError(err)
	case errors.As(err, &syntaxErr), errors.As(err, &typeErr):
		return bodyError(r, typeErr)
	default:
		return nil
	}
}

func bodyError(r *http.Request, typeErr *json.UnmarshalTypeError) *request.Error {
	if r.Body == nil {
		return nil
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return asRequestError(err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return &request.Error{Status: http.StatusBadRequest, Message: "body must not be empty"}
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		if errors.As(err, new(*json.UnmarshalTypeError)) {
			return &request.Error{Status: http.StatusBadRequest, Message: "body must be a JSON object"}
		}
		return asRequestError(err)
	}
	if typeErr != nil && typeErr.Field != "" {
		return asRequestError(typeErr)
	}
	return nil
}

func asRequestError(err error) *request.Error {
	var reqErr *request.Error
	if errors.As(request.DecodeError(err), &reqErr) {
		return reqErr
	}
	return nil
}

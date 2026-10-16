package auth

import (
	"bytes"
	"encoding/json"
	"mime"
	"net/http"
)

// errorWriter holds back JSON error responses so the request ID can be added to bodies the SDK writes itself.
type errorWriter struct {
	http.ResponseWriter
	status int
	held   bool
	buf    bytes.Buffer
}

func (e *errorWriter) WriteHeader(status int) {
	if e.status != 0 {
		return
	}
	e.status = status
	if status >= http.StatusBadRequest && isJSON(e.Header().Get("Content-Type")) {
		e.held = true
		return
	}
	e.ResponseWriter.WriteHeader(status)
}

func (e *errorWriter) Write(p []byte) (int, error) {
	if e.status == 0 {
		e.WriteHeader(http.StatusOK)
	}
	if e.held {
		return e.buf.Write(p)
	}
	return e.ResponseWriter.Write(p)
}

func (e *errorWriter) Flush() {
	if e.held {
		return
	}
	if f, ok := e.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (e *errorWriter) Unwrap() http.ResponseWriter { return e.ResponseWriter }

// finish sends a held error response, adding requestID when the body is a JSON object without one.
func (e *errorWriter) finish(requestID string) {
	if !e.held {
		return
	}
	body := e.buf.Bytes()
	var obj map[string]json.RawMessage
	if requestID != "" && json.Unmarshal(body, &obj) == nil && obj != nil {
		if _, ok := obj["request_id"]; !ok {
			obj["request_id"], _ = json.Marshal(requestID)
			if b, err := json.Marshal(obj); err == nil {
				body = b
			}
		}
	}
	e.Header().Del("Content-Length")
	e.ResponseWriter.WriteHeader(e.status)
	_, _ = e.ResponseWriter.Write(body)
}

func isJSON(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && mt == "application/json"
}

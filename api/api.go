// Package api holds the OpenAPI description of every route the server answers.
package api

import _ "embed"

// OpenAPI is openapi.yaml, served at /openapi.yaml when docs are enabled.
//
//go:embed openapi.yaml
var OpenAPI []byte

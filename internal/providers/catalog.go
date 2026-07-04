// Package providers ships the built-in LLM provider catalog embedded in the binary.
package providers

import _ "embed"

// CatalogYAML is the embedded internal/providers/catalog.yaml.
//
//go:embed catalog.yaml
var CatalogYAML []byte

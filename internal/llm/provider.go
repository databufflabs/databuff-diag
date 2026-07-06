package llm

import (
	"fmt"

	"github.com/databufflabs/databuff-diag/internal/store"
	"github.com/databufflabs/databuff-diag/internal/providers"
	"gopkg.in/yaml.v3"
)

// CatalogEntry is a built-in provider preset from internal/providers/catalog.yaml.
type CatalogEntry struct {
	ProviderCode   string `yaml:"provider_code"`
	DisplayName    string `yaml:"display_name"`
	DefaultBaseURL string `yaml:"default_base_url"`
	DefaultWireAPI string `yaml:"default_wire_api"`
	DefaultModel   string `yaml:"default_model,omitempty"`
}

type catalogFile struct {
	Providers []CatalogEntry `yaml:"providers"`
}

// LoadCatalog parses the embedded provider catalog.
func LoadCatalog() ([]CatalogEntry, error) {
	var file catalogFile
	if err := yaml.Unmarshal(providers.CatalogYAML, &file); err != nil {
		return nil, fmt.Errorf("parse provider catalog: %w", err)
	}
	return file.Providers, nil
}

// MergedProvider combines catalog defaults with user overrides.
type MergedProvider struct {
	ProviderCode      string
	DisplayName       string
	Enabled           bool
	WireAPI           string
	BaseURL           string
	APIKey            string
	Model             string
	TimeoutSec        int
	ResponseProcessor string
	SupportsVision    *bool
	ToolsEnabled      *bool
	FromCatalog       bool
}

// MergeProviders overlays user config on top of catalog presets.
func MergeProviders(catalog []CatalogEntry, cfg *store.Config) []MergedProvider {
	byCode := make(map[string]CatalogEntry, len(catalog))
	for _, entry := range catalog {
		byCode[entry.ProviderCode] = entry
	}

	seen := make(map[string]struct{})
	var merged []MergedProvider

	for _, entry := range catalog {
		merged = append(merged, mergeOne(entry, cfg))
		seen[entry.ProviderCode] = struct{}{}
	}

	if cfg != nil {
		for code, inst := range cfg.LLM.Providers {
			if _, ok := seen[code]; ok {
				continue
			}
			merged = append(merged, MergedProvider{
				ProviderCode:      code,
				DisplayName:       code,
				Enabled:           inst.Enabled,
				WireAPI:           inst.WireAPI,
				BaseURL:           inst.BaseURL,
				APIKey:            inst.APIKey,
				Model:             inst.Model,
				TimeoutSec:        inst.TimeoutSec,
				ResponseProcessor: inst.ResponseProcessor,
				SupportsVision:    inst.SupportsVision,
				ToolsEnabled:      inst.ToolsEnabled,
				FromCatalog:       false,
			})
		}
	}

	return merged
}

func mergeOne(entry CatalogEntry, cfg *store.Config) MergedProvider {
	out := MergedProvider{
		ProviderCode: entry.ProviderCode,
		DisplayName:  entry.DisplayName,
		WireAPI:      entry.DefaultWireAPI,
		BaseURL:      entry.DefaultBaseURL,
		Model:        entry.DefaultModel,
		FromCatalog:  true,
	}
	if cfg == nil {
		return out
	}

	inst, ok := cfg.LLM.Providers[entry.ProviderCode]
	if !ok {
		return out
	}

	out.Enabled = inst.Enabled
	if inst.WireAPI != "" {
		out.WireAPI = inst.WireAPI
	}
	if inst.BaseURL != "" {
		out.BaseURL = inst.BaseURL
	}
	if inst.Model != "" {
		out.Model = inst.Model
	}
	out.APIKey = inst.APIKey
	out.TimeoutSec = inst.TimeoutSec
	out.ResponseProcessor = inst.ResponseProcessor
	out.SupportsVision = inst.SupportsVision
	out.ToolsEnabled = inst.ToolsEnabled
	return out
}

// ActiveProvider returns the merged provider for cfg.LLM.Active, if configured.
func ActiveProvider(catalog []CatalogEntry, cfg *store.Config) (*MergedProvider, error) {
	if cfg == nil || cfg.LLM.Active == "" {
		return nil, fmt.Errorf("no active llm provider configured")
	}
	for _, p := range MergeProviders(catalog, cfg) {
		if p.ProviderCode == cfg.LLM.Active {
			copy := p
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("active provider %q not found", cfg.LLM.Active)
}

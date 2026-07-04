package llm

import (
	"testing"

	"github.com/databufflabs/databuff-diag/internal/store"
)

func TestLoadCatalogAtLeastTenProviders(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	if len(catalog) < 10 {
		t.Fatalf("catalog len = %d, want >= 10", len(catalog))
	}

	required := []string{
		"openai", "anthropic", "deepseek", "moonshot", "zhipu",
		"bailian", "qianfan", "minimax", "ollama", "openrouter",
		"groq", "together", "custom",
	}
	byCode := make(map[string]CatalogEntry, len(catalog))
	for _, entry := range catalog {
		byCode[entry.ProviderCode] = entry
	}
	for _, code := range required {
		entry, ok := byCode[code]
		if !ok {
			t.Fatalf("missing provider %q", code)
		}
		if entry.DisplayName == "" {
			t.Fatalf("provider %q missing display_name", code)
		}
		if entry.DefaultWireAPI == "" {
			t.Fatalf("provider %q missing default_wire_api", code)
		}
	}
}

func TestMergeProvidersUsesCatalogDefaults(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}

	cfg := store.DefaultConfig()
	cfg.LLM.Providers["deepseek"] = store.ProviderInstance{
		Enabled: true,
		APIKey:  "sk-test",
	}

	merged := MergeProviders(catalog, cfg)
	var deepseek *MergedProvider
	for i := range merged {
		if merged[i].ProviderCode == "deepseek" {
			copy := merged[i]
			deepseek = &copy
			break
		}
	}
	if deepseek == nil {
		t.Fatal("deepseek not found in merged providers")
	}
	if deepseek.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("base_url = %q", deepseek.BaseURL)
	}
	if deepseek.APIKey != "sk-test" {
		t.Fatalf("api_key = %q, want sk-test", deepseek.APIKey)
	}
	if !deepseek.Enabled {
		t.Fatal("expected deepseek enabled")
	}
}

func TestMergeProvidersIncludesCustomUserProvider(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}

	cfg := store.DefaultConfig()
	cfg.LLM.Providers["custom-gateway"] = store.ProviderInstance{
		Enabled: true,
		BaseURL: "http://10.0.0.8:8000/v1",
		Model:   "qwen2.5-72b-instruct",
		APIKey:  "not-needed",
	}

	merged := MergeProviders(catalog, cfg)
	var gateway *MergedProvider
	for i := range merged {
		if merged[i].ProviderCode == "custom-gateway" {
			copy := merged[i]
			gateway = &copy
			break
		}
	}
	if gateway == nil {
		t.Fatal("custom-gateway not found")
	}
	if gateway.BaseURL != "http://10.0.0.8:8000/v1" {
		t.Fatalf("base_url = %q", gateway.BaseURL)
	}
}

func TestActiveProvider(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}

	cfg := store.DefaultConfig()
	cfg.LLM.Active = "deepseek"
	cfg.LLM.Providers["deepseek"] = store.ProviderInstance{
		Enabled: true,
		Model:   "deepseek-chat",
	}

	active, err := ActiveProvider(catalog, cfg)
	if err != nil {
		t.Fatalf("ActiveProvider: %v", err)
	}
	if active.ProviderCode != "deepseek" {
		t.Fatalf("provider = %q, want deepseek", active.ProviderCode)
	}
}

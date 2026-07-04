package processor

import (
	"fmt"
	"sync"
)

// Processor extracts assistant text from an HTTP response body.
type Processor interface {
	// ID is referenced by llm.providers[].response_processor in config.
	ID() string
	// Description is shown in UI processor dropdowns.
	Description() string
	Extract(body []byte) (content string, err error)
}

// Meta describes a registered processor for listing.
type Meta struct {
	ID          string
	Description string
}

var (
	mu         sync.RWMutex
	byID       = make(map[string]Processor)
	registered []Processor
)

func init() {
	Register(&OpenAICompat{})
	Register(&AnthropicMessages{})
	Register(&DatabuffUltraResult{})
}

// Register adds a processor to the global registry.
func Register(p Processor) {
	mu.Lock()
	defer mu.Unlock()
	byID[p.ID()] = p
	registered = append(registered, p)
}

// Get returns a processor by ID.
func Get(id string) (Processor, error) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := byID[id]
	if !ok {
		return nil, fmt.Errorf("unknown response processor %q", id)
	}
	return p, nil
}

// List returns metadata for all registered processors.
func List() []Meta {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Meta, len(registered))
	for i, p := range registered {
		out[i] = Meta{ID: p.ID(), Description: p.Description()}
	}
	return out
}

// Resolve picks the processor for a provider: explicit response_processor,
// else anthropic_messages when wire_api is anthropic, else openai_compat.
func Resolve(responseProcessor, wireAPI string) (Processor, error) {
	id := responseProcessor
	if id == "" {
		if wireAPI == "anthropic" {
			id = "anthropic_messages"
		} else {
			id = "openai_compat"
		}
	}
	return Get(id)
}

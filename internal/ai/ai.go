// Package ai is the smallest client of an OpenAI-compatible chat API
// (docs/prd/dev-cycle.md, AI): one request, one answer. AI is off when no
// address is set. What it writes is a comment, never a decision.
package ai

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Client is an address, its key and the model asked.
type Client struct {
	Base, Key, Model string
}

// FromEnv is LLM_BASE_URL, LLM_API_KEY and LLM_MODEL; the address or the
// model missing, opencode's own (its opencode.json), so a machine set up
// for opencode needs only the key. nil when there is no address: AI off.
// ponytail: the [ai] model of the project file is dev-cycle.md's, later.
func FromEnv() *Client {
	c := &Client{Base: os.Getenv("LLM_BASE_URL"), Key: os.Getenv("LLM_API_KEY"), Model: os.Getenv("LLM_MODEL")}
	if c.Base == "" || c.Model == "" {
		base, model := opencode()
		if c.Base == "" {
			c.Base, c.Model = base, cmp.Or(c.Model, model)
		} else if c.Base == base {
			c.Model = model
		}
	}
	if c.Base == "" || c.Model == "" {
		return nil
	}
	c.Base = strings.TrimRight(c.Base, "/")
	return c
}

// opencode is the address and model of opencode's config: its model
// ("provider/model") when set, else the first provider by name with an
// address, and its first model by name. The key stays in opencode's store.
func opencode() (base, model string) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", ""
		}
		dir = filepath.Join(home, ".config")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "opencode", "opencode.json"))
	if err != nil {
		return "", ""
	}
	var cfg struct {
		Model    string `json:"model"`
		Provider map[string]struct {
			Options struct {
				BaseURL string `json:"baseURL"`
			} `json:"options"`
			Models map[string]json.RawMessage `json:"models"`
		} `json:"provider"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return "", ""
	}
	if name, m, ok := strings.Cut(cfg.Model, "/"); ok {
		if p, ok := cfg.Provider[name]; ok && p.Options.BaseURL != "" {
			return p.Options.BaseURL, m
		}
	}
	// ponytail: by name, as JSON objects have no order in Go; LLM_MODEL picks another
	for _, n := range slices.Sorted(maps.Keys(cfg.Provider)) {
		if p := cfg.Provider[n]; p.Options.BaseURL != "" && len(p.Models) > 0 {
			return p.Options.BaseURL, slices.Sorted(maps.Keys(p.Models))[0]
		}
	}
	return "", ""
}

// Chat sends a system and a user message and returns the answer.
func (c *Client) Chat(ctx context.Context, system, user string) (string, error) {
	in, err := json.Marshal(map[string]any{
		"model":       c.Model,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/chat/completions", bytes.NewReader(in))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode/100 != 2 {
		return "", fmt.Errorf("%s: %s: %s", c.Model, res.Status, strings.TrimSpace(string(raw[:min(len(raw), 300)])))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("%s: %w", c.Model, err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("%s: no answer", c.Model)
	}
	return out.Choices[0].Message.Content, nil
}

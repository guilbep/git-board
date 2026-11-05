package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChat(t *testing.T) {
	t.Parallel()
	var got struct {
		Model    string              `json:"model"`
		Messages []map[string]string `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, "wrong "+r.URL.Path, http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"normal\nOne area."}}]}`))
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL + "/v1", Key: "k", Model: "m"}
	out, err := c.Chat(context.Background(), "sys", "user")
	if err != nil || out != "normal\nOne area." {
		t.Fatalf("Chat = %q, %v", out, err)
	}
	if got.Model != "m" || len(got.Messages) != 2 || got.Messages[0]["content"] != "sys" || got.Messages[1]["role"] != "user" {
		t.Errorf("request = %+v", got)
	}
	c.Key = "wrong"
	if _, err := c.Chat(context.Background(), "s", "u"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("a refused request: %v, want the status", err)
	}
}

// FromEnv reads the env first, then opencode's config for what is missing.
func TestFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	for _, k := range []string{"LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL"} {
		t.Setenv(k, "")
	}
	if c := FromEnv(); c != nil {
		t.Fatalf("no address: %+v, want nil", c)
	}
	_ = os.MkdirAll(filepath.Join(dir, "opencode"), 0o755)
	cfg := `{"provider":{"z":{"options":{"baseURL":"https://z/v1/"},"models":{"b":{},"a":{}}},"local":{"models":{"x":{}}}}}`
	_ = os.WriteFile(filepath.Join(dir, "opencode", "opencode.json"), []byte(cfg), 0o644)
	t.Setenv("LLM_API_KEY", "k")
	if c := FromEnv(); c == nil || c.Base != "https://z/v1" || c.Model != "a" || c.Key != "k" {
		t.Errorf("from opencode: %+v", c)
	}
	t.Setenv("LLM_MODEL", "picked")
	if c := FromEnv(); c == nil || c.Model != "picked" {
		t.Errorf("LLM_MODEL: %+v", c)
	}
	t.Setenv("LLM_BASE_URL", "https://other")
	if c := FromEnv(); c == nil || c.Base != "https://other" || c.Model != "picked" {
		t.Errorf("the env: %+v", c)
	}
	t.Setenv("LLM_MODEL", "")
	if c := FromEnv(); c != nil {
		t.Errorf("another address without a model: %+v, want nil", c)
	}
}

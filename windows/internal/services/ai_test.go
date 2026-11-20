package services

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIConfigAndChatCompletion(t *testing.T) {
	root := t.TempDir()
	if _, err := ValidateAIBaseURL("http://api.example.com/v1"); err == nil {
		t.Fatal("expected insecure remote HTTP AI base URL to be rejected")
	}
	if _, err := ValidateAIBaseURL("http://localhost:11434/v1"); err != nil {
		t.Fatalf("expected localhost HTTP AI base URL to be allowed: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"qwen2.5:7b"},{"id":"llama3.2"}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"这是一篇关于 IPFS 的技术摘要。"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg, err := SaveAIConfig(root, srv.URL+"/v1", "qwen2.5:7b", "local-token")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.HasKey || cfg.Model != "qwen2.5:7b" {
		t.Fatalf("unexpected saved AI config: %+v", cfg)
	}
	probe, err := ProbeAIEndpoints(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(probe.Models) != 2 || probe.Models[0] != "llama3.2" {
		t.Fatalf("unexpected probed models: %+v", probe)
	}
	reply, err := ChatCompletion(root, []AIChatMessage{{Role: "user", Content: "总结本文"}})
	if err != nil || reply != "这是一篇关于 IPFS 的技术摘要。" {
		t.Fatalf("unexpected chat completion reply: %q (%v)", reply, err)
	}
}

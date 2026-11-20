package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type AIConfig struct {
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
	HasKey  bool   `json:"hasKey"`
}

type AIProbeResult struct {
	OllamaDetected   bool     `json:"ollamaDetected"`
	LMStudioDetected bool     `json:"lmStudioDetected"`
	Models           []string `json:"models"`
}

type AIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type storedAIConfig struct {
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
	Token   string `json:"token,omitempty"`
}

func ValidateAIBaseURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return "", errors.New("AI 服务地址无效")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return "", errors.New("AI 服务地址必须使用 HTTP 或 HTTPS")
	}
	if scheme == "http" && !isPrivateOrLoopbackHost(u.Hostname()) {
		return "", errors.New("公网 AI 服务必须使用 HTTPS；仅本机或局域网允许 HTTP")
	}
	return strings.TrimRight(rawURL, "/"), nil
}

func isPrivateOrLoopbackHost(host string) bool {
	lower := strings.ToLower(strings.TrimSpace(host))
	if lower == "localhost" || strings.HasSuffix(lower, ".local") {
		return true
	}
	ip := net.ParseIP(lower)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

func SaveAIConfig(root, baseURL, model, token string) (AIConfig, error) {
	normalizedURL, err := ValidateAIBaseURL(baseURL)
	if err != nil {
		return AIConfig{}, err
	}
	model = strings.TrimSpace(model)
	token = strings.TrimSpace(token)
	old, _ := loadStoredAIConfig(root)
	if token == "" && normalizedURL == old.BaseURL {
		token = old.Token
	}
	if normalizedURL == "" {
		token = ""
	}
	dir := filepath.Join(root, "integrations", "ai")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return AIConfig{}, err
	}
	path := filepath.Join(dir, "config.dat")
	if normalizedURL == "" && model == "" && token == "" {
		_ = os.Remove(path)
		return AIConfig{}, nil
	}
	raw, err := json.Marshal(storedAIConfig{BaseURL: normalizedURL, Model: model, Token: token})
	if err != nil {
		return AIConfig{}, err
	}
	sealed, err := protect(raw)
	if err != nil {
		return AIConfig{}, errors.New("无法加密保存 AI 配置")
	}
	if err := os.WriteFile(path, sealed, 0600); err != nil {
		return AIConfig{}, err
	}
	return AIConfig{BaseURL: normalizedURL, Model: model, HasKey: token != ""}, nil
}

func LoadAIConfig(root string) (AIConfig, error) {
	cfg, err := loadStoredAIConfig(root)
	if err != nil {
		return AIConfig{}, err
	}
	return AIConfig{BaseURL: cfg.BaseURL, Model: cfg.Model, HasKey: cfg.Token != ""}, nil
}

func loadStoredAIConfig(root string) (storedAIConfig, error) {
	path := filepath.Join(root, "integrations", "ai", "config.dat")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return storedAIConfig{}, nil
	}
	if err != nil {
		return storedAIConfig{}, err
	}
	raw, err := unprotect(data)
	if err != nil {
		return storedAIConfig{}, errors.New("无法读取 AI 配置")
	}
	var cfg storedAIConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return storedAIConfig{}, errors.New("AI 配置格式无效")
	}
	return cfg, nil
}

func ProbeAIEndpoints(root, baseURL string) (AIProbeResult, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	var out AIProbeResult
	if resp, err := client.Get("http://127.0.0.1:11434/api/tags"); err == nil {
		_ = resp.Body.Close()
		out.OllamaDetected = resp.StatusCode == http.StatusOK
	}
	if resp, err := client.Get("http://127.0.0.1:1234/v1/models"); err == nil {
		_ = resp.Body.Close()
		out.LMStudioDetected = resp.StatusCode == http.StatusOK
	}
	stored, _ := loadStoredAIConfig(root)
	targetBase := strings.TrimSpace(baseURL)
	if targetBase == "" {
		targetBase = stored.BaseURL
	}
	if targetBase == "" {
		return out, nil
	}
	normalized, err := ValidateAIBaseURL(targetBase)
	if err != nil {
		return out, err
	}
	modelsURL := normalized
	if !strings.HasSuffix(modelsURL, "/models") {
		modelsURL += "/models"
	}
	req, err := http.NewRequest(http.MethodGet, modelsURL, nil)
	if err != nil {
		return out, err
	}
	if stored.Token != "" && normalized == stored.BaseURL {
		req.Header.Set("Authorization", "Bearer "+stored.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return out, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return out, nil
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		for _, item := range parsed.Data {
			if id := strings.TrimSpace(item.ID); id != "" {
				out.Models = append(out.Models, id)
			}
		}
		sort.Strings(out.Models)
	}
	return out, nil
}

func ChatCompletion(root string, messages []AIChatMessage) (string, error) {
	cfg, err := loadStoredAIConfig(root)
	if err != nil {
		return "", err
	}
	if cfg.BaseURL == "" {
		return "", errors.New("请先在 AI 设置中配置 API 地址（如 Ollama http://localhost:11434/v1 或 LM Studio http://localhost:1234/v1）")
	}
	normalized, err := ValidateAIBaseURL(cfg.BaseURL)
	if err != nil {
		return "", err
	}
	model := cfg.Model
	if model == "" {
		model = "llama3"
	}
	endpoint := normalized
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	payload, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   false,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("无法连接 AI 服务，请检查本地服务是否已启动")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", errors.New("读取 AI 响应失败")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("AI 服务返回错误状态")
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Choices) == 0 {
		return "", errors.New("AI 响应格式无效")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

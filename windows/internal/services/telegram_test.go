package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

const testTelegramToken = "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi"

type telegramTransport func(*http.Request) (*http.Response, error)

func (f telegramTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func telegramResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func telegramTestClient(f telegramTransport) *TelegramClient {
	c := NewTelegramClient()
	c.http.Transport = f
	return c
}

func TestTelegramClientResolvesCanonicalGroupAndSendsPlainText(t *testing.T) {
	calls := 0
	c := telegramTestClient(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "api.telegram.org" || r.Method != http.MethodPost {
			t.Fatal("request did not use the fixed HTTPS API")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		switch calls {
		case 1:
			if r.URL.Path != "/bot"+testTelegramToken+"/getChat" || body["chat_id"] != "@planet_news" {
				t.Fatalf("unexpected lookup: %v", body)
			}
			return telegramResponse(200, `{"ok":true,"result":{"id":-1001234567890,"title":"Planet News","type":"channel"}}`), nil
		case 2:
			if r.URL.Path != "/bot"+testTelegramToken+"/sendMessage" || body["chat_id"] != "-1001234567890" || body["text"] != "Title\n\nSummary <b>plain</b>\nhttps://example.com/post" || body["parse_mode"] != nil {
				t.Fatalf("unexpected message: %v", body)
			}
			return telegramResponse(200, `{"ok":true,"result":{"message_id":42,"chat":{"id":-1001234567890,"type":"channel"},"date":1,"text":"Title"}}`), nil
		default:
			t.Fatal("extra request")
			return nil, errors.New("extra request")
		}
	})
	chat, err := c.ResolveChat(testTelegramToken, "@planet_news")
	if err != nil || chat.ID != "-1001234567890" || chat.Title != "Planet News" || chat.Type != "channel" {
		t.Fatalf("lookup: %+v %v", chat, err)
	}
	id, err := c.Send(testTelegramToken, chat.ID, "Title\n\nSummary <b>plain</b>\nhttps://example.com/post")
	if err != nil || id != 42 {
		t.Fatalf("send: %d %v", id, err)
	}
}

func TestTelegramClientRejectsUnsafeInputsAndPrivateChats(t *testing.T) {
	calls := 0
	c := telegramTestClient(func(r *http.Request) (*http.Response, error) {
		calls++
		return telegramResponse(200, `{"ok":true,"result":{"id":123,"first_name":"Private","type":"private"}}`), nil
	})
	for _, token := range []string{"", "bad/token", testTelegramToken + "?x", testTelegramToken + "\n"} {
		if _, err := c.ResolveChat(token, "@planet_news"); err == nil {
			t.Errorf("accepted token %q", token)
		}
	}
	for _, target := range []string{"", "123", "-0", "-01", "@bad/name", "https://t.me/planet_news", "-9223372036854775809"} {
		if _, err := c.ResolveChat(testTelegramToken, target); err == nil {
			t.Errorf("accepted target %q", target)
		}
	}
	if _, err := c.Send(testTelegramToken, "-123", strings.Repeat("😀", 2049)); err == nil {
		t.Fatal("accepted text over UTF-16 limit")
	}
	if calls != 0 {
		t.Fatalf("invalid input made %d requests", calls)
	}
	if _, err := c.ResolveChat(testTelegramToken, "@planet_news"); err == nil {
		t.Fatal("accepted a private chat")
	}
}

func TestTelegramClientSanitizesUncertainFailuresAndRejectsRedirects(t *testing.T) {
	for _, scenario := range []string{"transport", "redirect", "oversized", "invalid", "no-message"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			c := telegramTestClient(func(r *http.Request) (*http.Response, error) {
				calls++
				switch scenario {
				case "transport":
					return nil, fmt.Errorf("request %s failed", r.URL)
				case "redirect":
					response := telegramResponse(307, "")
					response.Header.Set("Location", "https://evil.example/"+testTelegramToken)
					return response, nil
				case "oversized":
					return telegramResponse(200, strings.Repeat("x", (1<<20)+1)), nil
				case "invalid":
					return telegramResponse(200, testTelegramToken), nil
				default:
					return telegramResponse(200, `{"ok":true,"result":{}}`), nil
				}
			})
			_, err := c.Send(testTelegramToken, "-123", "hello")
			var rejected *TelegramAPIError
			if err == nil || strings.Contains(err.Error(), testTelegramToken) || errors.As(err, &rejected) {
				t.Fatalf("unsafe or definite failure: %v", err)
			}
			if calls != 1 {
				t.Fatalf("redirect/retry produced %d requests", calls)
			}
		})
	}
}

func TestTelegramEditRecognizesNotModifiedAndAPIRejection(t *testing.T) {
	c := telegramTestClient(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(r.URL.Path, "/editMessageText") {
			if body["message_id"] != float64(42) {
				t.Fatalf("bad edit: %v", body)
			}
			return telegramResponse(400, `{"ok":false,"error_code":400,"description":"Bad Request: message is not modified: same text"}`), nil
		}
		return telegramResponse(403, `{"ok":false,"error_code":403,"description":"Forbidden: bot was kicked from the channel `+testTelegramToken+`"}`), nil
	})
	if err := c.Edit(testTelegramToken, "-123", 42, "hello"); err != nil {
		t.Fatal(err)
	}
	_, err := c.Send(testTelegramToken, "-123", "hello")
	var rejected *TelegramAPIError
	if !errors.As(err, &rejected) || rejected.Code != 403 || strings.Contains(err.Error(), testTelegramToken) {
		t.Fatalf("rejection: %v", err)
	}
}

func telegramConfigFixture() TelegramConfig {
	return TelegramConfig{Token: testTelegramToken, ChatID: "-1001234567890", ChatTitle: "Planet News", Messages: map[string]TelegramMessage{"old-post": {MessageID: 20, Hash: strings.Repeat("a", 64)}}}
}

func TestTelegramConfigurationEncryptsHistoryAndRemovesIt(t *testing.T) {
	root := t.TempDir()
	cfg := telegramConfigFixture()
	if err := SaveTelegramConfig(root, "site-1", cfg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "integrations", "telegram", "site-1.dat")
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), testTelegramToken) || strings.Contains(string(data), "old-post") {
		t.Fatalf("plaintext config or read error: %v", err)
	}
	loaded, err := LoadTelegramConfig(root, "site-1")
	if err != nil || !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("round trip: %+v %v", loaded, err)
	}
	cfg.ChatTitle = "Updated title"
	if err := SaveTelegramConfig(root, "site-1", cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadTelegramConfig(root, "site-1")
	if err != nil || loaded.Messages["old-post"].MessageID != 20 || loaded.ChatTitle != "Updated title" {
		t.Fatalf("replace: %+v %v", loaded, err)
	}
	if err := DeleteTelegramConfig(root, "site-1"); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadTelegramConfig(root, "site-1")
	if err != nil || loaded.Token != "" {
		t.Fatalf("delete: %+v %v", loaded, err)
	}
	if err := DeleteTelegramConfig(root, "site-1"); err != nil {
		t.Fatal(err)
	}
}

func TestGenericConfigurationCannotReadOrOverwriteTelegramState(t *testing.T) {
	root := t.TempDir()
	cfg := telegramConfigFixture()
	if err := SaveTelegramConfig(root, "site", cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(root, "site", "telegram"); err == nil {
		t.Fatal("generic configuration read Telegram state")
	}
	if err := SaveConfig(root, "site", "telegram", Config{Name: "other", Token: "generic-token"}); err == nil {
		t.Fatal("generic configuration overwrote Telegram state")
	}
	if err := SaveConfig(root, "site", "telegram", Config{}); err == nil {
		t.Fatal("generic configuration deleted Telegram state")
	}
	loaded, err := LoadTelegramConfig(root, "site")
	if err != nil || !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("generic access changed Telegram state: %v", err)
	}
	if _, err := LoadConfig(t.TempDir(), "site", "telegram"); err == nil {
		t.Fatal("generic configuration accepted Telegram kind without a saved file")
	}
}

func TestTelegramConfigurationRejectsInvalidOrOversizedFilesWithoutReplacing(t *testing.T) {
	root := t.TempDir()
	cfg := telegramConfigFixture()
	if err := SaveTelegramConfig(root, "site", cfg); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []TelegramConfig{{Token: "bad", ChatID: "-123"}, {Token: testTelegramToken, ChatID: "@planet_news"}, {Token: testTelegramToken, ChatID: "123"}} {
		if err := SaveTelegramConfig(root, "site", bad); err == nil {
			t.Fatalf("accepted config: %+v", bad)
		}
	}
	large := telegramConfigFixture()
	large.ChatTitle = strings.Repeat("x", 8<<20)
	if err := SaveTelegramConfig(root, "site", large); err == nil {
		t.Fatal("accepted oversized write")
	}
	loaded, err := LoadTelegramConfig(root, "site")
	if err != nil || !reflect.DeepEqual(loaded, cfg) {
		t.Fatal("failed write changed config")
	}
	if err := SaveTelegramConfig(root, "../site", cfg); err == nil {
		t.Fatal("accepted unsafe site ID")
	}
	path := filepath.Join(root, "integrations", "telegram", "site.dat")
	if err := os.WriteFile(path, make([]byte, (8<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTelegramConfig(root, "site"); err == nil {
		t.Fatal("accepted oversized read")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := SaveTelegramConfig(root, "site", cfg); err == nil {
		t.Fatal("replaced directory")
	}
	if err := DeleteTelegramConfig(root, "site"); err == nil {
		t.Fatal("removed directory")
	}
}

func TestTelegramConfigurationRejectsOversizedHistory(t *testing.T) {
	root := t.TempDir()
	cfg := telegramConfigFixture()
	if err := SaveTelegramConfig(root, "site", cfg); err != nil {
		t.Fatal(err)
	}
	large := telegramConfigFixture()
	for i := 0; i < 50000; i++ {
		large.Messages[fmt.Sprintf("%090d", i)] = TelegramMessage{MessageID: int64(i + 1), Hash: strings.Repeat("a", 64)}
	}
	if err := SaveTelegramConfig(root, "site", large); err == nil {
		t.Fatal("accepted history whose serialized size exceeds the read limit")
	}
	loaded, err := LoadTelegramConfig(root, "site")
	if err != nil || !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("oversized history changed saved configuration: %v", err)
	}
}

func TestTelegramConfigurationRejectsSymlinkPaths(t *testing.T) {
	for _, location := range []string{"ancestor", "integrations", "kind", "file"} {
		t.Run(location, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "root")
			target := filepath.Join(base, "target")
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			link := root
			switch location {
			case "ancestor":
				link = filepath.Join(base, "link")
				root = filepath.Join(link, "child")
				if err := os.MkdirAll(filepath.Join(target, "child"), 0700); err != nil {
					t.Fatal(err)
				}
			case "integrations":
				link = filepath.Join(root, "integrations")
			case "kind":
				link = filepath.Join(root, "integrations", "telegram")
			case "file":
				link = filepath.Join(root, "integrations", "telegram", "site.dat")
			}
			if location == "file" {
				if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.MkdirAll(target, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(link), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, link); err != nil {
				if errors.Is(err, syscall.Errno(1314)) {
					t.Skip("Windows symlinks require developer mode or privilege")
				}
				t.Fatal(err)
			}
			if err := SaveTelegramConfig(root, "site", telegramConfigFixture()); err == nil {
				t.Fatal("saved through symlink")
			}
			if _, err := LoadTelegramConfig(root, "site"); err == nil {
				t.Fatal("loaded through symlink")
			}
			if err := DeleteTelegramConfig(root, "site"); err == nil {
				t.Fatal("deleted through symlink")
			}
			if location == "file" {
				data, err := os.ReadFile(target)
				if err != nil || string(data) != "untouched" {
					t.Fatalf("changed symlink target: %v", err)
				}
			}
		})
	}
}

type fakeTelegramAPI struct {
	root             string
	sends, edits     int
	sendErr, editErr error
	t                *testing.T
}

func (f *fakeTelegramAPI) ResolveChat(string, string) (TelegramChat, error) {
	return TelegramChat{}, nil
}
func (f *fakeTelegramAPI) Send(token, chatID, text string) (int64, error) {
	f.sends++
	cfg, err := LoadTelegramConfig(f.root, "site")
	if err != nil {
		f.t.Fatal(err)
	}
	if !cfg.Messages["post"].Pending && !strings.HasPrefix(text, "batch ") {
		f.t.Fatal("send started before persisting pending state")
	}
	if token != testTelegramToken || chatID != "-1001234567890" {
		f.t.Fatal("incorrect send target")
	}
	return int64(100 + f.sends), f.sendErr
}
func (f *fakeTelegramAPI) Edit(token, chatID string, messageID int64, text string) error {
	f.edits++
	if messageID != 101 || token != testTelegramToken || chatID != "-1001234567890" {
		f.t.Fatal("incorrect edit target")
	}
	return f.editErr
}

func setupTelegramSync(t *testing.T) (string, *fakeTelegramAPI) {
	t.Helper()
	root := t.TempDir()
	if err := SaveTelegramConfig(root, "site", telegramConfigFixture()); err != nil {
		t.Fatal(err)
	}
	return root, &fakeTelegramAPI{root: root, t: t}
}

func TestTelegramSyncSendsOnceThenEditsAndPersistsEachVersion(t *testing.T) {
	root, api := setupTelegramSync(t)
	posts := []TelegramPost{{ID: "post", Text: "hello"}}
	if n, err := SyncTelegram(root, "site", posts, false, api); err != nil || n != 1 {
		t.Fatalf("first: %d %v", n, err)
	}
	cfg, err := LoadTelegramConfig(root, "site")
	if err != nil || cfg.Messages["post"].MessageID != 101 || cfg.Messages["post"].Pending || cfg.Messages["post"].Hash != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("persisted result: %+v %v", cfg.Messages["post"], err)
	}
	if n, err := SyncTelegram(root, "site", posts, false, api); err != nil || n != 0 {
		t.Fatalf("unchanged: %d %v", n, err)
	}
	posts[0].Text = "updated"
	api.editErr = errors.New("lost response " + testTelegramToken)
	if _, err := SyncTelegram(root, "site", posts, false, api); err == nil || strings.Contains(err.Error(), testTelegramToken) {
		t.Fatalf("edit error: %v", err)
	}
	api.editErr = nil
	if n, err := SyncTelegram(root, "site", posts, false, api); err != nil || n != 1 {
		t.Fatalf("edit retry: %d %v", n, err)
	}
	if api.sends != 1 || api.edits != 2 {
		t.Fatalf("sent %d edited %d", api.sends, api.edits)
	}
}

func TestTelegramSyncBlocksUncertainSendUntilExplicitRetry(t *testing.T) {
	root, api := setupTelegramSync(t)
	api.sendErr = errors.New("lost response " + testTelegramToken)
	posts := []TelegramPost{{ID: "post", Text: "hello"}}
	if _, err := SyncTelegram(root, "site", posts, false, api); err == nil || strings.Contains(err.Error(), testTelegramToken) {
		t.Fatalf("send error: %v", err)
	}
	api.sendErr = nil
	posts[0].Text = "changed after uncertain send"
	if _, err := SyncTelegram(root, "site", posts, false, api); err == nil {
		t.Fatal("automatically retried uncertain delivery")
	}
	if api.sends != 1 {
		t.Fatal("automatic retry duplicated send")
	}
	if n, err := SyncTelegram(root, "site", posts, true, api); err != nil || n != 1 {
		t.Fatalf("explicit retry: %d %v", n, err)
	}
}

func TestTelegramSyncClearsPendingAfterDefiniteRejection(t *testing.T) {
	root, api := setupTelegramSync(t)
	api.sendErr = &TelegramAPIError{Code: 403, Description: "Forbidden"}
	posts := []TelegramPost{{ID: "post", Text: "hello"}}
	if _, err := SyncTelegram(root, "site", posts, false, api); err == nil {
		t.Fatal("ignored API error")
	}
	cfg, err := LoadTelegramConfig(root, "site")
	if err != nil || cfg.Messages["post"].Pending {
		t.Fatalf("definite failure left uncertainty: %+v %v", cfg.Messages["post"], err)
	}
	api.sendErr = nil
	if n, err := SyncTelegram(root, "site", posts, false, api); err != nil || n != 1 {
		t.Fatalf("automatic retry: %d %v", n, err)
	}
}

func TestTelegramSyncOptInAndBatchLimit(t *testing.T) {
	if n, err := SyncTelegram(t.TempDir(), "site", []TelegramPost{{ID: "post", Text: "hello"}}, false, nil); err != nil || n != 0 {
		t.Fatalf("unconfigured: %d %v", n, err)
	}
	root, api := setupTelegramSync(t)
	var posts []TelegramPost
	for i := 0; i < 21; i++ {
		posts = append(posts, TelegramPost{ID: fmt.Sprintf("post-%d", i), Text: fmt.Sprintf("batch %d", i)})
	}
	if n, err := SyncTelegram(root, "site", posts, false, api); n != 20 || err == nil {
		t.Fatalf("batch: %d %v", n, err)
	}
	if api.sends != 20 {
		t.Fatalf("batch sent %d", api.sends)
	}
	if n, err := SyncTelegram(root, "site", posts, false, api); n != 1 || err != nil {
		t.Fatalf("remaining: %d %v", n, err)
	}
}

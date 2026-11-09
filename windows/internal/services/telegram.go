package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const telegramConfigLimit = 8 << 20
const telegramBatchLimit = 20

var telegramTokenPattern = regexp.MustCompile(`^[1-9][0-9]{4,19}:[A-Za-z0-9_-]{20,128}$`)
var telegramUsernamePattern = regexp.MustCompile(`^@[A-Za-z][A-Za-z0-9_]{4,31}$`)
var telegramHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type TelegramConfig struct {
	Token     string                     `json:"token"`
	ChatID    string                     `json:"chatId"`
	ChatTitle string                     `json:"chatTitle"`
	Messages  map[string]TelegramMessage `json:"messages,omitempty"`
}

type TelegramMessage struct {
	MessageID int64  `json:"messageId"`
	Hash      string `json:"hash"`
	Pending   bool   `json:"pending,omitempty"`
}

type TelegramPost struct {
	ID   string
	Text string
}

type TelegramChat struct {
	ID    string
	Title string
	Type  string
}

type TelegramAPI interface {
	ResolveChat(token, chatID string) (TelegramChat, error)
	Send(token, chatID, text string) (int64, error)
	Edit(token, chatID string, messageID int64, text string) error
}

// TelegramAPIError denotes an explicit Telegram API rejection. Description is
// interpreted through a fixed vocabulary so API responses cannot expose tokens.
type TelegramAPIError struct {
	Code        int
	Description string
}

func (e *TelegramAPIError) Error() string {
	description := "Telegram 拒绝请求，请检查机器人和目标设置"
	switch {
	case strings.Contains(strings.ToLower(e.Description), "message is not modified"):
		description = "Telegram 消息内容未变化"
	case strings.Contains(strings.ToLower(e.Description), "chat not found"):
		description = "找不到 Telegram 群组或频道，请检查目标并添加机器人"
	case strings.Contains(strings.ToLower(e.Description), "message to edit not found"):
		description = "找不到要更新的 Telegram 消息，请检查消息是否已删除"
	case strings.Contains(strings.ToLower(e.Description), "message can't be edited"):
		description = "Telegram 消息无法编辑，请检查机器人权限"
	case e.Code == 401:
		description = "Telegram 机器人 Token 无效"
	case e.Code == 403:
		description = "Telegram 机器人无权操作，请检查群组成员资格或频道发布权限"
	case e.Code == 429:
		description = "Telegram 请求过于频繁，请稍后重试"
	}
	return fmt.Sprintf("%s（%d）", description, e.Code)
}

type TelegramClient struct{ http *http.Client }

func NewTelegramClient() *TelegramClient {
	return &TelegramClient{http: &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func ValidTelegramToken(token string) bool { return telegramTokenPattern.MatchString(token) }

func telegramNumericChatID(chatID string) bool {
	id, err := strconv.ParseInt(chatID, 10, 64)
	return err == nil && id < 0 && strconv.FormatInt(id, 10) == chatID
}

func ValidTelegramTarget(chatID string) bool {
	return telegramUsernamePattern.MatchString(chatID) || telegramNumericChatID(chatID)
}

func validTelegramText(text string) bool {
	if !utf8.ValidString(text) || strings.TrimSpace(text) == "" {
		return false
	}
	units := 0
	for _, r := range text {
		units++
		if r > 0xffff {
			units++
		}
		if units > 4096 {
			return false
		}
	}
	return true
}

func (c *TelegramClient) request(token, method string, payload any, result any) error {
	if !ValidTelegramToken(token) {
		return errors.New("Telegram 机器人 Token 格式无效")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.New("Telegram 请求内容无效")
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.telegram.org/bot"+token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return errors.New("Telegram 请求无效")
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return errors.New("无法连接 Telegram，发送结果可能不确定")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return errors.New("Telegram 响应读取失败，发送结果可能不确定")
	}
	var envelope struct {
		OK          *bool           `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Code        int             `json:"error_code"`
		Description string          `json:"description"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.OK == nil {
		return errors.New("Telegram 响应无效，发送结果可能不确定")
	}
	if !*envelope.OK && envelope.Code >= 400 && envelope.Code < 500 {
		return &TelegramAPIError{Code: envelope.Code, Description: envelope.Description}
	}
	if !*envelope.OK || response.StatusCode < 200 || response.StatusCode >= 300 || json.Unmarshal(envelope.Result, result) != nil {
		return errors.New("Telegram 响应无效，发送结果可能不确定")
	}
	return nil
}

func (c *TelegramClient) ResolveChat(token, chatID string) (TelegramChat, error) {
	if !ValidTelegramTarget(chatID) {
		return TelegramChat{}, errors.New("Telegram 目标须为 @频道用户名或负数群组 ID")
	}
	var result struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
		Type  string `json:"type"`
	}
	if err := c.request(token, "getChat", map[string]string{"chat_id": chatID}, &result); err != nil {
		return TelegramChat{}, err
	}
	if result.ID >= 0 || (result.Type != "group" && result.Type != "supergroup" && result.Type != "channel") {
		return TelegramChat{}, errors.New("Telegram 目标必须是群组或频道，不能使用私聊")
	}
	return TelegramChat{ID: strconv.FormatInt(result.ID, 10), Title: result.Title, Type: result.Type}, nil
}

func (c *TelegramClient) Send(token, chatID, text string) (int64, error) {
	if !telegramNumericChatID(chatID) || !validTelegramText(text) {
		return 0, errors.New("Telegram 目标或消息无效（消息上限为 4096 字符）")
	}
	var result struct {
		MessageID int64 `json:"message_id"`
	}
	if err := c.request(token, "sendMessage", map[string]any{"chat_id": chatID, "text": text}, &result); err != nil {
		return 0, err
	}
	if result.MessageID <= 0 {
		return 0, errors.New("Telegram 未返回消息标识，发送结果不确定")
	}
	return result.MessageID, nil
}

func (c *TelegramClient) Edit(token, chatID string, messageID int64, text string) error {
	if !telegramNumericChatID(chatID) || messageID <= 0 || !validTelegramText(text) {
		return errors.New("Telegram 目标或消息无效（消息上限为 4096 字符）")
	}
	var result struct {
		MessageID int64 `json:"message_id"`
	}
	err := c.request(token, "editMessageText", map[string]any{"chat_id": chatID, "message_id": messageID, "text": text}, &result)
	var rejected *TelegramAPIError
	if errors.As(err, &rejected) && rejected.Code == 400 && strings.HasPrefix(strings.ToLower(rejected.Description), "bad request: message is not modified") {
		return nil
	}
	if err != nil {
		return err
	}
	if result.MessageID != messageID {
		return errors.New("Telegram 编辑响应无效，请重试更新")
	}
	return nil
}

func validTelegramConfig(cfg TelegramConfig) bool {
	if !ValidTelegramToken(cfg.Token) || !telegramNumericChatID(cfg.ChatID) || !utf8.ValidString(cfg.ChatTitle) || len(cfg.ChatTitle) > 1024 {
		return false
	}
	for id, message := range cfg.Messages {
		if !safeID.MatchString(id) || message.MessageID < 0 || !telegramHashPattern.MatchString(message.Hash) {
			return false
		}
	}
	return true
}

func telegramConfigFile(root, siteID string, create bool) (string, os.FileInfo, error) {
	path, err := configPath(root, siteID, "telegram", create)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	if info != nil && !info.Mode().IsRegular() {
		return "", nil, errors.New("Telegram 配置文件无效")
	}
	return path, info, nil
}

func SaveTelegramConfig(root, siteID string, cfg TelegramConfig) error {
	if !validTelegramConfig(cfg) {
		return errors.New("Telegram 配置无效")
	}
	raw, err := json.Marshal(cfg)
	if err != nil || len(raw) > telegramConfigLimit {
		return errors.New("Telegram 配置超过 8 MiB 上限")
	}
	sealed, err := protect(raw)
	if err != nil {
		return errors.New("无法保护 Telegram 配置")
	}
	if len(sealed) > telegramConfigLimit {
		return errors.New("Telegram 配置超过 8 MiB 上限")
	}
	path, _, err := telegramConfigFile(root, siteID, true)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".telegram-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(sealed)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if _, _, err = telegramConfigFile(root, siteID, false); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func LoadTelegramConfig(root, siteID string) (TelegramConfig, error) {
	path, info, err := telegramConfigFile(root, siteID, false)
	if errors.Is(err, os.ErrNotExist) || (err == nil && info == nil) {
		return TelegramConfig{}, nil
	}
	if err != nil {
		return TelegramConfig{}, err
	}
	if info.Size() > telegramConfigLimit {
		return TelegramConfig{}, errors.New("Telegram 配置超过 8 MiB 上限")
	}
	f, err := os.Open(path)
	if err != nil {
		return TelegramConfig{}, err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return TelegramConfig{}, errors.New("Telegram 配置读取失败")
	}
	data, err := io.ReadAll(io.LimitReader(f, telegramConfigLimit+1))
	if err != nil || len(data) > telegramConfigLimit {
		return TelegramConfig{}, errors.New("Telegram 配置读取失败")
	}
	raw, err := unprotect(data)
	if err != nil {
		return TelegramConfig{}, errors.New("无法解锁 Telegram 配置")
	}
	if len(raw) > telegramConfigLimit {
		return TelegramConfig{}, errors.New("Telegram 配置超过 8 MiB 上限")
	}
	var cfg TelegramConfig
	if json.Unmarshal(raw, &cfg) != nil || !validTelegramConfig(cfg) {
		return TelegramConfig{}, errors.New("Telegram 配置无效")
	}
	return cfg, nil
}

func DeleteTelegramConfig(root, siteID string) error {
	path, info, err := telegramConfigFile(root, siteID, false)
	if errors.Is(err, os.ErrNotExist) || (err == nil && info == nil) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func telegramSyncError(err error, sending bool) error {
	var rejected *TelegramAPIError
	if errors.As(err, &rejected) {
		return errors.New(rejected.Error())
	}
	if sending {
		return errors.New("Telegram 发送结果不确定；请检查群组或频道后手动重试，重试可能产生重复消息")
	}
	return errors.New("Telegram 消息更新失败，可再次同步重试")
}

// SyncTelegram requires the caller to serialize configuration changes and syncs
// for a site. A pending send is persisted before the network side effect; only
// an explicit retry may resubmit a send whose response was lost.
func SyncTelegram(root, siteID string, posts []TelegramPost, retry bool, api TelegramAPI) (int, error) {
	cfg, err := LoadTelegramConfig(root, siteID)
	if err != nil || cfg.Token == "" {
		return 0, err
	}
	if api == nil {
		return 0, errors.New("Telegram 客户端不可用")
	}
	if cfg.Messages == nil {
		cfg.Messages = make(map[string]TelegramMessage)
	}
	completed := 0
	for _, post := range posts {
		if !safeID.MatchString(post.ID) || !validTelegramText(post.Text) {
			return completed, errors.New("Telegram 文章标识或消息无效（消息上限为 4096 字符）")
		}
		digest := sha256.Sum256([]byte(post.Text))
		hash := hex.EncodeToString(digest[:])
		previous, exists := cfg.Messages[post.ID]
		if previous.MessageID > 0 && previous.Hash == hash && !previous.Pending {
			continue
		}
		if completed >= telegramBatchLimit {
			return completed, errors.New("本次已同步 20 篇文章，仍有文章待同步，请再次同步")
		}
		if previous.MessageID == 0 && previous.Pending && !retry {
			return completed, errors.New("上次 Telegram 发送结果不确定；请检查群组或频道后手动重试，重试可能产生重复消息")
		}
		messageID := previous.MessageID
		if messageID > 0 {
			if err := api.Edit(cfg.Token, cfg.ChatID, messageID, post.Text); err != nil {
				return completed, telegramSyncError(err, false)
			}
		} else {
			cfg.Messages[post.ID] = TelegramMessage{Hash: hash, Pending: true}
			if err := SaveTelegramConfig(root, siteID, cfg); err != nil {
				return completed, errors.New("无法保存 Telegram 待发送状态，已停止同步")
			}
			messageID, err = api.Send(cfg.Token, cfg.ChatID, post.Text)
			if err != nil {
				var rejected *TelegramAPIError
				if errors.As(err, &rejected) && rejected.Code >= 400 && rejected.Code < 500 {
					if exists {
						cfg.Messages[post.ID] = previous
					} else {
						delete(cfg.Messages, post.ID)
					}
					if saveErr := SaveTelegramConfig(root, siteID, cfg); saveErr != nil {
						return completed, errors.New("Telegram 拒绝发送，且本地状态保存失败；请检查配置后手动重试")
					}
				}
				return completed, telegramSyncError(err, true)
			}
			if messageID <= 0 {
				return completed, telegramSyncError(errors.New("missing message ID"), true)
			}
		}
		completed++
		cfg.Messages[post.ID] = TelegramMessage{MessageID: messageID, Hash: hash}
		if err := SaveTelegramConfig(root, siteID, cfg); err != nil {
			return completed, errors.New("Telegram 已同步，但本地状态保存失败；请检查群组或频道后手动重试")
		}
	}
	return completed, nil
}

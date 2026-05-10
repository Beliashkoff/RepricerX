package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/config"
	"github.com/Beliashkoff/RepricerX/internal/pkg/logger"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type telegramAPI struct {
	token  string
	client *http.Client
}

type updateResponse struct {
	OK     bool             `json:"ok"`
	Result []telegramUpdate `json:"result"`
}

type telegramUpdate struct {
	UpdateID int             `json:"update_id"`
	Message  telegramMessage `json:"message"`
}

type telegramMessage struct {
	Text string       `json:"text"`
	Chat telegramChat `json:"chat"`
	From telegramUser `json:"from"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramUser struct {
	Username string `json:"username"`
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	log := logger.New(cfg.Environment).With("module", "bot")
	if cfg.TelegramBotToken == "" {
		log.Info("telegram disabled: TELEGRAM_BOT_TOKEN is empty")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	bot := &telegramAPI{token: cfg.TelegramBotToken, client: &http.Client{Timeout: 35 * time.Second}}
	links := repository.NewTelegramLinksRepository(pool)
	users := repository.NewUsersRepository(pool)

	log.Info("telegram long-polling started")
	offset := 0
	for ctx.Err() == nil {
		updates, err := bot.getUpdates(ctx, offset)
		if err != nil {
			log.Warn("telegram getUpdates", "error", bot.safeError(err))
			sleep(ctx, 2*time.Second)
			continue
		}
		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			handleUpdate(ctx, log, bot, links, users, u)
		}
	}
	log.Info("telegram bot stopped")
}

func handleUpdate(ctx context.Context, log *slog.Logger, bot *telegramAPI, links repository.TelegramLinksRepository, users repository.UsersRepository, u telegramUpdate) {
	text := strings.TrimSpace(u.Message.Text)
	if text == "" || u.Message.Chat.ID == 0 {
		return
	}
	fields := strings.Fields(text)
	cmd := strings.SplitN(fields[0], "@", 2)[0]
	chatID := u.Message.Chat.ID

	switch cmd {
	case "/start":
		if len(fields) < 2 {
			_ = bot.sendMessage(ctx, chatID, "Откройте ссылку привязки из настроек RepricerX.")
			return
		}
		link, err := links.Confirm(ctx, fields[1], chatID, u.Message.From.Username)
		if err != nil {
			log.Warn("telegram confirm", "error", err)
			_ = bot.sendMessage(ctx, chatID, "Ссылка привязки недействительна или истекла.")
			return
		}
		_ = users.SetTelegramMutedUntil(ctx, link.UserID, nil)
		_ = bot.sendMessage(ctx, chatID, "Привязка успешна. Уведомления RepricerX будут приходить сюда.")
	case "/unlink":
		if err := links.UnlinkByChatID(ctx, chatID); err != nil && !errors.Is(err, repository.ErrNotFound) {
			log.Warn("telegram unlink", "error", err)
			_ = bot.sendMessage(ctx, chatID, "Не удалось отвязать Telegram.")
			return
		}
		_ = bot.sendMessage(ctx, chatID, "Telegram отвязан от RepricerX.")
	case "/mute":
		hours := 1
		if len(fields) >= 2 {
			if n, err := strconv.Atoi(fields[1]); err == nil && n > 0 && n <= 168 {
				hours = n
			}
		}
		link, err := links.GetByChatID(ctx, chatID)
		if err != nil {
			_ = bot.sendMessage(ctx, chatID, "Сначала привяжите Telegram в настройках RepricerX.")
			return
		}
		until := time.Now().UTC().Add(time.Duration(hours) * time.Hour)
		if err := users.SetTelegramMutedUntil(ctx, link.UserID, &until); err != nil {
			log.Warn("telegram mute", "error", err)
			_ = bot.sendMessage(ctx, chatID, "Не удалось включить mute.")
			return
		}
		_ = bot.sendMessage(ctx, chatID, fmt.Sprintf("Уведомления Telegram отключены на %d ч.", hours))
	default:
		_ = bot.sendMessage(ctx, chatID, "Доступные команды: /unlink, /mute <hours>.")
	}
}

func (b *telegramAPI) getUpdates(ctx context.Context, offset int) ([]telegramUpdate, error) {
	q := url.Values{}
	q.Set("timeout", "30")
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.telegram.org/bot"+b.token+"/getUpdates?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram status %d", resp.StatusCode)
	}
	var out updateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("telegram ok=false")
	}
	return out.Result, nil
}

func (b *telegramAPI) sendMessage(ctx context.Context, chatID int64, text string) error {
	form := url.Values{}
	form.Set("chat_id", fmt.Sprintf("%d", chatID))
	form.Set("text", text)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.telegram.org/bot"+b.token+"/sendMessage",
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram send status %d", resp.StatusCode)
	}
	return nil
}

func (b *telegramAPI) safeError(err error) string {
	if err == nil {
		return ""
	}
	return strings.ReplaceAll(err.Error(), b.token, "<redacted>")
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

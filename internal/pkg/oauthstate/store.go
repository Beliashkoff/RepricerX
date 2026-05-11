// Package oauthstate хранит короткоживущие OAuth state-токены и link-токены
// в Redis. State-токен связывает /start и /callback (передаёт PKCE verifier);
// link-токен передаёт «надо ввести пароль для привязки» между callback'ом и
// фронтовой страницей /link-oauth.
//
// Оба ключа потребляются атомарно через GETDEL — гарантирует, что один токен
// нельзя использовать дважды.
package oauthstate

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
)

const (
	stateKeyPrefix = "rx:oauth:state:"
	linkKeyPrefix  = "rx:oauth:link:"
	defaultTimeout = 2 * time.Second
)

// ErrNotFound — ключ отсутствует или уже потреблён.
var ErrNotFound = errors.New("oauthstate: not found")

// StatePayload — данные, которые сохраняются по `state` между /start и /callback.
type StatePayload struct {
	Provider     domain.OAuthProvider `json:"provider"`
	CodeVerifier string               `json:"code_verifier"`
}

// LinkPayload — данные для подтверждения привязки OAuth-идентичности к
// существующему email-аккаунту (callback → /link-oauth → ConfirmOAuthLink).
type LinkPayload struct {
	Provider       domain.OAuthProvider `json:"provider"`
	ExternalUserID string               `json:"external_user_id"`
	Email          string               `json:"email"`
	UserID         uuid.UUID            `json:"user_id"`
	DisplayName    string               `json:"display_name"`
}

// Store — интерфейс для использования сервисом auth (мокается в тестах).
type Store interface {
	SaveState(ctx context.Context, state string, p StatePayload, ttl time.Duration) error
	ConsumeState(ctx context.Context, state string) (StatePayload, error)
	SaveLink(ctx context.Context, token string, p LinkPayload, ttl time.Duration) error
	ConsumeLink(ctx context.Context, token string) (LinkPayload, error)
}

// RedisStore — реализация Store поверх Redis (минимальный RESP-клиент).
type RedisStore struct {
	addr    string
	timeout time.Duration
}

func NewRedisStore(addr string) *RedisStore {
	return &RedisStore{addr: addr, timeout: defaultTimeout}
}

func (s *RedisStore) SaveState(ctx context.Context, state string, p StatePayload, ttl time.Duration) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("oauthstate: marshal: %w", err)
	}
	return s.setEx(ctx, stateKeyPrefix+state, string(raw), ttl)
}

func (s *RedisStore) ConsumeState(ctx context.Context, state string) (StatePayload, error) {
	raw, err := s.getDel(ctx, stateKeyPrefix+state)
	if err != nil {
		return StatePayload{}, err
	}
	var p StatePayload
	if err = json.Unmarshal([]byte(raw), &p); err != nil {
		return StatePayload{}, fmt.Errorf("oauthstate: unmarshal: %w", err)
	}
	return p, nil
}

func (s *RedisStore) SaveLink(ctx context.Context, token string, p LinkPayload, ttl time.Duration) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("oauthstate: marshal: %w", err)
	}
	return s.setEx(ctx, linkKeyPrefix+token, string(raw), ttl)
}

func (s *RedisStore) ConsumeLink(ctx context.Context, token string) (LinkPayload, error) {
	raw, err := s.getDel(ctx, linkKeyPrefix+token)
	if err != nil {
		return LinkPayload{}, err
	}
	var p LinkPayload
	if err = json.Unmarshal([]byte(raw), &p); err != nil {
		return LinkPayload{}, fmt.Errorf("oauthstate: unmarshal: %w", err)
	}
	return p, nil
}

// setEx — SET key value EX <ttl_seconds>
func (s *RedisStore) setEx(ctx context.Context, key, value string, ttl time.Duration) error {
	seconds := int64(ttl.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	reply, err := s.do(ctx, "SET", key, value, "EX", strconv.FormatInt(seconds, 10))
	if err != nil {
		return err
	}
	// SET возвращает "OK" (simple string) при успехе.
	if str, ok := reply.(string); ok && str == "OK" {
		return nil
	}
	return fmt.Errorf("oauthstate: unexpected SET reply: %T", reply)
}

// getDel — GETDEL key (Redis 6.2+). Возвращает значение и удаляет ключ атомарно.
func (s *RedisStore) getDel(ctx context.Context, key string) (string, error) {
	reply, err := s.do(ctx, "GETDEL", key)
	if err != nil {
		return "", err
	}
	if reply == nil {
		return "", ErrNotFound
	}
	str, ok := reply.(string)
	if !ok {
		return "", fmt.Errorf("oauthstate: unexpected GETDEL reply: %T", reply)
	}
	return str, nil
}

// do отправляет одну команду Redis и читает ответ.
func (s *RedisStore) do(ctx context.Context, parts ...string) (any, error) {
	timeout := s.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return nil, fmt.Errorf("oauthstate: dial: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("oauthstate: deadline: %w", err)
	}
	if _, err = conn.Write(encodeCommand(parts...)); err != nil {
		return nil, fmt.Errorf("oauthstate: write: %w", err)
	}
	return readRESP(bufio.NewReader(conn))
}

func encodeCommand(parts ...string) []byte {
	var b strings.Builder
	b.WriteString("*")
	b.WriteString(strconv.Itoa(len(parts)))
	b.WriteString("\r\n")
	for _, part := range parts {
		b.WriteString("$")
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteString("\r\n")
		b.WriteString(part)
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

func readRESP(r *bufio.Reader) (any, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+':
		return readLine(r)
	case '-':
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		return nil, errors.New(line)
	case ':':
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		return strconv.ParseInt(line, 10, 64)
	case '$':
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err = io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	default:
		return nil, fmt.Errorf("oauthstate: unknown RESP prefix %q", prefix)
	}
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

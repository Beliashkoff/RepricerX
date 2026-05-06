package redislimit

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const allowScript = `
local current = redis.call("INCR", KEYS[1])
if current == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
local ttl = redis.call("PTTL", KEYS[1])
return {current, ttl}
`

type Result struct {
	Allowed    bool
	Count      int
	Limit      int
	RetryAfter time.Duration
}

type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error)
}

type RedisLimiter struct {
	addr    string
	prefix  string
	timeout time.Duration
}

func New(addr, prefix string) *RedisLimiter {
	if prefix == "" {
		prefix = "rx:rl:"
	}
	return &RedisLimiter{addr: addr, prefix: prefix, timeout: 2 * time.Second}
}

func Key(scope, value string) string {
	sum := sha256.Sum256([]byte(value))
	return strings.Trim(scope, ":") + ":" + hex.EncodeToString(sum[:])
}

func (l *RedisLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error) {
	if limit <= 0 || window <= 0 {
		return Result{Allowed: true, Limit: limit}, nil
	}
	if l == nil || l.addr == "" {
		return Result{}, errors.New("redis limiter is not configured")
	}

	timeout := l.timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", l.addr)
	if err != nil {
		return Result{}, fmt.Errorf("redis limiter dial: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return Result{}, fmt.Errorf("redis limiter deadline: %w", err)
	}

	windowMS := window.Milliseconds()
	if windowMS < 1 {
		windowMS = 1
	}
	if _, err = conn.Write(encodeCommand("EVAL", allowScript, "1", l.prefix+key, strconv.FormatInt(windowMS, 10))); err != nil {
		return Result{}, fmt.Errorf("redis limiter write: %w", err)
	}

	reply, err := readRESP(bufio.NewReader(conn))
	if err != nil {
		return Result{}, fmt.Errorf("redis limiter read: %w", err)
	}
	values, ok := reply.([]any)
	if !ok || len(values) != 2 {
		return Result{}, fmt.Errorf("redis limiter unexpected reply: %T", reply)
	}
	count, ok := values[0].(int64)
	if !ok {
		return Result{}, fmt.Errorf("redis limiter unexpected count: %T", values[0])
	}
	ttlMS, ok := values[1].(int64)
	if !ok {
		return Result{}, fmt.Errorf("redis limiter unexpected ttl: %T", values[1])
	}
	retryAfter := time.Duration(ttlMS) * time.Millisecond
	if retryAfter < 0 {
		retryAfter = window
	}
	return Result{
		Allowed:    int(count) <= limit,
		Count:      int(count),
		Limit:      limit,
		RetryAfter: retryAfter,
	}, nil
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
		line, err := readLine(r)
		return line, err
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
	case '*':
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
		values := make([]any, n)
		for i := 0; i < n; i++ {
			values[i], err = readRESP(r)
			if err != nil {
				return nil, err
			}
		}
		return values, nil
	default:
		return nil, fmt.Errorf("redis limiter unknown RESP prefix %q", prefix)
	}
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

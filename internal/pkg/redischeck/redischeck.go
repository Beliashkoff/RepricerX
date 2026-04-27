package redischeck

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

const pingCommand = "*1\r\n$4\r\nPING\r\n"

// Ping performs a minimal Redis RESP PING without pulling a full client
// dependency before queues/cache are implemented.
func Ping(ctx context.Context, addr string) error {
	if addr == "" {
		return errors.New("redis address is empty")
	}

	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("redis dial: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("redis deadline: %w", err)
	}

	if _, err = conn.Write([]byte(pingCommand)); err != nil {
		return fmt.Errorf("redis ping write: %w", err)
	}

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return fmt.Errorf("redis ping read: %w", err)
	}
	if strings.TrimSpace(line) != "+PONG" {
		return fmt.Errorf("redis ping unexpected response: %q", strings.TrimSpace(line))
	}
	return nil
}

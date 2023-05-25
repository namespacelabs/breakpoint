package sshd

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/rs/zerolog"
)

func keepAlive(ctx context.Context, logger zerolog.Logger, session ssh.Session) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			t := time.Now()
			if _, err := session.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				if !errors.Is(err, io.EOF) {
					logger.Err(err).Msg("Failed to send keepalive")
				} else {
					return
				}
			} else {
				logger.Debug().Dur("took", time.Since(t)).Msg("Got KeepAlive response")
			}

		case <-ctx.Done():
			return
		}
	}
}

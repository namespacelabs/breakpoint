package sshd

import (
	"io"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	"github.com/rs/zerolog"
)

func makeSftpHandler(logger zerolog.Logger) ssh.SubsystemHandler {
	return func(sess ssh.Session) {
		server, err := sftp.NewServer(sess, sftp.WithDebug(io.Discard))
		if err != nil {
			logger.Err(err).Msg("sftp: failed to init server")
			return
		}

		defer server.Close()

		if err := server.Serve(); err != nil && err != io.EOF {
			logger.Err(err).Msg("sftp: session done with error")
		} else {
			logger.Info().Msg("sftp: session done")
		}
	}
}

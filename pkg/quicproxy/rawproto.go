package quicproxy

import (
	"context"
	"fmt"
	"net"

	"github.com/rs/zerolog"
)

type RawFrontend struct {
	PublicAddr string
}

func (rf RawFrontend) ListenAndServe(ctx context.Context) error {
	return nil
}

func (rf RawFrontend) Handle(ctx context.Context, handlers Handlers) error {
	var d net.ListenConfig
	listener, err := d.Listen(ctx, "tcp", "0.0.0.0:0")
	if err != nil {
		return err
	}

	// If the context is canceled (e.g. the registration stream breaks), also
	// stop the listener.
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	// If we leave the Serve handler for reasons other than the listener
	// closing, make sure it's closed.
	defer func() {
		_ = listener.Close()
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	alloc := Allocation{Endpoint: fmt.Sprintf("%s:%d", rf.PublicAddr, port)}

	if err := handlers.OnAllocation(alloc); err != nil {
		return err
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			if handlers.OnCleanup != nil {
				handlers.OnCleanup(alloc, err)
			}
			return err
		}

		zerolog.Ctx(ctx).Debug().Stringer("remote_addr", conn.RemoteAddr()).
			Stringer("local_addr", conn.LocalAddr()).
			Str("allocation", alloc.Endpoint).Msg("New connection")

		go handlers.HandleConn(conn)
	}
}

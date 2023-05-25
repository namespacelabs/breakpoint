package quicproxy

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"

	proxyproto "github.com/pires/go-proxyproto"
	"github.com/rs/zerolog"
)

type ProxyProtoFrontend struct {
	ListenPort         int
	PortStart, PortEnd int
	PublicAddr         string

	mu    sync.RWMutex
	alloc map[int]func(net.Conn)
}

func (pf *ProxyProtoFrontend) ListenAndServe(ctx context.Context) error {
	var l net.ListenConfig
	lst, err := l.Listen(ctx, "tcp", fmt.Sprintf(":%d", pf.ListenPort))
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		_ = lst.Close()
	}()

	proxyListener := &proxyproto.Listener{Listener: lst}

	for {
		conn, err := proxyListener.Accept()
		if err != nil {
			return err
		}

		l := zerolog.Ctx(ctx).With().Stringer("remote_addr", conn.RemoteAddr()).
			Stringer("local_addr", conn.LocalAddr()).Logger()

		if tcpaddr, ok := conn.LocalAddr().(*net.TCPAddr); ok {
			go func() {
				pf.mu.RLock()
				handler, ok := pf.alloc[tcpaddr.Port]
				if ok {
					l.Debug().Msg("New connection")
					// Call handler with the rlock held to make sure we're
					// always handling streams consistently. Handler will
					// quickly spawn a go routine and return.
					handler(conn)
				} else {
					l.Debug().Msg("No match")
				}
				pf.mu.RUnlock()

				// Close without holding the lock.
				if !ok {
					_ = conn.Close()
				}
			}()
		} else {
			l.Debug().Msg("Ignored non-tcp")
			_ = conn.Close()
		}
	}
}

func (pf *ProxyProtoFrontend) allocate(ctx context.Context, handler func(net.Conn)) (int, func(), error) {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	// XXX naive; move to pre-shuffle.
	for i := 0; i < 100; i++ {
		port := pf.PortStart + rand.Int()%(pf.PortEnd-pf.PortStart)
		if _, ok := pf.alloc[port]; !ok {
			if pf.alloc == nil {
				pf.alloc = map[int]func(net.Conn){}
			}
			pf.alloc[port] = handler
			return port, func() {
				pf.mu.Lock()
				delete(pf.alloc, port)
				pf.mu.Unlock()
			}, nil
		}
	}

	return -1, nil, errors.New("failed to allocate port")
}

func (pf *ProxyProtoFrontend) Handle(ctx context.Context, handlers Handlers) error {
	port, cleanup, err := pf.allocate(ctx, func(conn net.Conn) {
		go handlers.HandleConn(conn)
	})

	if err != nil {
		return err
	}

	defer cleanup()

	alloc := Allocation{Endpoint: fmt.Sprintf("%s:%d", pf.PublicAddr, port)}

	if err := handlers.OnAllocation(alloc); err != nil {
		return err
	}

	<-ctx.Done()
	ctxErr := ctx.Err()

	if handlers.OnCleanup != nil {
		handlers.OnCleanup(alloc, ctxErr)
	}

	return ctxErr
}

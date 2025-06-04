package waiter

import (
	"context"
	"io"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog"
	v1 "namespacelabs.dev/breakpoint/api/private/v1"
	"namespacelabs.dev/breakpoint/pkg/webhook"
)

const (
	logTickInterval = 1 * time.Minute

	Stamp = time.Stamp + " MST"
)

type ManagerOpts struct {
	InitialDur time.Duration

	Webhooks  []v1.Webhook
	SlackBots []v1.SlackBot
}

type ManagerStatus struct {
	Endpoint       string    `json:"endpoint"`
	Expiration     time.Time `json:"expiration"`
	NumConnections uint32    `json:"num_connections"`
}

type Manager struct {
	ctx    context.Context
	logger zerolog.Logger

	opts ManagerOpts

	mu                      sync.Mutex
	updated                 chan struct{}
	expiration              time.Time
	endpoint                string
	resources               []io.Closer
	connectionCountCallback func() uint32
}

func NewManager(ctx context.Context, opts ManagerOpts) (*Manager, context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	l := zerolog.Ctx(ctx).With().Logger()
	m := &Manager{
		ctx:        ctx,
		logger:     l,
		opts:       opts,
		updated:    make(chan struct{}, 1),
		expiration: time.Now().Add(opts.InitialDur),
	}

	go func() {
		defer cancel()
		m.loop(ctx)

		m.mu.Lock()
		resources := m.resources
		m.resources = nil
		m.mu.Unlock()

		// Resources should clean up quickly as they hold up the cancelation of the context.
		// We're guaranteed to wait for these because the incoming `ctx` is never cancelled.
		for _, closer := range resources {
			if err := closer.Close(); err != nil {
				l.Err(err).Msg("Failed while cleaning up resource")
			}
		}
	}()

	return m, ctx
}

func (m *Manager) Wait() error {
	<-m.ctx.Done()
	return m.ctx.Err()
}

func (m *Manager) loop(ctx context.Context) {
	exitTimer := time.NewTicker(time.Until(m.expiration))
	defer exitTimer.Stop()

	logTicker := time.NewTicker(logTick())
	defer logTicker.Stop()

	for {
		select {
		case _, ok := <-m.updated:
			if !ok {
				return
			}

			m.mu.Lock()
			newExp := m.expiration
			m.mu.Unlock()

			exitTimer.Reset(time.Until(newExp))
			m.announce()

		case <-exitTimer.C:
			// Timer has expired, terminate the program
			m.logger.Info().Msg("Breakpoint expired")
			return

		case <-logTicker.C:
			m.announce()

		case <-ctx.Done():
			return
		}
	}
}

func logTick() time.Duration {
	// If running in CI, announce on a regular basis.
	if os.Getenv("CI") != "" {
		return logTickInterval
	}

	return math.MaxInt64
}

func (m *Manager) ExtendWait(dur time.Duration) time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.expiration = m.expiration.Add(dur)

	m.updated <- struct{}{}

	m.logger.Info().
		Dur("dur", dur).
		Time("expiration", m.expiration).
		Msg("Extend wait")
	return m.expiration
}

func (m *Manager) StopWait() {
	m.logger.Info().Msg("Resume requested")
	close(m.updated)
}

func (m *Manager) Expiration() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.expiration
}

func (m *Manager) Endpoint() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.endpoint
}

func (m *Manager) Status() ManagerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return ManagerStatus{
		Endpoint:       m.endpoint,
		Expiration:     m.expiration,
		NumConnections: m.connectionCountCallback(),
	}
}

func (m *Manager) SetEndpoint(addr string) {
	m.mu.Lock()
	m.endpoint = addr
	m.mu.Unlock()

	var resources []io.Closer
	for _, bot := range m.opts.SlackBots {
		if bot := startBot(m.ctx, m, bot); bot != nil {
			resources = append(resources, bot)
		}
	}

	m.mu.Lock()
	m.resources = resources
	m.mu.Unlock()

	m.updated <- struct{}{}

	expandf := expand(addr, m.Expiration())

	for _, wh := range m.opts.Webhooks {
		ctx, done := context.WithTimeout(m.ctx, 30*time.Second)
		defer done()

		payload := execTemplate(wh.Payload, expandf)

		t := time.Now()
		if err := webhook.Notify(ctx, os.Expand(wh.URL, expandf), payload); err != nil {
			m.logger.Err(err).Msg("Failed to notify Webhook")
		} else {
			m.logger.Info().Dur("took", time.Since(t)).Str("url", wh.URL).Msg("Notified webhook")
		}
	}
}

func (m *Manager) SetConnectionCountCallback(callback func() uint32) {
	m.mu.Lock()
	m.connectionCountCallback = callback
	m.mu.Unlock()
}

func expand(addr string, exp time.Time) func(key string) string {
	host, port, _ := net.SplitHostPort(addr)

	return func(key string) string {
		switch key {
		case "BREAKPOINT_ENDPOINT":
			return addr

		case "BREAKPOINT_HOST":
			return host

		case "BREAKPOINT_PORT":
			return port

		case "BREAKPOINT_TIME_LEFT":
			return strings.TrimSpace(humanize.RelTime(exp, time.Now(), "", ""))

		case "BREAKPOINT_EXPIRATION":
			return exp.Format(Stamp)
		}

		return os.Getenv(key)
	}
}

func (m *Manager) announce() {
	status := m.Status()
	PrintConnectionInfo(status.Endpoint, status.Expiration, os.Stderr)
}

func nchars(ch rune, n int) string {
	str := make([]rune, n)
	for k := 0; k < n; k++ {
		str[k] = ch
	}
	return string(str)
}

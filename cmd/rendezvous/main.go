package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"

	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"namespacelabs.dev/breakpoint/pkg/blog"
	"namespacelabs.dev/breakpoint/pkg/quicproxy"
	"namespacelabs.dev/breakpoint/pkg/tlscerts"
)

var (
	listenOn         = flag.String("l", "", "The address:port to listen on.")
	publicAddress    = flag.String("pub", "", "If unset, defaults to listen address.")
	subjectDomains   = flag.String("sub", "", "Attaches the specified domain names as TLS cert subjects.")
	frontend         = flag.String("frontend", "", "If specified, configures the frontend (in JSON).")
	httpPort         = flag.Int("http_port", 10020, "Where we listen on HTTP.")
	enableGitHubOIDC = flag.Bool("validate_github_oidc", false, "Validate GitHub OIDC tokens.")
	redirectTarget   = flag.String("redirect_target", "https://github.com/namespacelabs/breakpoint", "Where to redirect users to when accessed via HTTP.")
)

type frontendConfig struct {
	Kind       string `json:"kind"`
	PortStart  int    `json:"port_start"`
	PortEnd    int    `json:"port_end"`
	PortListen int    `json:"listen_port"`
}

func main() {
	flag.Parse()

	var fcfg frontendConfig
	if frontendData := flagOrEnv("PROXY_FRONTEND", *frontend); frontendData != "" {
		if err := json.Unmarshal([]byte(frontendData), &fcfg); err != nil {
			log.Fatal(err)
		}
	}

	var domains []string
	if val := flagOrEnv("PROXY_DOMAINS", *subjectDomains); len(val) > 0 {
		domains = strings.Split(val, ",")
	}

	if err := run(Config{
		ListenAddr:       flagOrEnv("PROXY_LISTEN", *listenOn),
		HttpPort:         *httpPort,
		FrontendConfig:   fcfg,
		PublicAddr:       flagOrEnv("PROXY_PUBLIC", *publicAddress),
		Domains:          domains,
		EnableGitHubOIDC: flagOrEnvBool("PROXY_VALIDATE_GITHUB_OIDC", *enableGitHubOIDC),
		RedirectURL:      *redirectTarget,
	}); err != nil {
		log.Fatal(err)
	}
}

func flagOrEnv(env, flag string) string {
	if flag != "" {
		return flag
	}

	return os.Getenv(env)
}

func flagOrEnvBool(env string, flag bool) bool {
	return flag || os.Getenv(env) == "true" || os.Getenv(env) == "1"
}

type Config struct {
	ListenAddr       string
	HttpPort         int
	FrontendConfig   frontendConfig
	PublicAddr       string
	Domains          []string
	EnableGitHubOIDC bool
	RedirectURL      string
}

func run(opts Config) error {
	if opts.ListenAddr == "" {
		return errors.New("-l or PROXY_LISTEN is required")
	}

	if opts.PublicAddr == "" {
		addrport, err := netip.ParseAddrPort(opts.ListenAddr)
		if err != nil {
			return err
		}

		opts.PublicAddr = addrport.Addr().String()
	}

	subjects := tlscerts.Subjects{
		DNSNames: opts.Domains,
	}

	if addr, err := netip.ParseAddr(opts.PublicAddr); err == nil {
		if !addr.IsUnspecified() {
			subjects.IPAddresses = append(subjects.IPAddresses, net.IP(addr.AsSlice()))
		}
	} else {
		if !slices.Contains(subjects.DNSNames, opts.PublicAddr) {
			subjects.DNSNames = append(subjects.DNSNames, opts.PublicAddr)
		}
	}

	frontend := makeFrontend(opts.FrontendConfig, opts.PublicAddr)

	l := blog.New()
	ctx := l.WithContext(context.Background())

	proxy, err := quicproxy.NewServer(ctx, quicproxy.ServerOpts{
		ProxyFrontend:    frontend,
		ListenAddr:       opts.ListenAddr,
		Subjects:         subjects,
		EnableGitHubOIDC: opts.EnableGitHubOIDC,
	})
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return frontend.ListenAndServe(ctx)
	})

	eg.Go(func() error {
		return proxy.Serve(ctx)
	})

	eg.Go(func() error {
		h := http.NewServeMux()

		h.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", opts.RedirectURL)
			w.WriteHeader(http.StatusTemporaryRedirect)
			fmt.Fprintf(w, "Heading over to <a href=%q>%s</a>", opts.RedirectURL, opts.RedirectURL)
		}))

		return http.ListenAndServe(fmt.Sprintf(":%d", opts.HttpPort), h)
	})

	return eg.Wait()
}

func makeFrontend(fcfg frontendConfig, pub string) quicproxy.ProxyFrontend {
	switch fcfg.Kind {
	case "proxy_proto":
		return &quicproxy.ProxyProtoFrontend{
			ListenPort: fcfg.PortListen,
			PortStart:  fcfg.PortStart,
			PortEnd:    fcfg.PortEnd,
			PublicAddr: pub,
		}

	default:
		return quicproxy.RawFrontend{
			PublicAddr: pub,
		}
	}
}

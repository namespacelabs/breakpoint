package quicproxy

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"github.com/MicahParks/keyfunc"
	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	apipb "namespacelabs.dev/breakpoint/api/public/v1"
	"namespacelabs.dev/breakpoint/pkg/githuboidc"
	"namespacelabs.dev/breakpoint/pkg/quicgrpc"
	"namespacelabs.dev/breakpoint/pkg/quicnet"
	"namespacelabs.dev/breakpoint/pkg/quicproxyclient"
	"namespacelabs.dev/breakpoint/pkg/tlscerts"
)

type Server struct {
	p        ProxyFrontend
	listener quic.Listener
	ghJWKS   *keyfunc.JWKS
}

type ServerOpts struct {
	ProxyFrontend    ProxyFrontend
	ListenAddr       string
	Subjects         tlscerts.Subjects
	EnableGitHubOIDC bool
}

func NewServer(ctx context.Context, opts ServerOpts) (*Server, error) {
	t := time.Now()
	public, private, err := tlscerts.GenerateECDSAPair(opts.Subjects, 365*24*time.Hour)
	if err != nil {
		return nil, err
	}
	zerolog.Ctx(ctx).Info().Dur("took", time.Since(t)).Msg("Generated new keys")

	srv := &Server{p: opts.ProxyFrontend}

	if opts.EnableGitHubOIDC {
		t = time.Now()
		jwks, err := githuboidc.ProvideVerifier(ctx)
		if err != nil {
			return nil, err
		}
		zerolog.Ctx(ctx).Info().Dur("took", time.Since(t)).Msg("Prepared GitHub JWKS")
		srv.ghJWKS = jwks
	}

	cert, err := tls.X509KeyPair(public, private)
	if err != nil {
		return nil, err
	}

	tlsconf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{apipb.QuicProto},
	}

	listener, err := quic.ListenAddr(opts.ListenAddr, tlsconf, quicproxyclient.DefaultConfig)
	if err != nil {
		return nil, err
	}

	srv.listener = *listener
	return srv, nil
}

func (srv *Server) Close() error {
	return srv.listener.Close()
}

func (srv *Server) Serve(ctx context.Context) error {
	zerolog.Ctx(ctx).Info().Str("addr", srv.listener.Addr().String()).Msg("Listening")

	grpcServer := grpc.NewServer(grpc.Creds(quicgrpc.QuicCreds{NonQuicCreds: insecure.NewCredentials()}))
	apipb.RegisterProxyServiceServer(grpcServer, server{
		logger:   zerolog.Ctx(ctx).With().Logger(),
		frontend: srv.p,
		ghJWKS:   srv.ghJWKS,
	})
	return grpcServer.Serve(quicnet.NewListener(ctx, srv.listener))
}

type server struct {
	apipb.UnimplementedProxyServiceServer

	logger   zerolog.Logger
	frontend ProxyFrontend
	ghJWKS   *keyfunc.JWKS

	restrictToRepositories []string
	restrictToOwners       []string
}

func (srv server) Register(req *apipb.RegisterRequest, server apipb.ProxyService_RegisterServer) error {
	peer, _ := peer.FromContext(server.Context())
	quic, ok := peer.AuthInfo.(quicgrpc.QuicAuthInfo)
	if !ok {
		return errors.New("internal error, expected quic")
	}

	githubClaims, logger := validateGitHubOIDC(server.Context(), srv.logger, srv.ghJWKS)

	if len(srv.restrictToRepositories) > 0 {
		if githubClaims == nil || !slices.Contains(srv.restrictToRepositories, githubClaims.Repository) {
			return status.Errorf(codes.PermissionDenied, "repository %q not allowed", githubClaims.Repository)
		}
	}

	if len(srv.restrictToOwners) > 0 {
		if githubClaims == nil || !slices.Contains(srv.restrictToOwners, githubClaims.RepositoryOwner) {
			return status.Errorf(codes.PermissionDenied, "repository owner %q not allowed", githubClaims.RepositoryOwner)
		}
	}

	return ServeProxy(logger.WithContext(server.Context()), srv.frontend, quic.Conn, func(alloc Allocation) error {
		return server.Send(&apipb.RegisterResponse{Endpoint: alloc.Endpoint})
	})
}

func validateGitHubOIDC(ctx context.Context, logger zerolog.Logger, jwks *keyfunc.JWKS) (*githuboidc.Claims, zerolog.Logger) {
	if jwks != nil {
		md, _ := metadata.FromIncomingContext(ctx)
		if token, ok := md[apipb.GitHubOIDCTokenHeader]; ok && len(token) > 0 {
			claims, err := githuboidc.Validate(ctx, jwks, token[0])

			if err != nil {
				logger.Warn().Err(err).Msg("Failed to validate GitHub OIDC Token")
			} else if slices.Contains(claims.Audience, apipb.GitHubOIDCAudience) {
				logger.Warn().Str("expected", apipb.GitHubOIDCAudience).Strs("audience", claims.Audience).
					Msg("Failed to validate GitHub OIDC Token audience")
			} else {
				return claims, logger.With().Str("repository", claims.Repository).Logger()
			}
		}
	}

	return nil, logger
}

package githuboidc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MicahParks/keyfunc"
	"github.com/golang-jwt/jwt/v4"
	"github.com/rs/zerolog"
)

const (
	githubJWKSURL = "https://token.actions.githubusercontent.com/.well-known/jwks"
)

func ProvideVerifier(ctx context.Context) (*keyfunc.JWKS, error) {
	options := keyfunc.Options{
		Ctx: ctx,
		RefreshErrorHandler: func(err error) {
			zerolog.Ctx(ctx).Err(err).Str("jwks_url", githubJWKSURL).Msg("Failed to refresh JWKS")
		},
		RefreshInterval:   time.Hour,
		RefreshRateLimit:  time.Minute * 5,
		RefreshTimeout:    time.Second * 10,
		RefreshUnknownKID: true,
	}

	return keyfunc.Get(githubJWKSURL, options)
}

func Validate(ctx context.Context, jwks *keyfunc.JWKS, tokenStr string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenStr, claims, jwks.Keyfunc)
	if err != nil {
		return nil, fmt.Errorf("failed to verify Github JWT: %w", err)
	}

	if !token.Valid {
		return nil, errors.New("invalid Github JWT")
	}

	return claims, nil
}

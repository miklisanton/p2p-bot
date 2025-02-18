package JWTConfig

import (
	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type JWTCustomClaims struct {
	ChatID int64 `json:"chat_id"`
	jwt.RegisteredClaims
}

func NewJWTConfig(secret string) echojwt.Config {
	return echojwt.Config{
		NewClaimsFunc: func(c echo.Context) jwt.Claims {
			return new(JWTCustomClaims)
		},
		SigningKey:  []byte(secret),
		TokenLookup: "header:Authorization:Bearer ",
		ErrorHandler: func(c echo.Context, err error) error {
			log.Error().Err(err).Msg("JWT error")
			log.Debug().Str("token", c.Request().Header.Get("Authorization")).Msg("Token")
			return echo.ErrUnauthorized
		},
	}
}

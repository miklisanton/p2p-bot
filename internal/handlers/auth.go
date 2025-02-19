package handlers

import (
	"database/sql"
	"net/http"
	"os"
	"p2pbot/internal/JWTConfig"
	"p2pbot/internal/db/models"
	"p2pbot/internal/rediscl"
	"p2pbot/internal/requests"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	initdata "github.com/telegram-mini-apps/init-data-golang"
	"github.com/teris-io/shortid"
)

// This POST request handler is used to sync auth0 users with database
func (contr *Controller) Signup(c echo.Context) error {
	u := new(requests.Auth0Request)
	if err := c.Bind(u); err != nil {
		return err
	}

	log.Debug().Fields(map[string]interface{}{
		"email":  u.Email,
		"secret": u.Secret,
	}).Msg("Signup request")

	if u.Secret != os.Getenv("AUTH0_SIGNUP_SECRET") {
		log.Error().Msg("Invalid secret in signup request")
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"message": "Invalid secret",
		})
	}

	_, err := contr.userService.CreateUser(&models.User{
		Email: &u.Email,
	})

	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"message": "User created",
		"user": map[string]any{
			"email": u.Email,
		},
	})
}

func (contr *Controller) GetProfile(c echo.Context) error {
	email := c.Get("email").(string)
	u, err := contr.userService.GetUserByEmail(email)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "User not found",
			"errors": map[string]any{
				"user": "not found",
			},
		})
	}
	if err != nil {
		return err
	}
	log.Info().Fields(map[string]interface{}{
		"email":   u.Email,
		"chat_id": u.ChatID,
	}).Msg("User found")
	return c.JSON(http.StatusOK, map[string]any{
		"message": "User found",
		"user": map[string]any{
			"email":    u.Email,
			"telegram": u.ChatID,
		},
	})
}

func (contr *Controller) ConnectTelegram(c echo.Context) error {
	email := c.Get("email").(string)
	u, err := contr.userService.GetUserByEmail(email)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "User not found",
			"errors": map[string]any{
				"user": "not found",
			},
		})
	}
	if err != nil {
		return err
	}
	// generate code with shortid
	code, err := shortid.Generate()
	if err != nil {
		return err
	}
	// save code to redis
	ctx := rediscl.RDB.Ctx
	if err := rediscl.RDB.Client.Set(ctx, "telegram_codes:"+code, u.ID, 15*time.Minute).Err(); err != nil {
		return err
	}
	// send link to user
	link := contr.TgLink + "?start=" + code
	return c.JSON(http.StatusOK, map[string]any{
		"message": "Connect your telegram",
		"link":    link,
	})
}

func (cont *Controller) GetCSRFToken(c echo.Context) error {
	csrf := c.Get("csrf").(string)
	return c.JSON(http.StatusOK, map[string]any{
		"csrf": csrf,
	})
}

// Login is used for telegram mini app authentication
func (cont *Controller) Login(c echo.Context) error {
	// Get initdata from request header
	auth := c.Request().Header.Get("Authorization")
	data := strings.Split(auth, " ")
	if len(data) != 2 || data[0] != "tma" {
		return echo.ErrUnauthorized
	}
	// Validate initdata
	if err := initdata.Validate(data[1], cont.BotSecret, 24*time.Hour); err != nil {
		return echo.ErrUnauthorized
	}
	// Find user in database
	initParsed, err := initdata.Parse(data[1])
	if err != nil {
		return err
	}
	user, err := cont.userService.GetUserByChatID(initParsed.Chat.ID)
	if err == sql.ErrNoRows {
		// Create user if not found
		user = &models.User{
			ChatID: &initParsed.Chat.ID,
		}
		id, err := cont.userService.CreateUser(user)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create user")
			return err
		}
		log.Info().Int("id", id).Msg("User created")
	} else {
		log.Info().Int("id", user.ID).Msg("User logged in")
	}
	// Get user's subscription
	subscription, err := cont.subscriptionsService.GetByUserId(user.ID)
	if err != nil {
		return err
	}
	// Mark subscription as expired if it is expired
	if subscription != nil {
		var expired bool
		if time.Now().Before(subscription.ValidUntil) {
			expired = false
		} else {
			expired = true
		}
		subscription.Expired = &expired
	}
	// Issue JWT
	claims := JWTConfig.JWTCustomClaims{
		ChatID: initParsed.Chat.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	t, err := token.SignedString([]byte(cont.JWTSecret))
	if err != nil {
		log.Error().Err(err).Msg("Failed to sign token")
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message":      "Logged in",
		"token":        t,
		"subscription": subscription,
	})
}

package main

import (
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog/log"
	"net/http"
	"p2pbot/internal/app"
	"p2pbot/internal/db/repository"
	"p2pbot/internal/handlers"
	"p2pbot/internal/rediscl"
	"p2pbot/internal/services"
	"p2pbot/internal/utils"
)

func main() {

	DB, cfg, err := app.Init()
	if err != nil {
		panic(err)
	}

	userRepo := repository.NewUserRepository(DB)
	userService := services.NewUserService(userRepo)
	trackerRepo := repository.NewTrackerRepository(DB)
	trackerService := services.NewTrackerService(trackerRepo)
	subscriptionRepo := repository.NewSubscriptionRepository(DB)
	subscriptionService := services.NewSubscriptionService(subscriptionRepo)

	binance := services.NewBinanceExchange(cfg)
	bybit := services.NewBybitExcahnge(cfg)

	rediscl.InitRedisClient(cfg.Redis.Host, cfg.Redis.Port)

	controller := handlers.NewController(
		userService,
		trackerService,
		subscriptionService,
		map[string]services.ExchangeI{
			"binance": binance,
			"bybit":   bybit,
		},
		cfg,
	)

	e := echo.New()
	e.Use(utils.LoggingMiddleware)

	e.Use(echomiddleware.CORSWithConfig(echomiddleware.CORSConfig{
		AllowOrigins: []string{
			"https://p2phub.top",
			"https://dev.p2phub.top",
			"https://localhost",
			"https://dev.localhost",
			"https://localhost:443",
			"https://localhost:8443",
			"http://127.0.0.1",
			"http://127.0.0.1:5173",
		},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderXCSRFToken,
			echo.HeaderAuthorization,
			"X-Client-Id",
		},
		AllowCredentials: true,
	}))

	publicGroup := e.Group("/api/v1/public")
	//Route for auth0 signup webhook
	publicGroup.POST("/signup", controller.Signup)
	//Route for tg mini app login
	publicGroup.POST("/telegram/login", controller.Login)
	//CSRF token
	publicGroup.GET("/csrf", controller.GetCSRFToken)
	// Route for payment webhook
	publicGroup.POST("/subscriptions/confirm", controller.ConfirmOrder)

	privateGroup := e.Group("/api/v1/private")

	//JWT middleware
	privateGroup.Use(utils.AuthMiddleware)
	// Identify user
	privateGroup.Use(utils.ExtractID)
	// tracker routes
	privateGroup.GET("/trackers", controller.GetTrackers)
	privateGroup.POST("/trackers", controller.CreateTracker)
	privateGroup.GET("/trackers/:id", controller.GetTracker)
	privateGroup.DELETE("/trackers/:id", controller.DeleteTracker)
	privateGroup.PATCH("/trackers/:id", controller.SetNotifyTracker)
	// tracker options for forms
	privateGroup.GET("/trackers/options/methods", controller.GetPaymentMethods)
	privateGroup.GET("/trackers/options/currencies", controller.GetCurrencies)
	privateGroup.GET("/trackers/options/exchanges", controller.GetExchanges)
	// User routes
	privateGroup.GET("/profile", controller.GetProfile)
	// connect telegram route
	privateGroup.POST("/telegram/connect", controller.ConnectTelegram)
	privateGroup.GET("/test", controller.TestFunc)
	// Subscription routes
	privateGroup.POST("/subscriptions", controller.CreateOrder)
	publicGroup.POST("/subscriptions/callback", controller.ConfirmOrder)
	privateGroup.GET("/subscriptions", controller.GetSubscription)

	server := &http.Server{
		Addr:    ":" + cfg.Website.BackendPort,
		Handler: e,
	}
	log.Info().Msgf("Server is running on port %s", cfg.Website.BackendPort)
	e.Logger.Fatal(server.ListenAndServe())
}

package main

import (
	"context"
	"fmt"
	"github.com/rs/zerolog/log"
	"p2pbot/internal/app"
	"p2pbot/internal/db/repository"
	"p2pbot/internal/rabbitmq"
	"p2pbot/internal/rediscl"
	"p2pbot/internal/services"
	"p2pbot/internal/tasks"
	"time"
)

func main() {
	// wait until all services are up
	time.Sleep(10 * time.Second)
	DB, cfg, err := app.Init()
	fmt.Println("DB: ", DB)
	if err != nil {
		panic(err)
	}

	trackerRepo := repository.NewTrackerRepository(DB)
	userRepo := repository.NewUserRepository(DB)
	trackerService := services.NewTrackerService(trackerRepo)
	userService := services.NewUserService(userRepo)
	subscriptionRepo := repository.NewSubscriptionRepository(DB)
	subscriptionService := services.NewSubscriptionService(subscriptionRepo)

	bybit := services.NewBybitExcahnge(cfg)
	binance := services.NewBinanceExchange(cfg)
	exs := []services.ExchangeI{binance, bybit}

	rabbit, err := rabbitmq.NewRabbitMQ(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Error starting rabbitmq")
	}

	rediscl.InitRedisClient(cfg.Redis.Host, cfg.Redis.Port)

	observer := tasks.NewAdsObserver(trackerService, userService, subscriptionService, exs, rabbit)

	ctx := context.Background()
	observer.Start(1*time.Minute, ctx)
}

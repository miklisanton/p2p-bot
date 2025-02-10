package main

import (
	"os"
	"p2pbot/internal/bot"
	"p2pbot/internal/config"
	"p2pbot/internal/utils"

	"github.com/rs/zerolog/log"
	"time"

	"github.com/rs/zerolog"
)

// Delete keyboard message after send
func main() {
	// wait until all services are up
	//	time.Sleep(10 * time.Second)
	//	DB, cfg, err := app.Init()
	//	if err != nil {
	//		panic(err)
	//	}
	//
	//	//url := "https://p2p.binance.com/bapi/c2c/v2/friendly/c2c/adv/search"
	//	//payload := `{"fiat":"CZK","page":1,"rows":10,"tradeType":"BUY","asset":"USDT","countries":[],"proMerchantAds":false,"shieldMerchantAds":false,"filterType":"all","periods":[],"additionalKycVerifyFilter":0,"publisherType":null,"payTypes":[],"classifies":["mass","profession"]}`
	//
	//	trackerRepo := repository.NewTrackerRepository(DB)
	//	userRepo := repository.NewUserRepository(DB)
	//
	//	trackerService := services.NewTrackerService(trackerRepo)
	//	userService := services.NewUserService(userRepo)
	//
	//	rediscl.InitRedisClient(cfg.Redis.Host, cfg.Redis.Port)
	//
	//	//Supported exchanges
	//	binance := services.NewBinanceExchange(cfg)
	//	bybit := services.NewBybitExcahnge(cfg)
	//	exs := []services.ExchangeI{binance, bybit}
	//
	//	tgbot, err := bot.NewBot(cfg, userService, trackerService, exs)
	//	if err != nil {
	//		log.Fatal("Error starting bot: ", err)
	//	}
	//	// Rabbitmq setup
	//	rabbit, err := rabbitmq.NewRabbitMQ(cfg)
	//	if err != nil {
	//		log.Fatal("Error starting rabbitmq: ", err)
	//	}
	//	if err := rabbit.DeclareExchange("notifications"); err != nil {
	//		log.Fatal("Error declaring exchange: ", err)
	//	}
	//	queueName, err := rabbit.QueueBindNDeclare()
	//	if err != nil {
	//		log.Fatal("Error declaring queue: ", err)
	//	}
	//	rabbit.StartConsuming(queueName, tgbot.HandleNotification)

	path, err := config.ParseCLI()
	if err != nil {
		log.Fatal().Err(err).Msg("Error parsing CLI")
	}

	cfg, err := config.NewConfig(path)
	if err != nil {
		log.Fatal().Err(err).Msg("Error reading config")
	}

	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC1123Z}).
		Level(zerolog.DebugLevel).
		With().
		Timestamp().
		Caller().
		Logger().
		Hook(utils.GoroutineHook{})

	tgbot, err := bot.NewBot(cfg, nil, nil, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("Error starting bot")
	}

	tgbot.Start()
}

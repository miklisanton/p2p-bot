package tasks

import (
	"fmt"
	"os"
	"p2pbot/internal/app"
	"p2pbot/internal/db/models"
	"p2pbot/internal/db/repository"
	"p2pbot/internal/rabbitmq"
	"p2pbot/internal/rediscl"
	"p2pbot/internal/services"
	"path/filepath"
	"testing"
	"time"
)

var observer *AdsObserver

func findProjectRoot() (string, error) {
	// Get the current working directory of the test
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Traverse upwards to find the project root (where .env file is located)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".env")); os.IsNotExist(err) {
			parent := filepath.Dir(dir)
			if parent == dir {
				// Reached the root of the filesystem, and .env wasn't found
				return "", os.ErrNotExist
			}
			dir = parent
		} else {
			return dir, nil
		}
	}
}

func TestMain(m *testing.M) {
	time.Sleep(3 * time.Second)
	root, err := findProjectRoot()
	fmt.Println("Root: ", root)
	if err != nil {
		panic("Error finding project root: " + err.Error())
	}

	err = os.Chdir(root)
	if err != nil {
		panic(err)
	}
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
		fmt.Println("Error: ", err)
	}

	rediscl.InitRedisClient(cfg.Redis.Host, cfg.Redis.Port)

	observer = NewAdsObserver(trackerService, userService, subscriptionService, exs, rabbit, 7, 7)

	m.Run()
}

func TestNotify(t *testing.T) {
	chatID := int64(7349692872)
	user := &models.User{
		ChatID: &chatID,
	}
	id, err := observer.userService.CreateUser(user)
	if err != nil {
		t.Fatalf("error saving user: %v", err)
	}
	tracker := &models.Tracker{
		ID:            1,
		UserID:        id,
		Exchange:      "binance",
		Currency:      "USD",
		Side:          "BUY",
		Username:      "test",
		Notify:        true,
		WaitingUpdate: false,
		IsAggregated:  false,
		Price:         10,
		Payment:       nil,
	}

	err = observer.trackerService.CreateTracker(tracker)
	if err != nil {
		t.Fatalf("error saving tracker: %v", err)
	}

	observer.Notify(tracker, nil)
	observer.Notify(tracker, nil)
	observer.Notify(tracker, nil)
	observer.Notify(tracker, nil)
}

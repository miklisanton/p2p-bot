package handlers

import (
	"p2pbot/internal/config"
	"p2pbot/internal/services"
)

type Controller struct {
	userService          *services.UserService
	trackerService       *services.TrackerService
	subscriptionsService *services.SubscriptionService
	exchanges            map[string]services.ExchangeI
	JWTSecret            string
	TgLink               string
	SubPrice             string
	SubCurrency          string
	BotSecret            string
}

func NewController(userService *services.UserService,
	trackerService *services.TrackerService,
	subscriptionsService *services.SubscriptionService,
	exs map[string]services.ExchangeI,
	cfg *config.Config) *Controller {

	return &Controller{
		userService,
		trackerService,
		subscriptionsService,
		exs,
		cfg.Website.JWTSecret,
		cfg.Telegram.InviteLink,
		cfg.Website.SubPrice,
		cfg.Website.SubCurrency,
		cfg.Telegram.APIkey,
	}

}

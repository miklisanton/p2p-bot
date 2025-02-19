package bot

import (
	"fmt"
	"github.com/rs/zerolog/log"
	"p2pbot/internal/config"
	"p2pbot/internal/rediscl"
	"p2pbot/internal/services"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

type Bot struct {
	api            *tgbotapi.BotAPI
	userService    *services.UserService
	trackerService *services.TrackerService
	NotificationCh chan services.Notification
	exchanges      []services.ExchangeI
	toDelete       []int
	miniAppURL     string
}

func NewBot(cfg *config.Config, userSvc *services.UserService, trackerSvc *services.TrackerService, exs []services.ExchangeI) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Telegram.APIkey)
	if err != nil {
		return nil, err
	}
	api.Debug = true

	return &Bot{
		api:            api,
		userService:    userSvc,
		trackerService: trackerSvc,
		NotificationCh: make(chan services.Notification),
		miniAppURL:     cfg.Telegram.MiniAppURL,
		exchanges:      exs}, nil
}

func (bot *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.api.GetUpdatesChan(u)
	log.Info().Msg("Bot started")

	for update := range updates {
		if update.Message == nil {
			continue
		}
		chatID := update.Message.Chat.ID

		switch update.Message.Command() {
		case "start":
			err := bot.HandleStart(update.Message)
			if err != nil {
				log.Error().Fields(map[string]interface{}{
					"error": err.Error(),
				}).Msg("bot message")
			}
		default:
			bot.SendMessage(chatID, "Unknown command")
		}
	}
}

func (bot *Bot) HandleStart(msg *tgbotapi.Message) error {
	args := strings.Split(msg.CommandArguments(), " ")
	if len(args) == 1 && args[0] != "" {
		// Handle telegram connect
		// Extract unique_code from /start command
		code := args[0]
		// Get user_id from redis, telegram_codes:unique_code
		ctx := rediscl.RDB.Ctx
		userID, err := rediscl.RDB.Client.Get(ctx, "telegram_codes:"+code).Result()
		if userID == "" || err == redis.Nil {
			bot.SendMessage(msg.Chat.ID, "Link doesn't exist or expired")
			return nil
		}
		if err != nil {
			return err
		}
		// Set chat_id for user
		uid, err := strconv.Atoi(userID)
		if err != nil {
			return err
		}
		user, err := bot.userService.GetUserByID(uid)
		if err != nil {
			return err
		}
		user.ChatID = &msg.Chat.ID
		if _, err := bot.userService.CreateUser(user); err != nil {
			if err, ok := err.(*pq.Error); ok && err.Code == "23505" {
				bot.SendMessage(msg.Chat.ID, "This telegram account is already connected. Contact support @p2phubb")
			}
			return err
		}
		// Delete unique_code from redis
		if err := rediscl.RDB.Client.Del(ctx, "telegram_codes:"+code).Err(); err != nil {
			return err
		}
		bot.SendMessage(msg.Chat.ID, "Successfully connected")
		return nil
	} else {
		// Send welcome message
		bot.SendMessage(msg.Chat.ID, "Welcome to P2P Hub.\n Press Trackers to launch the mini app")
		return nil
	}
}

func (bot *Bot) SendMessage(chatID int64, text string) int {
	msg := tgbotapi.NewMessage(chatID, text)
	msgSent, err := bot.api.Send(msg)
	if err != nil {
		log.Printf("Failed to send message: %v", err)
		return 0
	}

	return msgSent.MessageID
}

func (bot *Bot) SendMultiple(ids []int64, text string) {
	for _, id := range ids {
		bot.SendMessage(id, text)
	}
}

//func (bot *Bot) MonitorAds(refreshRate time.Duration) error {
//  ticker := time.NewTicker(refreshRate)
//  for range ticker.C {
//      trackers, err := bot.trackerService.GetAllTrackers()
//      if err != nil {
//          return fmt.Errorf("error getting trackers: %s", err)
//      }
//      for _, tracker := range trackers {
//          for _, exchange := range bot.exchanges {
//              if strings.ToLower(exchange.GetName()) != tracker.Exchange {
//                  continue
//              }
//              // Retrieve username on exchange
//              username, err := utils.GetField(tracker, exchange.GetName()+"Name")
//              if err != nil {
//                  return fmt.Errorf("error getting exchange name: %s", err)
//              }
//              // Skip, username not provided
//              if username == "" {
//                  continue
//              }
//
//              exchangeBestAdv, err := exchange.GetBestAdv(tracker.Currency, tracker.Side, tracker.Payment)
//              if err != nil {
//                    log.Printf("Error getting best adv on %s : %s",exchange.GetName(), err)
//                    continue
//              }
//              log.Printf("Best advertisement on %s is %s ", exchange.GetName(), exchangeBestAdv.GetName())
//              // Send notification if best advertisement doesn't match with tracker username
//              if exchangeBestAdv.GetName() != username {
//                  // Notify if user reacted on previous notification(updated his adv)
//                  if !tracker.Waiting {
//                      bot.NotificationCh <- utils.Notification{
//                          ChatID:    *tracker.ChatID,
//                          Data:      exchangeBestAdv,
//                          Exchange:  tracker.Exchange,
//                          Direction: tracker.Side,
//                          Currency:  tracker.Currency,
//                      }
//                  }
//                  // Set waiting_adv flag, until user puts his advertisement on top
//                  if err := bot.trackerService.SetWaitingFlag(tracker.ID, true); err != nil {
//                      return err
//                  }
//              } else {
//                  // User updated his adv, enable notifications
//                  if err := bot.trackerService.SetWaitingFlag(tracker.ID, false); err != nil {
//                      return err
//                  }
//              }
//          }
//      }
//  }
//  return nil
//}

func (bot *Bot) NotifyUsers() {
	for notification := range bot.NotificationCh {
		q, minAmount, maxAmount := notification.Data.GetQuantity()
		c := notification.Currency
		msg := fmt.Sprintf("Your %s order on %s was outbided by %s.\nPrice: %.3f %s\nQuantity: %.2fUSDT\nMin: %.2f%s\nMax: %.2f%s",
			notification.Side, notification.Exchange, notification.Data.GetName(), notification.Data.GetPrice(), c, q, minAmount, c, maxAmount, c)

		bot.SendMessage(notification.ChatID, msg)
		bot.SendMessage(notification.ChatID, "You won't get notifications until you update your order")
	}
}

func (bot *Bot) DeleteMessages(chatID int64) error {
	for _, msgID := range bot.toDelete {
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, msgID)
		_, err := bot.api.Send(deleteMsg)
		if err != nil {
			log.Printf("Failed to send message: %v", err)
		}
	}

	bot.toDelete = []int{}
	return nil
}

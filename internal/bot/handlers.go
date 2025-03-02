package bot

import (
	"encoding/json"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"p2pbot/internal/rediscl"
	"p2pbot/internal/services"
	"strconv"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

const WelcomeMessage = `🇬🇧Welcome!
To start using the tracker:
- Click "Create Tracker"
- Select a platform (Binance or Bybit)
- Enter your username on the chosen platform
- Specify the currency of your ad

Need help? Contact Support: @p2phubb

🇷🇺Добро пожаловать!
Чтобы начать использовать трекер:
- Нажмите "Create Tracker"
- Выберите площадку (Binance или Bybit)
- Введите имя пользователя на выбранной платформе
- Укажите валюту вашего объявления

Нужна помощь? Свяжитесь с поддержкой: @p2phubb`

func (bot *Bot) HandleNotification(msg amqp.Delivery) {
	if msg.ContentType == "application/json" {
		var tmp map[string]interface{}
		if err := json.Unmarshal(msg.Body, &tmp); err != nil {
			log.Error().Msg(err.Error())
			return
		}
		if _, ok := tmp["message"]; ok {
			// Simple text message
			bot.HandleSimpleNotification(msg)
		} else if _, ok := tmp["top_order"]; ok {
			// Tracker notification
			bot.HandleTrackerNotification(msg)
		}

	} else {
		log.Error().Str("msg body", string(msg.Body)).Msg("Invalid content type")
	}
}

func (bot *Bot) HandleTrackerNotification(msg amqp.Delivery) {
	var n services.Notification
	if err := json.Unmarshal(msg.Body, &n); err != nil {
		log.Error().Msg(err.Error())
		return
	}
	q, minA, maxA := n.Data.GetQuantity()
	price := n.Data.GetPrice()
	name := n.Data.GetName()
	// Obtain payment methods id to name translation
	var translation []services.PaymentMethod
	for _, ex := range bot.exchanges {
		if strings.ToLower(ex.GetName()) == n.Exchange {
			translation, _ = ex.GetCachedPaymentMethods(n.Currency)
			log.Debug().Interface("translation", translation).Msg("Translation")
			break
		}
	}

	pms := strings.Join(n.Data.GetMethodsPrintable(translation), ", ")

	template := `Your %s %s advertisement on %s was outbided by %s.
Payment methods: %s.
%.1f USDT
%.1f - %.1f %s
%.3f %s/USDT`
	message := fmt.Sprintf(
		template,
		n.Currency,
		n.Side,
		n.Exchange,
		name,
		pms,
		q,
		minA,
		maxA,
		n.Currency,
		price,
		n.Currency)
	bot.SendMessage(n.ChatID, message)
}

func (bot *Bot) HandleSimpleNotification(msg amqp.Delivery) {
	var n services.StringNotification
	if err := json.Unmarshal(msg.Body, &n); err != nil {
		log.Error().Msg(err.Error())
		return
	}
	bot.SendMessage(n.ChatID, n.Msg)
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
		bot.SendMessage(msg.Chat.ID, WelcomeMessage)
		return nil
	}
}

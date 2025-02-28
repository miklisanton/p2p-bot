package bot

import (
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"p2pbot/internal/services"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

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

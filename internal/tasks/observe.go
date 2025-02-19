package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"p2pbot/internal/db/models"
	"p2pbot/internal/rabbitmq"
	"p2pbot/internal/rediscl"
	"p2pbot/internal/services"
	"p2pbot/internal/utils"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type AdsObserver struct {
	trackerService       *services.TrackerService
	subscriptionsService *services.SubscriptionService
	userService          *services.UserService
	exchanges            []services.ExchangeI
	rabbitCl             *rabbitmq.RabbitMQ
}

func NewAdsObserver(
	trackerService *services.TrackerService,
	userService *services.UserService,
	subscriptionsService *services.SubscriptionService,
	exchanges []services.ExchangeI,
	rabbit *rabbitmq.RabbitMQ) *AdsObserver {
	return &AdsObserver{
		trackerService:       trackerService,
		userService:          userService,
		subscriptionsService: subscriptionsService,
		exchanges:            exchanges,
		rabbitCl:             rabbit,
	}
}

func (ao *AdsObserver) Start(rate time.Duration, ctx context.Context) {
	if err := ao.rabbitCl.DeclareExchange("notifications"); err != nil {
		log.Error().Err(err).Msg("Error declaring exchange")
		return
	}
	ao.rabbitCl.Publish([]byte("Starting ads observer"))
	// Check ads with given rate
	ao.CheckAds()
	ticker := time.NewTicker(rate)
	for {
		select {
		case <-ticker.C:
			ao.CheckAds()
		case <-ctx.Done():
			return
		}
	}
}

func (ao *AdsObserver) CheckAds() {
	var wg sync.WaitGroup
	for _, ex := range ao.exchanges {

		wg.Add(1)
		go func() {
			defer wg.Done()
			// Get map of "currency+side" -> [trackerID]
			idsMap, err := ao.trackerService.GetIdsByCurrency(strings.ToLower(ex.GetName()))
			log.Debug().Fields(map[string]any{
				"map": idsMap,
			}).Msg("monitoring ads")
			if err != nil {
				return
			}
			ao.CheckAdsOnExchange(ex, idsMap)
		}()
	}
	wg.Wait()
}

func (ao *AdsObserver) CheckAdsOnExchange(ex services.ExchangeI, idsMap map[string][]int) {
	log.Info().Msg("Checking ads on " + ex.GetName())
	var wg sync.WaitGroup
	for key, ids := range idsMap {
		wg.Add(1)
		go func() error {
			defer wg.Done()
			// key, for example: "CZKSELL"
			currency := key[:3]
			side := key[3:]
			ads, err := ex.GetAds(currency, side)
			if err != nil {
				return err
			}
			for _, id := range ids {
				ao.CheckTracker(ads, id)
			}
			return nil
		}()
	}
	wg.Wait()
	log.Info().Msg("Finished checking ads on " + ex.GetName())
}

func (ao *AdsObserver) CheckTracker(ads []services.P2PItemI, trackerID int) {
	tracker, err := ao.trackerService.GetTrackerById(trackerID)
	if err != nil {
		return
	}
	if tracker.IsAggregated {
		for _, ad := range ads {
			if utils.ComparePaymentMethods(ad.GetPaymentMethods(), tracker.Payment) {
				// if advertisements payment methods contain one of the tracker payment methods
				if ad.GetName() != tracker.Username && ad.GetPrice() != tracker.Price {
					// if advertisement name doesnt match tracker username
					// Notify user
					if !tracker.WaitingUpdate {
						ao.Notify(tracker, ad)
					}
					tracker.WaitingUpdate = true
					if err := ao.trackerService.CreateTracker(tracker); err != nil {
						log.Printf("Error updating tracker waiting update: %s", err)
					}
					return
				} else {
					// Tracked advertisement is the best advertisement across payment methods
					// Set outbidded to false
					tracker.WaitingUpdate = false
					tracker.Price = ad.GetPrice()
					log.Printf("User %s is not outbidded on %s", tracker.Username, tracker.Exchange)
					if err := ao.trackerService.CreateTracker(tracker); err != nil {
						log.Printf("Error updating tracker price: %s", err)
					}
					return
				}
			}
		}
	} else {
		for _, pMethod := range tracker.Payment {
			for _, ad := range ads {
				if utils.Contains(ad.GetPaymentMethods(), pMethod.Id) {
					if ad.GetName() != tracker.Username && ad.GetPrice() != tracker.Price {
						//Notify user
						if !pMethod.Outbided {
							ao.Notify(tracker, ad)
						}
						//Set outbidded to true
						err = ao.trackerService.UpdateMethodOutbiddded(tracker.ID, pMethod.Id, true)
						if err != nil {
							log.Printf("Error updating outbidded status for %s on %s", pMethod.Id, tracker.Exchange)
						}
					} else {
						//set outbidded to false
						err := ao.trackerService.UpdateMethodOutbiddded(tracker.ID, pMethod.Id, false)
						if err != nil {
							log.Printf("Error updating outbidded status for %s on %s", pMethod.Id, tracker.Exchange)
						}
						log.Printf("User %s is not outbidded on %s for %s", tracker.Username, tracker.Exchange, pMethod.Id)
						//Update tracker price
						tracker.Price = ad.GetPrice()
						if err := ao.trackerService.CreateTracker(tracker); err != nil {
							log.Printf("Error updating tracker price: %s", err)
						} else {
							log.Debug().Fields(map[string]interface{}{
								"id": tracker.ID,
							}).Msg("tracker updated")
						}
					}
					break
				}
			}
		}
	}
}

func (ao *AdsObserver) Notify(tracker *models.Tracker, ad services.P2PItemI) {
	user, err := ao.userService.GetUserByID(tracker.UserID)
	if err != nil {
		log.Error().Msg("Error retreiving user")
		return
	}
	if user.ChatID == nil {
		// if telegram  not connected
		log.Info().Msg(fmt.Sprintf("Can't sent notification, because user with userID %d has no telegram connected", user.ID))
		return
	}
	// Check if notifications enabled
	if !tracker.Notify {
		return
	}
	// Create notification
	n := services.Notification{
		Data:     ad,
		Exchange: tracker.Exchange,
		Side:     tracker.Side,
		Currency: tracker.Currency,
		ChatID:   *user.ChatID,
	}
	nJson, err := json.Marshal(n)
	if err != nil {
		log.Error().Msg("Error converting user to json")
	}
	// Check if user has active subscription, if not allow only 3 notifications a week
	subscription, err := ao.subscriptionsService.GetByUserId(user.ID)
	if err != nil {
		log.Error().Str("error ", err.Error()).Msg("Error getting subscription")
		return
	}
	if subscription == nil || subscription.ValidUntil.Before(time.Now()) {
		ctx := rediscl.RDB.Ctx
		count := rediscl.RDB.Client.Get(ctx, fmt.Sprintf("notification:%d", user.ID))
		if count.Err() == redis.Nil {
			rediscl.RDB.Client.Set(ctx, fmt.Sprintf("notification:%d", user.ID), 1, time.Hour*24*7)
		} else {
			c, err := count.Int()
			if err != nil {
				log.Error().Msg("Error getting notification count")
				return
			}
			if c > 3 {
				log.Info().Msg(fmt.Sprintf("User %d has reached notification limit", user.ID))
				return
			}
		}
		if err := ao.rabbitCl.Publish([]byte(nJson)); err != nil {
			log.Error().Fields(map[string]interface{}{
				"error": err.Error(),
			}).Msg("Error publishing message")
		}
		rediscl.RDB.Client.Incr(ctx, fmt.Sprintf("notification:%d", user.ID))
	} else {
		// Just publish notification if user has active subscription
		if err := ao.rabbitCl.Publish([]byte(nJson)); err != nil {
			log.Error().Fields(map[string]interface{}{
				"error": err.Error(),
			}).Msg("Error publishing message")
		}
	}
}

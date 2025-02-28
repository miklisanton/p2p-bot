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
	windowDays           time.Duration
	maxNotifications     int
}

func NewAdsObserver(
	trackerService *services.TrackerService,
	userService *services.UserService,
	subscriptionsService *services.SubscriptionService,
	exchanges []services.ExchangeI,
	rabbit *rabbitmq.RabbitMQ,
	days int,
	limit int) *AdsObserver {
	return &AdsObserver{
		trackerService:       trackerService,
		userService:          userService,
		subscriptionsService: subscriptionsService,
		exchanges:            exchanges,
		rabbitCl:             rabbit,
		windowDays:           time.Duration(days),
		maxNotifications:     limit,
	}
}

const LimitMessage = "You have reached your notification limit for this week. Buy a susbcription to get unlimited notifications."

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
				isPresent := ao.CheckTracker(ads, id)
				log.Debug().Bool("isPresent", isPresent).Int("id", id).Msg("tracker checked")
			}
			return nil
		}()
	}
	wg.Wait()
	log.Info().Msg("Finished checking ads on " + ex.GetName())
}

// Checks if tracked ad is present in ads slice
// checks if tracked advertisement is outbided
// and sends notification
// if advertisement is missing, return false
// if advertisement is present, return true
func (ao *AdsObserver) CheckTracker(ads []services.P2PItemI, trackerID int) bool {
	tracker, err := ao.trackerService.GetTrackerById(trackerID)
	if err != nil {
		return false
	}
	if tracker.IsAggregated {
		if !ao.IsAdPresent(tracker, ads) {
			log.Debug().Int64("tracker_id", tracker.ID).Msg("advertisement not found")
			return false
		}

		for _, ad := range ads {
			if utils.ComparePaymentMethods(ad.GetPaymentMethods(), tracker.Payment) {
				// if advertisements payment methods contain one of the tracker payment methods
				if ad.GetName() != tracker.Username && ad.GetPrice() != tracker.Price {
					// if advertisement name doesnt match tracker username
					if notified, err := ao.CheckAdNotified(tracker.UserID, ad); err != nil {
						log.Error().Msg("Error checking if ad is notified")
					} else if !notified {
						log.Info().Int64("tracker_id", tracker.ID).Str("adv_id", ad.GetId()).Float64("price", ad.GetPrice()).Msg("Sending notification")
						ao.Notify(tracker, ad)
					} else {
						log.Info().Int64("tracker_id", tracker.ID).Str("adv_id", ad.GetId()).Float64("price", ad.GetPrice()).Msg("Notification already sent, skipping")
					}
				} else {
					// Tracked advertisement found
					log.Debug().Int64("tracker_id", tracker.ID).Msg("found tracked ad")
					// Update tracker price
					tracker.Price = ad.GetPrice()
					if err := ao.trackerService.CreateTracker(tracker); err != nil {
						log.Printf("Error updating tracker price: %s", err)
					} else {
						log.Debug().Fields(map[string]interface{}{
							"id": tracker.ID,
						}).Msg("tracker updated")
					}
					return true
				}
			}
		}
		return false
	} else {
		// TODO reimplement this
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
		return false
	}
}

func (ao *AdsObserver) Notify(tracker *models.Tracker, ad services.P2PItemI) {
	// Set notified for ad+price combination
	ctx := rediscl.RDB.Ctx
	rediscl.RDB.Client.Set(ctx, fmt.Sprintf("user%d:%s:%f", tracker.UserID, ad.GetId(), ad.GetPrice()), "true", time.Hour*12)
	log.Debug().Str("key", fmt.Sprintf("user%d:%s:%f", tracker.UserID, ad.GetId(), ad.GetPrice())).Msg("Redis key set")

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
			rediscl.RDB.Client.Set(ctx, fmt.Sprintf("notification:%d", user.ID), 1, time.Hour*24*ao.windowDays)
		} else {
			c, err := count.Int()
			if err != nil {
				log.Error().Msg("Error getting notification count")
				return
			}
			if c > ao.maxNotifications {
				log.Info().Msg(fmt.Sprintf("User %d has reached notification limit", user.ID))
				// Send notification to user that he has reached limit
				if flag, err := ao.CheckTrialNotified(user.ID); err != nil {
					log.Error().Msg("Error checking if user is notified about trial limit")
					return
				} else if !flag {
					// Create notification
					sn := services.StringNotification{
						ChatID: *user.ChatID,
						Msg:    LimitMessage,
					}
					snJson, err := json.Marshal(sn)
					if err != nil {
						log.Error().Msg("Error converting user to json")
					}
					if err := ao.rabbitCl.Publish(snJson); err != nil {
						log.Error().Err(err).Msg("Error publishing message")
					}
					rediscl.RDB.Client.Set(ctx, fmt.Sprintf("trial:%d", user.ID), "true", time.Hour*24*ao.windowDays)
					return
				} else {
					log.Info().Int("uid", user.ID).Msg("User already notified about trial limit")
					return
				}
			}
		}
		if err := ao.rabbitCl.Publish([]byte(nJson)); err != nil {
			log.Error().Err(err).Msg("Error publishing message")
		}
		rediscl.RDB.Client.Incr(ctx, fmt.Sprintf("notification:%d", user.ID))
	} else {
		// Just publish notification if user has active subscription
		if err := ao.rabbitCl.Publish([]byte(nJson)); err != nil {
			log.Error().Err(err).Msg("Error publishing message")
		}
	}
}

func (ao *AdsObserver) CheckAdNotified(uid int, ad services.P2PItemI) (bool, error) {
	ctx := rediscl.RDB.Ctx
	notified := rediscl.RDB.Client.Get(ctx, fmt.Sprintf("user%d:%s:%f", uid, ad.GetId(), ad.GetPrice()))
	log.Debug().Str("value", notified.Val()).Msg("Redis value retreived")
	if notified.Err() == redis.Nil || notified.Val() == "" {
		return false, nil
	}
	if notified.Err() != nil {
		return false, notified.Err()
	}
	return notified.Val() == "true", nil
}

func (ao *AdsObserver) CheckTrialNotified(uid int) (bool, error) {
	ctx := rediscl.RDB.Ctx
	notified := rediscl.RDB.Client.Get(ctx, fmt.Sprintf("trial:%d", uid))
	log.Debug().Str("value", notified.Val()).Msg("Redis value retreived")
	if notified.Err() == redis.Nil || notified.Val() == "" {
		return false, nil
	}
	if notified.Err() != nil {
		return false, notified.Err()
	}
	return notified.Val() == "true", nil
}

func (ao *AdsObserver) IsAdPresent(tracker *models.Tracker, ads []services.P2PItemI) bool {
	for _, ad := range ads {
		if ad.GetName() == tracker.Username && utils.ComparePaymentMethods(ad.GetPaymentMethods(), tracker.Payment) {
			return true
		}
	}
	return false
}

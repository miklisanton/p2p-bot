package handlers

import (
	"database/sql"
	"fmt"
	"github.com/rs/zerolog/log"
	"net/http"
	"p2pbot/internal/db/models"
	"p2pbot/internal/requests"
	"p2pbot/internal/services"
	"p2pbot/internal/utils"
	"slices"
	"strconv"

	"github.com/labstack/echo/v4"
)

// GetTrackers gets all trackers for a user
// if page is not provided, it defaults to 1
// returns 10 trackers per page

func (contr *Controller) GetTrackers(c echo.Context) error {
	// Retreive user
	email, chatID, err := utils.RetreiveEmailNChatID(c)
	if err != nil {
		log.Error().Err(err).Msg("Error retreiving email and chatID")
		return err
	}
	u, err := contr.userService.GetUser(email, chatID)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "User not found",
			"errors": map[string]any{
				"user": "not found",
			},
		})
	}
	if err != nil {
		log.Error().Err(err).Msg("Error getting user")
		return err
	}
	// Get page
	page := c.QueryParam("page")
	if page == "" {
		page = "1"
	}
	p, err := strconv.Atoi(page)
	if err != nil {
		return err
	}
	if p < 1 {
		p = 1
	}
	// Get limit
	limit := c.QueryParam("limit")
	if limit == "" {
		limit = "1000"
	}
	l, err := strconv.Atoi(limit)
	if err != nil {
		return err
	}
	if l < 1 {
		l = 10
	}

	log.Info().Fields(map[string]interface{}{
		"email":   email,
		"chat_id": chatID,
		"page":    p,
	}).Msg("Trackers requested")

	trackers, err := contr.trackerService.GetTrackersByUserId(u.ID)
	if err != nil {
		return err
	}

	if len(trackers) == 0 {
		return c.JSON(http.StatusOK, map[string]any{
			"message":  "Trackers",
			"trackers": make([]models.Tracker, 0),
			"hasMore":  false,
		})
	}

	if (p-1)*l >= len(trackers) {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "No trackers found",
			"errors": map[string]any{
				"trackers": "not found",
			},
		})
	}

	hasMore := p*l < len(trackers)

	return c.JSON(http.StatusOK, map[string]any{
		"message":  fmt.Sprintf("Trackers for user %s", email),
		"trackers": trackers[(p-1)*l : min(p*l, len(trackers))],
		"hasMore":  hasMore,
	})
}

func (contr *Controller) GetTracker(c echo.Context) error {
	// Retreive user
	email, chatID, err := utils.RetreiveEmailNChatID(c)
	if err != nil {
		log.Error().Err(err).Msg("Error retreiving email and chatID")
		return err
	}
	u, err := contr.userService.GetUser(email, chatID)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "User not found",
			"errors": map[string]any{
				"user": "not found",
			},
		})
	}
	if err != nil {
		log.Error().Err(err).Msg("Error getting user")
		return err
	}

	trackerID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"message": "Invalid tracker ID",
			"errors": map[string]any{
				"tracker": "invalid ID",
			},
		})
	}
	tracker, err := contr.trackerService.GetTrackerById(trackerID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "Tracker not found",
			"errors": map[string]any{
				"tracker": "not found",
			},
		})
	}
	// Check if tracker created by user
	if tracker.UserID != u.ID {
		return c.JSON(http.StatusForbidden, map[string]any{
			"message": "Forbidden",
			"errors": map[string]any{
				"tracker": "not found",
			},
		})
	}
	return c.JSON(http.StatusFound, map[string]any{
		"message": "Tracker found",
		"tracker": tracker,
	})
}

func (contr *Controller) CreateTracker(c echo.Context) error {
	// Retreive user
	email, chatID, err := utils.RetreiveEmailNChatID(c)
	if err != nil {
		log.Error().Err(err).Msg("Error retreiving email and chatID")
		return err
	}
	u, err := contr.userService.GetUser(email, chatID)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "User not found",
			"errors": map[string]any{
				"user": "not found",
			},
		})
	}
	if err != nil {
		log.Error().Err(err).Msg("Error getting user")
		return err
	}

	trackerReq := new(requests.TrackerRequest)
	if err := c.Bind(trackerReq); err != nil {
		return err
	}

	// Create tracker entity and validate its fields
	if trackerReq.Notify == nil {
		trackerReq.Notify = new(bool)
		*trackerReq.Notify = false
	}
	tracker := &models.Tracker{
		UserID:   u.ID,
		Exchange: trackerReq.Exchange,
		Currency: trackerReq.Currency,
		Side:     trackerReq.Side,
		Username: trackerReq.Username,
		Notify:   *trackerReq.Notify,
		Payment:  make([]*models.PaymentMethod, 0),
	}
	// If no payments method provided in request, treat as aggregated tracker
	if len(trackerReq.Payment) == 0 {
		tracker.IsAggregated = true
	}
	// Recieve payment methods from request and add them to tracker
	for _, p := range trackerReq.Payment {
		tracker.Payment = append(tracker.Payment, &models.PaymentMethod{
			Id: p,
		})
	}

	if err := contr.trackerService.ValidateTracker(tracker, false); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"message": "Validation error",
			"errors": map[string]any{
				"invalid_param": err.Error(),
			},
		})
	}
	// Check if exchange is supported
	exchange, ok := contr.exchanges[tracker.Exchange]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"message": "exchange not found",
			"errors": map[string]any{
				"exchange": fmt.Sprintf("%s not supported", tracker.Exchange),
			},
		})
	}

	ads, err := exchange.GetAdsByName(tracker.Currency,
		tracker.Side,
		tracker.Username,
		trackerReq.Payment)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "error getting ads",
			"errors": map[string]any{
				exchange.GetName(): err.Error(),
			},
		})
	}

	log.Info().Fields(map[string]interface{}{
		"email":            email,
		exchange.GetName(): ads,
	}).Msg("Ads found")

	// needed for retreiving payment methods names from ids
	pMethods, err := exchange.GetCachedPaymentMethods(tracker.Currency)
	if err != nil {
		return err
	}
	createdTrackers := make([]models.Tracker, 0)
	for _, adv := range ads {
		// Set price
		tracker.Price = adv.GetPrice()
		// Set advID
		idStr := adv.GetId()
		advID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing adv ID")
			return err
		}
		tracker.AdvID = advID
		// Set payment methods
		var pmStrings []string
		if tracker.IsAggregated {
			// Recieve payment methods from ad if not provided in request(only for non-aggregated trackers)
			pmStrings = adv.GetPaymentMethods()
		} else {
			// Recieve payment methods from request if provided
			pmStrings = trackerReq.Payment
		}
		pms := make([]*models.PaymentMethod, 0)
		for _, p := range pmStrings {
			// Get name from id and add to tracker
			name, err := services.GetPMethodName(pMethods, p)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]any{
					"message": "payment method not found",
					"errors": map[string]any{
						"payment_method": fmt.Sprintf("%s not found", p),
					},
				})
			}
			pms = append(pms, &models.PaymentMethod{
				Id:   p,
				Name: name,
			})
		}
		tracker.Payment = pms
		// Add to DB
		if err = contr.trackerService.CreateTracker(tracker); err != nil {
			return err
		}
		// Add to response
		createdTrackers = append(createdTrackers, *tracker)
		// Log
		log.Debug().Fields(map[string]interface{}{
			"tracker": tracker,
		}).Msg("Tracker created")
		// Reset tracker ID
		tracker.ID = 0
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"message":  "Trackers created",
		"trackers": createdTrackers,
	})
}

func (contr *Controller) DeleteTracker(c echo.Context) error {
	// Retreive user
	email, chatID, err := utils.RetreiveEmailNChatID(c)
	if err != nil {
		log.Error().Err(err).Msg("Error retreiving email and chatID")
		return err
	}
	u, err := contr.userService.GetUser(email, chatID)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "User not found",
			"errors": map[string]any{
				"user": "not found",
			},
		})
	}
	if err != nil {
		log.Error().Err(err).Msg("Error getting user")
		return err
	}

	trackerID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"message": "Invalid tracker ID",
			"errors": map[string]any{
				"tracker": "invalid ID",
			},
		})
	}
	tracker, err := contr.trackerService.GetTrackerById(trackerID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "Tracker not found",
			"errors": map[string]any{
				"tracker": "not found",
			},
		})
	}
	// Check if tracker created by user
	if tracker.UserID != u.ID {
		return c.JSON(http.StatusForbidden, map[string]any{
			"message": "Forbidden",
			"errors": map[string]any{
				"tracker": "not found",
			},
		})
	}
	// Delete tracker from database
	err = contr.trackerService.DeleteTracker(trackerID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"message": "Tracker deleted",
		"tracker": tracker.ID,
	})
}

func (contr *Controller) SetNotifyTracker(c echo.Context) error {
	// Retreive user
	email, chatID, err := utils.RetreiveEmailNChatID(c)
	if err != nil {
		log.Error().Err(err).Msg("Error retreiving email and chatID")
		return err
	}
	u, err := contr.userService.GetUser(email, chatID)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "User not found",
			"errors": map[string]any{
				"user": "not found",
			},
		})
	}
	if err != nil {
		log.Error().Err(err).Msg("Error getting user")
		return err
	}

	trackerID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"message": "Invalid tracker ID",
			"errors": map[string]any{
				"tracker": "invalid ID",
			},
		})
	}
	tracker, err := contr.trackerService.GetTrackerById(trackerID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{
			"message": "Tracker not found",
			"errors": map[string]any{
				"tracker": "not found",
			},
		})
	}
	// Check if tracker created by user
	if tracker.UserID != u.ID {
		return c.JSON(http.StatusForbidden, map[string]any{
			"message": "Forbidden",
			"errors": map[string]any{
				"tracker": "not found",
			},
		})
	}

	trackerReq := new(requests.TrackerRequest)
	if err := c.Bind(trackerReq); err != nil {
		return err
	}

	// Update tracker
	if trackerReq.Notify != nil {
		tracker.Notify = *trackerReq.Notify
	}

	err = contr.trackerService.CreateTracker(tracker)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, map[string]any{
		"message":  "Tracker updated",
		"trackers": tracker,
	})
}

// Options related endpoints
func (contr *Controller) GetPaymentMethods(c echo.Context) error {
	// Retreive email and chatID
	email, chatID, err := utils.RetreiveEmailNChatID(c)
	if err != nil {
		log.Error().Err(err).Msg("Error retreiving email and chatID")
		return err
	}
	// Check query parameters
	exchange := c.QueryParam("exchange")
	if exchange == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"message": "exchange not found",
			"errors": map[string]any{
				"exchange": "query parameter not provided",
			},
		})
	}
	exch, ok := contr.exchanges[exchange]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"message": "exchange not found",
			"errors": map[string]any{
				"exchange": fmt.Sprintf("%s not supported", exchange),
			},
		})
	}

	currency := c.QueryParam("currency")
	if currency == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"message": "currency not found",
			"errors": map[string]any{
				"currency": "query parameter not provided",
			},
		})
	}

	supportedCurrencies, err := exch.GetCachedCurrencies()
	if err != nil {
		return err
	}
	if !utils.Contains(supportedCurrencies, currency) {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"message": "currency not found",
			"errors": map[string]any{
				"currency": fmt.Sprintf("%s not supported", currency),
			},
		})
	}
	out, err := exch.GetCachedPaymentMethods(currency)
	if err != nil {
		return err
	}

	log.Info().Fields(map[string]interface{}{
		"email":    email,
		"chat_id":  chatID,
		"exchange": exchange,
		"currency": currency,
		"options":  out,
	}).Msg("Payment methods requested")

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Form options",
		"options": out,
	})
}

func (contr *Controller) GetCurrencies(c echo.Context) error {
	// Retreive email and chatID
	email, chatID, err := utils.RetreiveEmailNChatID(c)
	if err != nil {
		log.Error().Err(err).Msg("Error retreiving email and chatID")
		return err
	}
	// Check query parameters
	exchange := c.QueryParam("exchange")
	if exchange == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"message": "exchange not found",
			"errors": map[string]any{
				"exchange": "query parameter not provided",
			},
		})
	}
	exch, ok := contr.exchanges[exchange]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"message": "exchange not found",
			"errors": map[string]any{
				"exchange": fmt.Sprintf("%s not supported", exchange),
			},
		})
	}

	out, err := exch.GetCachedCurrencies()

	if err != nil {
		return err
	}
	// Sort alphabetically
	slices.Sort(out)

	log.Info().Fields(map[string]interface{}{
		"email":    email,
		"chat_id":  chatID,
		"exchange": exchange,
		"options":  out,
	}).Msg("Currencies requested")

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Currencies",
		"options": out,
	})
}

func (cont *Controller) GetExchanges(c echo.Context) error {
	email, chatID, err := utils.RetreiveEmailNChatID(c)
	if err != nil {
		log.Error().Err(err).Msg("Error retreiving email and chatID")
		return err
	}
	out := make([]string, 0)
	for k := range cont.exchanges {
		out = append(out, k)
	}

	log.Info().Fields(map[string]interface{}{
		"email":     email,
		"chat_id":   chatID,
		"exchanges": out,
	}).Msg("Exchanges requested")

	return c.JSON(http.StatusOK, map[string]any{
		"message":   "Exchanges",
		"exchanges": out,
	})
}

func (cont *Controller) TestFunc(c echo.Context) error {
	// Retreive email and chatID
	email, chatID, err := utils.RetreiveEmailNChatID(c)
	if err != nil {
		log.Error().Err(err).Msg("Error retreiving email and chatID")
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"message": "Test",
		"email":   email,
		"chat_id": chatID,
	})
}

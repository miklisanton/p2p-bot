package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"p2pbot/internal/db/models"
	"p2pbot/internal/requests"
	"p2pbot/internal/utils"
	"strconv"

	"github.com/labstack/echo/v4"
)

// GetTrackers gets all trackers for a user
// if page is not provided, it defaults to 1
// returns 10 trackers per page

func (contr *Controller) GetTrackers(c echo.Context) error {
    email := c.Get("email").(string)
    u, err := contr.userService.GetUserByEmail(email)
    if err == sql.ErrNoRows {
        return c.JSON(http.StatusNotFound, map[string]any{
            "message": "User not found",
            "errors": map[string]any{
                "user": "not found",
            },
        })
    }
    if err != nil {
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
        limit = "10"
    }
    l, err := strconv.Atoi(limit)
    if err != nil {
        return err
    }
    if l < 1 {
        l = 10
    }
    
    utils.Logger.Info().Fields(map[string]interface{}{
        "email": email,
        "page": p,
    }).Msg("Trackers requested")

    trackers, err := contr.trackerService.GetTrackersByUserId(u.ID)
    if err != nil {
        return err
    }

    if (p - 1) * l >= len(trackers) {
        return c.JSON(http.StatusNotFound, map[string]any{
            "message": "No trackers found",
            "errors": map[string]any{
                "trackers": "not found",
            },
        })
    }

    hasMore := p * l < len(trackers)

    return c.JSON(http.StatusOK, map[string]any{
        "message": fmt.Sprintf("Trackers for user %s", email),
        "trackers": trackers[(p - 1 ) * l : min(p * l, len(trackers))],  
        "hasMore": hasMore,
    })
}

func (contr *Controller) CreateTracker(c echo.Context) error {
    email := c.Get("email").(string)
    u, err := contr.userService.GetUserByEmail(email)
    if err == sql.ErrNoRows {
        return c.JSON(http.StatusNotFound, map[string]any{
            "message": "User not found",
            "errors": map[string]any{
                "user": "not found",
            },
        })
    }

    if err != nil {
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
        UserID: u.ID,
        Exchange: trackerReq.Exchange,
        Currency: trackerReq.Currency,
        Side: trackerReq.Side,
        Username: trackerReq.Username,
        Notify: *trackerReq.Notify,
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
                                        tracker.Username)
    if err != nil {
        return c.JSON(http.StatusNotFound, map[string]any{
            "message": "error getting ads",
            "errors": map[string]any{
                exchange.GetName(): err.Error(),
            },
        })
    }

    utils.Logger.LogInfo().Fields(map[string]interface{}{
        "email": email,
        exchange.GetName(): ads,
    }).Msg("Ads found")

    createdTrackers := make([]models.Tracker, 0)
    for _, adv := range ads {
        // Recieve payment methods from ad
        pmStrings := adv.GetPaymentMethods() 
        pms := make([]models.PaymentMethod, 0)
        for _, p := range pmStrings {
            pms = append(pms, models.PaymentMethod{
                Name: p,
            })
        }

        tracker.Payment = pms

        err = contr.trackerService.CreateTracker(tracker)
        createdTrackers = append(createdTrackers, *tracker)
        if err != nil {
            return err
        }
        tracker.ID = 0
    }

    return c.JSON(http.StatusCreated, map[string]any{
        "message": "Trackers created",
        "trackers": createdTrackers,
    })
}

func (contr *Controller) DeleteTracker(c echo.Context) error {
    email := c.Get("email").(string)
    u, err := contr.userService.GetUserByEmail(email)
    if err == sql.ErrNoRows {
        return c.JSON(http.StatusNotFound, map[string]any{
            "message": "User not found",
            "errors": map[string]any{
                "user": "not found",
            },
        })
    }
    if err != nil {
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
    email := c.Get("email").(string)
    u, err := contr.userService.GetUserByEmail(email)
    if err == sql.ErrNoRows {
        return c.JSON(http.StatusNotFound, map[string]any{
            "message": "User not found",
            "errors": map[string]any{
                "user": "not found",
            },
        })
    }
    if err != nil {
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
        "message": "Tracker updated",
        "trackers": tracker,
    })
}


// Options related endpoints
func (contr *Controller) GetPaymentMethods (c echo.Context) error {
    email := c.Get("email").(string)
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

    utils.Logger.LogInfo().Fields(map[string]interface{}{
        "email": email,
        "exchange": exchange,
        "currency": currency,
        "options": out,
    }).Msg("Payment methods requested")

    return c.JSON(http.StatusOK, map[string]any{
        "message": "Form options",
        "options": out,
    })
}

func (contr *Controller) GetCurrencies(c echo.Context) error {
    email := c.Get("email").(string)
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

    utils.Logger.LogInfo().Fields(map[string]interface{}{
        "email": email,
        "exchange": exchange,
        "options": out,
    }).Msg("Currencies requested")

    return c.JSON(http.StatusOK, map[string]any{
        "message": "Currencies",
        "options": out,
    })
}

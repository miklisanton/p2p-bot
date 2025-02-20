package handlers

import (
	"crypto/md5"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"p2pbot/internal/db/models"
	"p2pbot/internal/rediscl"
	"p2pbot/internal/requests"
	"p2pbot/internal/utils"

	"github.com/rs/zerolog/log"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/teris-io/shortid"
)

// Generatre order creates new invoice on cryptomus gateway.
// Unique order_id is stored in redis cache
// returns payment link
func (contr *Controller) CreateOrder(c echo.Context) error {
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
	// generate order id with shortid
	orderID, err := shortid.Generate()
	if err != nil {
		return err
	}
	log.Info().Fields(map[string]interface{}{
		"email":    u.Email,
		"order_id": orderID,
	}).Msg("Order ID generated")
	// save order id to redis
	ctx := rediscl.RDB.Ctx
	if err := rediscl.RDB.Client.Set(ctx, "order_id:"+orderID, u.ID, 2*time.Hour).Err(); err != nil {
		return err
	}
	// POST request to cryptomus gateway
	invoiceReq := requests.CryptomusRequest{
		OrderID:     orderID,
		Amount:      contr.SubPrice,
		Currency:    contr.SubCurrency,
		CallbackURL: os.Getenv("SUBSCRIPTION_CALLBACK_URL"),
		SuccessURL:  os.Getenv("SUBSCRIPTION_REDIRECT_URL"),
	}
	jsonReq, err := json.Marshal(invoiceReq)
	if err != nil {
		return err
	}
	log.Info().RawJSON("request", []byte(jsonReq)).Msg("json request data")

	// Create signature and add it to request headers
	base64Req := base64.StdEncoding.EncodeToString([]byte(jsonReq))

	sign := md5.Sum([]byte(base64Req + os.Getenv("GATEWAY_API_KEY")))

	log.Info().Str("sign", fmt.Sprintf("%x", sign)).Msg("Signature generated")

	req, err := http.NewRequest("POST", "https://api.cryptomus.com/v1/payment", strings.NewReader(string(jsonReq)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("sign", fmt.Sprintf("%x", sign))
	req.Header.Set("merchant", os.Getenv("GATEWAY_MERCHANT"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	log.Info().Str("status", resp.Status).Msg("Invoice request sent")

	if resp.StatusCode != http.StatusOK {
		// Read body and return it
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return c.JSONBlob(resp.StatusCode, body)
	}
	// Parse response
	respStruct := struct {
		State  int `json:"state"`
		Result struct {
			Url     string `json:"url"`
			OrderID string `json:"order_id"`
			Uuid    string `json:"uuid"`
		} `json:"result"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&respStruct); err != nil {
		return err
	}

	log.Info().Fields(map[string]interface{}{
		"order_id": respStruct.Result.OrderID,
		"uuid":     respStruct.Result.Uuid,
	}).Msg("Invoice created")

	return c.Redirect(http.StatusSeeOther, respStruct.Result.Url)
	//	return c.JSON(http.StatusOK, map[string]any{
	//		"message": "Invoice created",
	//		"invoice": map[string]any{
	//			"url":      respStruct.Result.Url,
	//			"order_id": respStruct.Result.OrderID,
	//			"uuid":     respStruct.Result.Uuid,
	//		},
	//	})
}

// ConfirmOrder is a webhook endpoint for cryptomus gateway
// It is called when payment is confirmed
// It checks if signature and order_id is valid and updates user subscription
func (contr *Controller) ConfirmOrder(c echo.Context) error {
	// Read request body
	data, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return err
	}
	var confirmReq map[string]interface{}
	if err := json.Unmarshal(data, &confirmReq); err != nil {
		return err
	}
	// Check ip
	if c.RealIP() != os.Getenv("GATEWAY_IP") {
		return fmt.Errorf("Invalid IP")
	}
	// Verify signature
	//if err := utils.VerifySignature(confirmReq); err != nil {
	//    return err
	//}
	//Check status
	if confirmReq["status"] != "paid" {
		log.Error().Fields(map[string]interface{}{
			"order_id": confirmReq["order_id"],
			"uuid":     confirmReq["uuid"],
			"status":   confirmReq["status"],
		}).Msg("Payment not confirmed")
		return fmt.Errorf("Payment not confirmed")
	}

	// Get user id from redis
	ctx := rediscl.RDB.Ctx
	userID, err := rediscl.RDB.Client.Get(ctx, "order_id:"+confirmReq["order_id"].(string)).Result()
	if err != nil {
		return err
	}
	// Update user subscription
	log.Info().Fields(map[string]interface{}{
		"user_id":  userID,
		"order_id": confirmReq["order_id"],
		"uuid":     confirmReq["uuid"],
		"status":   confirmReq["status"],
	}).Msg("Payment confirmed")
	// Update user subscription
	uid, err := strconv.Atoi(userID)
	if err != nil {
		return err
	}
	subscription, err := contr.subscriptionsService.GetByUserId(uid)
	if err != nil {
		return err
	}

	if subscription == nil {
		err := contr.subscriptionsService.Create(&models.Subscription{
			User_id: uid,
		})
		if err != nil {
			return err
		}
	} else {
		if subscription.ValidUntil.Before(time.Now()) {
			// Set subscription to expire in one month if it is expired
			subscription.ValidUntil = time.Now().AddDate(0, 1, 0)
		} else {
			// Add one month to subscription if it is not expired
			contr.subscriptionsService.AddMonth(subscription)
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message":  "Subscription updated",
		"order_id": confirmReq["order_id"],
	})
}

func (contr *Controller) GetSubscription(c echo.Context) error {
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

	// Get user's subscription
	subscription, err := contr.subscriptionsService.GetByUserId(u.ID)
	if err != nil {
		return err
	}
	// Mark subscription as expired if it is expired
	if subscription != nil {
		var expired bool
		if time.Now().Before(subscription.ValidUntil) {
			expired = false
		} else {
			expired = true
		}
		subscription.Expired = &expired
	}

	return c.JSON(http.StatusOK, map[string]any{
		"subscription": subscription,
	})
}

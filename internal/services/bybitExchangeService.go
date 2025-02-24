package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"p2pbot/internal/config"
	"p2pbot/internal/rediscl"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type BybitExchange struct {
	adsEndpoint string
	name        string
	maxRetries  int
	retryDelay  time.Duration
}

type BybitPayload struct {
	TokenID    string   `json:"tokenId"`
	CurrencyID string   `json:"currencyId"`
	Side       string   `json:"side"`
	Payment    []string `json:"payment"`
	Page       string   `json:"page"`
	Size       string   `json:"size"`
}

type BybitAdsResponse struct {
	RetCode int    `json:"ret_code"`
	RetMsg  string `json:"ret_msg"`
	Result  Result `json:"result"`
}

type Result struct {
	Items []Item `json:"items"`
}

type Item struct {
	Id                string   `json:"id"`
	NickName          string   `json:"nickName"`
	Price             string   `json:"price"`
	Quantity          string   `json:"quantity"`
	MinAmount         string   `json:"minAmount"`
	MaxAmount         string   `json:"maxAmount"`
	Payments          []string `json:"payments"`
	RecentOrderNum    int      `json:"recentOrderNum"`
	RecentExecuteRate int      `json:"recentExecuteRate"`
}

type BybitPayment struct {
	PaymentType string `json:"paymentType"`
	PaymentName string `json:"paymentName"`
}

func NewBybitExcahnge(config *config.Config) *BybitExchange {
	return &BybitExchange{
		adsEndpoint: "https://api2.bybit.com/fiat/otc/item/online",
		name:        "Bybit",
		maxRetries:  config.Exchange.MaxRetries,
		retryDelay:  time.Second * time.Duration(config.Exchange.RetryDelay),
	}
}

func (ex BybitExchange) GetName() string {
	return ex.name
}

func (ex BybitExchange) GetBestAdv(currency, side string, paymentMethods []string) (P2PItemI, error) {
	if side == "SELL" {
		side = "1"
	} else if side == "BUY" {
		side = "0"
	} else {
		return nil, fmt.Errorf("Invalid Side")
	}

	payload := BybitPayload{
		TokenID:    "USDT",
		CurrencyID: currency,
		Side:       side,
		Payment:    paymentMethods,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var body []byte

	for attempt := 1; attempt <= ex.maxRetries; attempt++ {
		resp, err := http.Post(ex.adsEndpoint, "application/json", bytes.NewBuffer(jsonPayload))
		if err == nil {
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				log.Printf("bad status: %s", resp.Status)
				log.Print("retrying...")
				time.Sleep(ex.retryDelay)
				continue
			}

			body, err = io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("could not read response body: %v", err)
				log.Print("retrying...")
				time.Sleep(ex.retryDelay)
				continue
			}
			break
		}
		// sleep before retry
		if attempt < ex.maxRetries {
			time.Sleep(ex.retryDelay)
			log.Printf("could not connect to bybit exchange: %v, retrying...", err)
		} else {
			return nil, fmt.Errorf("could not connect to bybit exchange: %v, after %d attempts", err, ex.maxRetries)
		}
	}

	bybitResponse := BybitAdsResponse{}
	if err := json.Unmarshal(body, &bybitResponse); err != nil {
		return nil, fmt.Errorf("could not parse response: %w", err)
	}

	if bybitResponse.RetCode != 0 {
		return nil, fmt.Errorf("bybit error: %s", bybitResponse.RetMsg)
	}

	if len(bybitResponse.Result.Items) == 0 {
		return nil, fmt.Errorf("no items found")
	}

	return bybitResponse.Result.Items[0], nil
}

func (i Item) GetPrice() float64 {
	price, _ := strconv.ParseFloat(i.Price, 64)
	return price
}

func (i Item) GetQuantity() (quantity, minAmount, maxAmount float64) {
	quantity, _ = strconv.ParseFloat(i.Quantity, 64)
	minAmount, _ = strconv.ParseFloat(i.MinAmount, 64)
	maxAmount, _ = strconv.ParseFloat(i.MaxAmount, 64)
	return
}

func (i Item) GetName() string {
	return i.NickName
}

func (ex BybitExchange) requestData(page int, currency, side string, pMethods []string) (*BybitAdsResponse, error) {
	if side == "SELL" {
		side = "1"
	} else if side == "BUY" {
		side = "0"
	} else {
		return nil, fmt.Errorf("invalid Side %s", side)
	}

	payload := BybitPayload{
		TokenID:    "USDT",
		CurrencyID: currency,
		Side:       side,
		Payment:    pMethods,
		Page:       fmt.Sprintf("%d", page),
		Size:       "1000",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("could not marshal json: %w", err)
	}

	var body []byte

	for attempt := 1; attempt <= ex.maxRetries; attempt++ {
		resp, err := http.Post(ex.adsEndpoint, "application/json", bytes.NewBuffer(jsonPayload))
		if err == nil {
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				log.Printf("bad status: %s", resp.Status)
				log.Print("retrying...")
				time.Sleep(ex.retryDelay)
				continue
			}

			body, err = io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("could not read response body: %v", err)
				log.Panic().Msg("retrying...")
				time.Sleep(ex.retryDelay)
				continue
			}
			break
		}
		// sleep before retry
		if attempt < ex.maxRetries {
			time.Sleep(ex.retryDelay)
			log.Printf("could not connect to bybit exchange: %v, retrying...", err)
		} else {
			return nil, fmt.Errorf("could not connect to bybit exchange: %v, after %d attempts", err, ex.maxRetries)
		}
	}

	bybitResponse := BybitAdsResponse{}
	if err := json.Unmarshal(body, &bybitResponse); err != nil {
		return nil, fmt.Errorf("could not parse response: %w", err)
	}

	if bybitResponse.RetCode != 0 {
		return nil, fmt.Errorf("bybit error: %s", bybitResponse.RetMsg)
	}
	return &bybitResponse, nil
}

func (ex BybitExchange) GetAdsByName(currency, side, username string, pMethods []string) ([]P2PItemI, error) {
	out := make([]P2PItemI, 0)
	i := 1
	for {
		resp, err := ex.requestData(i, currency, side, pMethods)
		if err != nil {
			return nil, fmt.Errorf("could not find advertisement with username %s", username)
		}

		if len(resp.Result.Items) == 0 {
			if len(out) == 0 {
				return nil, fmt.Errorf("could not find advertisement with username %s", username)
			} else {
				return out, nil
			}
		}

		for _, item := range resp.Result.Items {
			if item.GetName() == username {
				out = append(out, item)
			}
		}
		i++
	}
}

// GetAds returns all advertisements for a given currency and side
func (ex BybitExchange) GetAds(currency, side string) ([]P2PItemI, error) {
	out := make([]P2PItemI, 0)
	i := 1
	for {
		start := time.Now()
		response, err := ex.requestData(i, currency, side, []string{})
		log.Debug().
			Int("page", i).
			Str("currency", currency).
			Str("side", side).
			Int("len(ads)", len(response.Result.Items)).
			TimeDiff("request time(ms)", time.Now(), start).Msg("Fetching bybit advertisements")
		if err != nil {
			return nil, fmt.Errorf("error while getting advertisements %v", err)
		}
		if len(response.Result.Items) == 0 {
			// all pages parsed
			return out, nil
		}
		for _, item := range response.Result.Items {
			out = append(out, item)
		}
		i++
	}
}

func (ex BybitExchange) FetchAllPaymentList() (map[string][]PaymentMethod, error) {
	url := "https://api2.bybit.com/fiat/otc/configuration/queryAllPaymentList"

	resp, err := http.Post(url, "", nil)
	if err != nil {
		return nil, fmt.Errorf("could not get list of all currencies and payment methods: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %w", err)
	}

	jsonResp := struct {
		RetCode int `json:"ret_code"`
		Result  struct {
			CurrencyMapStr string         `json:"currencyPaymentIdMap"`
			Payments       []BybitPayment `json:"paymentConfigVo"`
		} `json:"result"`
	}{}

	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("could not parse response: %w", err)
	}

	if jsonResp.RetCode != 0 {
		return nil, fmt.Errorf("bybit error: %d", jsonResp.RetCode)
	}
	// Convert json string to map
	CurrencyMap := make(map[string][]int)
	if err := json.Unmarshal([]byte(jsonResp.Result.CurrencyMapStr), &CurrencyMap); err != nil {
		return nil, fmt.Errorf("could not parse currency map: %w", err)
	}
	// Convert slice of Payment Mehods to map
	idToName := make(map[string]string)
	for _, payment := range jsonResp.Result.Payments {
		idToName[payment.PaymentType] = payment.PaymentName
	}
	// Create new map which holds PaymentMethod struct
	currencyPayMethodMap := make(map[string][]PaymentMethod, 0)
	for key, value := range CurrencyMap {
		paymentNames := make([]PaymentMethod, 0)
		for _, id := range value {
			paymentNames = append(paymentNames, PaymentMethod{
				Name: idToName[strconv.Itoa(id)],
				Id:   strconv.Itoa(id),
			})
		}
		currencyPayMethodMap[key] = paymentNames
	}

	return currencyPayMethodMap, nil
}

func (ex BybitExchange) GetCachedPaymentMethods(curr string) ([]PaymentMethod, error) {
	ctx := rediscl.RDB.Ctx
	// Retrieve from cache
	currenciesJSON, err := rediscl.RDB.Client.JSONGet(ctx, "bybit:currencies",
		fmt.Sprintf("$.%s", curr)).Result()
	if currenciesJSON == "[]" {
		return nil, fmt.Errorf("no payment methods found")
	}
	if err == redis.Nil || currenciesJSON == "" {
		// Cache miss
		methods, err := ex.FetchAllPaymentList()
		if err != nil {
			return nil, err
		}
		jsonMethods, err := json.Marshal(methods)
		if err != nil {
			return nil, err
		}
		// Cache the result
		if err := rediscl.RDB.Client.JSONSet(ctx, "bybit:currencies", "$", string(jsonMethods)).Err(); err != nil {
			return nil, err
		}
		// Set expiration
		if err := rediscl.RDB.Client.Expire(ctx, "bybit:currencies", 12*time.Hour).Err(); err != nil {
			return nil, err
		}
		return methods[curr], nil
	}

	if err != nil && err != redis.Nil {
		return nil, err
	}

	currenciesJSON = currenciesJSON[1 : len(currenciesJSON)-1]
	paymentMethods := make([]PaymentMethod, 0)
	if err := json.Unmarshal([]byte(currenciesJSON), &paymentMethods); err != nil {
		return nil, err
	}

	return paymentMethods, nil
}

func (ex *BybitExchange) GetCachedCurrencies() ([]string, error) {
	ctx := rediscl.RDB.Ctx
	// Retrieve from cache
	currencies, err := rediscl.RDB.Client.SMembers(ctx, "bybit:currencies_list").Result()
	if err != nil {
		return nil, err
	}
	if err == redis.Nil || len(currencies) == 0 {
		// Cache miss
		paymentsMethodsMap, err := ex.FetchAllPaymentList()
		if err != nil {
			return nil, err
		}
		// Get all keys from the map
		currencies := make([]string, 0)
		for key := range paymentsMethodsMap {
			currencies = append(currencies, key)
		}
		// Cache currency list
		for _, item := range currencies {
			err := rediscl.RDB.Client.SAdd(ctx, "bybit:currencies_list", item).Err()
			if err != nil {
				return nil, err
			}
		}
		if err := rediscl.RDB.Client.Expire(ctx, "bybit:currencies_list", 24*time.Hour).Err(); err != nil {
			return nil, err
		}
		// Cache map
		jsonMethods, err := json.Marshal(paymentsMethodsMap)
		if err != nil {
			return nil, err
		}
		if err := rediscl.RDB.Client.JSONSet(ctx, "bybit:currencies", "$", string(jsonMethods)).Err(); err != nil {
			return nil, err
		}
		if err := rediscl.RDB.Client.Expire(ctx, "bybit:currencies", 24*time.Hour).Err(); err != nil {
			return nil, err
		}
		return currencies, nil
	}

	if err != nil && err != redis.Nil {
		return nil, err
	}

	return currencies, nil
}

func (i Item) GetPaymentMethods() []string {
	return i.Payments
}

func (i Item) GetId() string {
	return i.Id
}

package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"p2pbot/internal/config"
	"p2pbot/internal/rediscl"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type BinanceExchange struct {
	adsEndpoint string
	name        string
	maxRetries  int
	retryDelay  time.Duration
}

type BinancePayload struct {
	Fiat                      string   `json:"fiat"`
	Page                      int      `json:"page"`
	Rows                      int      `json:"rows"`
	TradeType                 string   `json:"tradeType"`
	Asset                     string   `json:"asset"`
	Countries                 []string `json:"countries"`
	ProMerchantAds            bool     `json:"proMerchantAds"`
	ShieldMerchantAds         bool     `json:"shieldMerchantAds"`
	FilterType                string   `json:"filterType"`
	Periods                   []string `json:"periods"`
	AdditionalKycVerifyFilter int      `json:"additionalKycVerifyFilter"`
	PublisherType             *string  `json:"publisherType"`
	PayTypes                  []string `json:"payTypes"`
	Classifies                []string `json:"classifies"`
}

type BinanceAdsResponse struct {
	Code    string     `json:"code"`
	Data    []DataItem `json:"data"`
	Total   int        `json:"total"`
	Success bool       `json:"success"`
}

// Implements P2PItem interface
type DataItem struct {
	Adv        Adv        `json:"adv"`
	Advertiser Advertiser `json:"advertiser"`
}

type Adv struct {
	Price                string        `json:"price"`
	TradableQuantity     string        `json:"tradableQuantity"`
	MaxSingleTransAmount string        `json:"maxSingleTransAmount"`
	MinSingleTransAmount string        `json:"minSingleTransAmount"`
	TradeMethods         []TradeMethod `json:"tradeMethods"`
	IsTradable           bool          `json:"isTradable"`
}

type TradeMethod struct {
	Identifier string `json:"identifier"`
}

type Advertiser struct {
	NickName        string  `json:"nickName"`
	MonthOrderCount int     `json:"monthOrderCount"`
	MonthFinishRate float64 `json:"monthFinishRate"`
	PositiveRate    float64 `json:"positiveRate"`
}

func NewBinanceExchange(config *config.Config) *BinanceExchange {
	return &BinanceExchange{
		adsEndpoint: "https://p2p.binance.com/bapi/c2c/v2/friendly/c2c/adv/search",
		name:        "Binance",
		maxRetries:  config.Exchange.MaxRetries,
		retryDelay:  time.Second * time.Duration(config.Exchange.RetryDelay),
	}
}

func (ex BinanceExchange) GetName() string {
	return ex.name
}

func (ex BinanceExchange) GetBestAdv(currency, side string, paymentMethods []string) (P2PItemI, error) {
	if side == "BUY" {
		side = "SELL"
	} else if side == "SELL" {
		side = "BUY"
	} else {
		return nil, fmt.Errorf("invalid side")
	}

	payload := BinancePayload{
		Fiat:                      currency,
		Page:                      1,
		Rows:                      20,
		TradeType:                 side,
		Asset:                     "USDT",
		Countries:                 []string{},
		ProMerchantAds:            false,
		ShieldMerchantAds:         false,
		FilterType:                "all",
		Periods:                   []string{},
		AdditionalKycVerifyFilter: 0,
		PublisherType:             nil,
		PayTypes:                  paymentMethods,
		Classifies:                []string{"mass", "profession"},
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
				log.Println("retrying...")
				time.Sleep(ex.retryDelay)
				continue
			}

			body, err = io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("could not read response body: %v", err)
				log.Println("retrying...")
				time.Sleep(ex.retryDelay)
				continue
			}
			break
		}
		// sleep before retry
		if attempt < ex.maxRetries {
			time.Sleep(ex.retryDelay)
			log.Printf("could not connect to binance exchange: %v, retrying...", err)
		} else {
			return nil, fmt.Errorf("could not connect to binance exchange: %v, after %d attempts", err, ex.maxRetries)
		}
	}

	binanceResponse := BinanceAdsResponse{}
	if err := json.Unmarshal(body, &binanceResponse); err != nil {
		return nil, fmt.Errorf("could not parse response: %w", err)
	}

	if !binanceResponse.Success {
		return nil, fmt.Errorf("binance error: %s", binanceResponse.Code)
	}

	if len(binanceResponse.Data) == 0 {
		log.Println(payload)

		return nil, fmt.Errorf("binance response has no data")
	}
	return binanceResponse.Data[0], nil
}

func (i DataItem) GetName() string {
	return i.Advertiser.NickName
}

func (i DataItem) GetPrice() float64 {
	price, _ := strconv.ParseFloat(i.Adv.Price, 64)
	return price
}

func (i DataItem) GetQuantity() (quantity, minAmount, maxAmount float64) {
	quantity, _ = strconv.ParseFloat(i.Adv.TradableQuantity, 64)
	minAmount, _ = strconv.ParseFloat(i.Adv.MinSingleTransAmount, 64)
	maxAmount, _ = strconv.ParseFloat(i.Adv.MaxSingleTransAmount, 64)
	return
}

func (ex BinanceExchange) RequestData(page int, currency, side string, pMethods []string) (*BinanceAdsResponse, error) {
	if side == "BUY" {
		side = "SELL"
	} else if side == "SELL" {
		side = "BUY"
	} else {
		return nil, fmt.Errorf("invalid side")
	}

	payload := BinancePayload{
		Fiat:                      currency,
		Page:                      page,
		Rows:                      20,
		TradeType:                 side,
		Asset:                     "USDT",
		Countries:                 []string{},
		ProMerchantAds:            false,
		ShieldMerchantAds:         false,
		FilterType:                "all",
		Periods:                   []string{},
		AdditionalKycVerifyFilter: 0,
		PublisherType:             nil,
		PayTypes:                  pMethods,
		Classifies:                []string{"mass", "profession"},
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
				log.Println("retrying...")
				time.Sleep(ex.retryDelay)
				continue
			}

			body, err = io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("could not read response body: %v", err)
				log.Println("retrying...")
				time.Sleep(ex.retryDelay)
				continue
			}
			break
		}
		// sleep before retry
		if attempt < ex.maxRetries {
			time.Sleep(ex.retryDelay)
			log.Printf("could not connect to binance exchange: %v, retrying...", err)
		} else {
			return nil, fmt.Errorf("could not connect to binance exchange: %v, after %d attempts", err, ex.maxRetries)
		}
	}

	binanceResponse := BinanceAdsResponse{}
	if err := json.Unmarshal(body, &binanceResponse); err != nil {
		return nil, fmt.Errorf("could not parse response: %w", err)
	}

	if !binanceResponse.Success {
		return nil, fmt.Errorf("binance error: %s", binanceResponse.Code)
	}

	return &binanceResponse, nil
}

func (ex BinanceExchange) GetAdsByName(currency, side, username string, pMethods []string) ([]P2PItemI, error) {
	out := make([]P2PItemI, 0)
	i := 1
	for {
		response, err := ex.RequestData(i, currency, side, pMethods)
		if err != nil {

			return nil, fmt.Errorf("could not find advertisement with username %s", username)
		}
		// All pages parsed, adv not found
		if len(response.Data) == 0 {
			if len(out) == 0 {
				return nil, fmt.Errorf("could not find advertisement with username %s", username)
			} else {
				return out, nil
			}
		}

		for _, item := range response.Data {
			if item.GetName() == username {
				out = append(out, item)
			}
		}
		i++
	}
}

func (ex BinanceExchange) GetAds(currency, side string) ([]P2PItemI, error) {
	out := make([]P2PItemI, 0)
	i := 1
	for {
		response, err := ex.RequestData(i, currency, side, []string{})
		if err != nil {
			return nil, fmt.Errorf("error while getting advertisements %v", err)
		}
		if len(response.Data) == 0 {
			// all pages parsed
			return out, nil
		}
		for _, item := range response.Data {
			out = append(out, item)
		}
		i++
	}
}

func (ex BinanceExchange) FetchCurrencies() ([]string, error) {
	url := "https://p2p.binance.com/bapi/c2c/v1/friendly/c2c/trade-rule/fiat-list"
	resp, err := http.Post(url, "", nil)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var binanceFiatListResponse struct {
		Code string `json:"code"`
		Data []struct {
			Currency string `json:"currencyCode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &binanceFiatListResponse); err != nil {
		return nil, fmt.Errorf("could not parse response: %w", err)
	}

	if binanceFiatListResponse.Code != "000000" {
		return nil, fmt.Errorf("binance error: %s", binanceFiatListResponse.Code)
	}

	out := make([]string, 0)
	for _, currency := range binanceFiatListResponse.Data {
		out = append(out, currency.Currency)
	}

	return out, nil
}

func (ex BinanceExchange) FetchPaymentMethods(currencies []string) (map[string][]PaymentMethod, error) {
	url := "https://p2p.binance.com/bapi/c2c/v2/public/c2c/adv/filter-conditions"

	out := make(map[string][]PaymentMethod)
	for _, currency := range currencies {
		reader := strings.NewReader(fmt.Sprintf(`{"fiat":"%s"}`, currency))
		resp, err := http.Post(url, "application/json", reader)
		if err != nil {
			return nil, err
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var binancePaymentMethodsResponse struct {
			Code string `json:"code"`
			Data struct {
				PayTypes []struct {
					Identifier string `json:"identifier"`
					Name       string `json:"tradeMethodShortName"`
				} `json:"tradeMethods"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &binancePaymentMethodsResponse); err != nil {
			return nil, fmt.Errorf("could not parse response: %w", err)
		}

		if binancePaymentMethodsResponse.Code != "000000" {
			return nil, fmt.Errorf("binance error: %s", binancePaymentMethodsResponse.Code)
		}

		methodsList := make([]PaymentMethod, 0)
		for _, method := range binancePaymentMethodsResponse.Data.PayTypes {
			methodsList = append(methodsList, PaymentMethod{
				Id:   method.Identifier,
				Name: method.Name,
			})
		}
		out[currency] = methodsList
	}
	return out, nil
}

func (ex BinanceExchange) GetCachedPaymentMethods(curr string) ([]PaymentMethod, error) {
	ctx := rediscl.RDB.Ctx
	// Retrieve from cache
	var err error
	var currenciesJSON string

	if curr == "" {
		// Retrieve all
		currenciesJSON, err = rediscl.RDB.Client.JSONGet(ctx, "binance:currencies", "$").Result()
	} else {
		// Retrieve specific currency
		currenciesJSON, err = rediscl.RDB.Client.JSONGet(ctx, "binance:currencies",
			fmt.Sprintf("$.%s", curr)).Result()
	}

	if currenciesJSON == "[]" {
		return nil, fmt.Errorf("currency not found")
	}

	if err == redis.Nil || currenciesJSON == "" {
		// Cache miss
		currencies, err := ex.FetchCurrencies()
		if err != nil {
			return nil, err
		}
		methods, err := ex.FetchPaymentMethods(currencies)
		if err != nil {
			return nil, err
		}
		jsonMethods, err := json.Marshal(methods)
		if err != nil {
			return nil, err
		}
		// Cache the result
		if err := rediscl.RDB.Client.JSONSet(ctx, "binance:currencies", "$", string(jsonMethods)).Err(); err != nil {
			return nil, err
		}
		// Set expiration
		if err := rediscl.RDB.Client.Expire(ctx, "binance:currencies", 24*time.Hour).Err(); err != nil {
			return nil, err
		}
		return methods[curr], nil
	}

	if err != nil && err != redis.Nil {
		return nil, err
	}

	// trim [] from string
	currenciesJSON = currenciesJSON[1 : len(currenciesJSON)-1]
	paymentMethods := make([]PaymentMethod, 0)
	if err := json.Unmarshal([]byte(currenciesJSON), &paymentMethods); err != nil {
		return nil, err
	}

	return paymentMethods, nil
}

func (ex *BinanceExchange) GetCachedCurrencies() ([]string, error) {
	ctx := rediscl.RDB.Ctx
	// Retrieve from cache
	currencies, err := rediscl.RDB.Client.SMembers(ctx, "binance:currencies_list").Result()
	if err != nil {
		return nil, err
	}
	if err == redis.Nil || len(currencies) == 0 {
		// Cache miss
		currencies, err := ex.FetchCurrencies()
		if err != nil {
			return nil, err
		}
		// Cache the result
		for _, item := range currencies {
			err := rediscl.RDB.Client.SAdd(ctx, "binance:currencies_list", item).Err()
			if err != nil {
				return nil, err
			}
		}
		// Set expiration
		if err := rediscl.RDB.Client.Expire(ctx, "binance:currencies_list", 24*time.Hour).Err(); err != nil {
			return nil, err
		}
		return currencies, nil
	}

	if err != nil && err != redis.Nil {
		return nil, err
	}

	return currencies, nil
}

func (i DataItem) GetPaymentMethods() []string {
	out := make([]string, 0)
	for _, method := range i.Adv.TradeMethods {
		out = append(out, method.Identifier)
	}
	return out
}

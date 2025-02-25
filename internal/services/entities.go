package services

import (
	"encoding/json"
	"fmt"
)

type Notification struct {
	ChatID   int64    `json:"chat_id"`
	Data     P2PItemI `json:"top_order"`
	Exchange string   `json:"exchange"`
	Side     string   `json:"side"`
	Currency string   `json:"currency"`
}

type StringNotification struct {
	ChatID int64  `json:"chat_id"`
	Msg    string `json:"message"`
}

func (n *Notification) UnmarshalJSON(data []byte) error {
	// Define a temporary structure for the concrete type
	type Alias Notification
	aux := &struct {
		Data json.RawMessage `json:"top_order"`
		*Alias
	}{
		Alias: (*Alias)(n),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.Exchange == "binance" {
		var item DataItem
		if err := json.Unmarshal(aux.Data, &item); err != nil {
			return err
		}
		n.Data = item
	} else if aux.Exchange == "bybit" {
		var item Item
		if err := json.Unmarshal(aux.Data, &item); err != nil {
			return err
		}
		n.Data = item
	} else {
		return fmt.Errorf("no unmarshal logic for %s", aux.Exchange)
	}

	return nil
}

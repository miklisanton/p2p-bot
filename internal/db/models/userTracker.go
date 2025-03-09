package models

type UserTracker struct {
	ID            int64            `db:"tracker_id" json:"id"`
	AdvID         string           `db:"adv_id" json:"adv_id"`
	Exchange      string           `db:"exchange" json:"exchange"`
	Currency      string           `db:"currency" json:"currency"`
	Side          string           `db:"side" json:"side"`
	Notify        bool             `db:"notify" json:"notify"`
	Payment       []*PaymentMethod `db:"-" json:"payment_methods"`
	Price         float64          `db:"price" json:"price"`
	UserID        int              `db:"user_id" json:"-"`
	ChatID        *int64           `db:"chat_id" json:"tg_chat_id"`
	WaitingUpdate bool             `db:"waiting_update" json:"waiting_update"`
	IsAggregated  bool             `db:"is_aggregated" json:"is_aggregated"`
	Username      string           `db:"username" json:"username"`
}

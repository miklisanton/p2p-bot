package models

type Tracker struct {
	ID            int64            `db:"id"`
	AdvID         int64            `db:"adv_id"`
	UserID        int              `db:"user_id"`
	Exchange      string           `db:"exchange"`
	Currency      string           `db:"currency"`
	Side          string           `db:"side"`
	Username      string           `db:"username"`
	Notify        bool             `db:"notify"`
	Price         float64          `db:"price"`
	WaitingUpdate bool             `db:"waiting_update"`
	IsAggregated  bool             `db:"is_aggregated"`
	Payment       []*PaymentMethod `db:"-"`
}

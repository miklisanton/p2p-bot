package repository

import (
	"fmt"
	"p2pbot/internal/db/models"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type TrackerRepository struct {
	db *sqlx.DB
}

func NewTrackerRepository(db *sqlx.DB) *TrackerRepository {
	return &TrackerRepository{db}
}

func (repo *TrackerRepository) UpdateWaitingUpdate(id int64, flag bool) error {
	query := `UPDATE trackers SET waiting_update = $1 WHERE id = $2`
	_, err := repo.db.Exec(query, flag, id)
	return err
}

func (repo *TrackerRepository) Save(tracker *models.Tracker) error {
	if tracker == nil {
		return fmt.Errorf("tracker is nil")
	}

	tx, err := repo.db.Begin()
	if err != nil {
		return err
	}

	if tracker.ID == 0 {
		query := `INSERT INTO trackers (user_id, adv_id, exchange, currency, side, username, notify, price, is_aggregated)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
            RETURNING id`
		err := tx.QueryRow(query, tracker.UserID, tracker.AdvID, tracker.Exchange,
			tracker.Currency, tracker.Side,
			tracker.Username, tracker.Notify, tracker.Price, tracker.IsAggregated).Scan(&tracker.ID)

		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error creating new tracker : %v", err)
		}
	} else {
		query := `UPDATE trackers SET exchange = $1, currency = $2, side = $3, username = $4, notify = $5, price = $6,
            is_aggregated = $7, waiting_update = $8 WHERE id = $9`
		_, err = tx.Exec(query, tracker.Exchange, tracker.Currency,
			tracker.Side, tracker.Username, tracker.Notify,
			tracker.Price, tracker.IsAggregated, tracker.WaitingUpdate, tracker.ID)
		if err != nil {
			tx.Rollback()
			return err
		}

		// Remove old payment methods
		_, err = tx.Exec(`DELETE FROM methods WHERE tracker_id = $1`, tracker.ID)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// Insert new payment methods
	query := `INSERT INTO methods (tracker_id, payment_method, payment_name)
                VALUES ($1, $2, $3)
                ON CONFLICT DO NOTHING`
	for _, method := range tracker.Payment {
		_, err = tx.Exec(query, tracker.ID, method.Id, method.Name)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (repo *TrackerRepository) GetMethodsForTracker(trackerId int64) ([]*models.PaymentMethod, error) {
	var out []*models.PaymentMethod

	err := repo.db.Select(&out, "SELECT payment_method, payment_name, outbidded FROM methods WHERE tracker_id = $1", trackerId)
	if err != nil {
		return nil, fmt.Errorf("error getting payment methods: %s", err)
	}
	return out, nil
}

func (repo *TrackerRepository) GetAllTrackers() ([]*models.UserTracker, error) {
	var trackers []*models.UserTracker
	query := `SELECT t.id as tracker_id, t.exchange, t.currency, t.side, t.username,
        t.notify, t.waiting_update, t.is_aggregated, t.price, t.adv_id, u.id, u.chat_id as user_id
        FROM trackers t JOIN public.users u on t.user_id = u.id`
	err := repo.db.Select(&trackers, query)
	if err != nil {
		return nil, err
	}
	//Insert payment methods into trackers
	for _, tracker := range trackers {
		tracker.Payment, err = repo.GetMethodsForTracker(tracker.ID)
		if err != nil {
			return nil, err
		}
	}
	return trackers, nil
}

func (repo *TrackerRepository) GetTrackersByUserId(id int) ([]*models.UserTracker, error) {
	var trackers []*models.UserTracker
	query := `SELECT t.id as tracker_id, t.exchange, t.currency, t.side, t.username,
        t.notify, t.waiting_update, t.is_aggregated, t.price, t.adv_id, u.id as user_id, u.chat_id
        FROM trackers t JOIN public.users u on t.user_id = u.id WHERE u.id = $1`
	err := repo.db.Select(&trackers, query, id)
	if err != nil {
		return nil, err
	}
	//Insert payment methods into trackers
	for _, tracker := range trackers {
		tracker.Payment, err = repo.GetMethodsForTracker(tracker.ID)
		if err != nil {
			return nil, err
		}
	}
	return trackers, nil
}

func (repo *TrackerRepository) GetTrackerById(id int) (*models.Tracker, error) {
	var trackers []*models.Tracker
	query := `SELECT * FROM trackers WHERE id = $1`
	err := repo.db.Select(&trackers, query, id)
	if err != nil {
		return nil, err
	}
	var tracker *models.Tracker

	if len(trackers) == 1 {
		tracker = trackers[0]
	} else {
		return nil, fmt.Errorf("tracker not found")
	}
	// Get payment methods
	tracker.Payment, err = repo.GetMethodsForTracker(tracker.ID)
	if err != nil {
		return nil, err
	}

	return tracker, nil
}

func (repo *TrackerRepository) UpdatePaymentMethodOutbided(trackerId int64, name string, outbidded bool) error {
	query := `UPDATE methods SET outbidded = $1 WHERE tracker_id = $2 AND payment_method = $3`
	_, err := repo.db.Exec(query, outbidded, trackerId, name)
	if err != nil {
		return err
	}
	return nil
}

func (repo *TrackerRepository) DeleteTracker(id int) (int64, error) {
	query := `DELETE FROM trackers WHERE id = $1`
	result, err := repo.db.Exec(query, id)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// methods specifc to observer
func (repo *TrackerRepository) GetIdsByCurrency(exchange string) (map[string][]int, error) {
	var Result []struct {
		Key string `db:"key"`
		Ids []byte `db:"ids"`
	}

	err := repo.db.Select(&Result, `SELECT CONCAT(currency, side) as key , array_agg(id::int) as ids
            FROM trackers WHERE exchange = $1 GROUP BY currency, side`, exchange)
	if err != nil {
		return nil, err
	}
	// Psql returns array of bytes, we need to convert it to int
	out := make(map[string][]int)
	for _, r := range Result {
		var ids []int64
		if err := pq.Array(&ids).Scan(r.Ids); err != nil {
			return nil, err
		}

		out[r.Key] = make([]int, len(ids))
		for i, id := range ids {
			out[r.Key][i] = int(id)
		}
	}
	return out, nil
}

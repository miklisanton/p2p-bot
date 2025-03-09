-- +goose Up
-- +goose StatementBegin
CREATE TABLE trackers (
    id SERIAL PRIMARY KEY,
    adv_id varchar NOT NULL,
    user_id INT NOT NULL,
    exchange varchar NOT NULL,
    currency varchar(3) NOT NULL,
    username varchar NOT NULL,
    side varchar(4) NOT NULL,
    price decimal NOT NULL,
    notify boolean DEFAULT false,
    waiting_update boolean DEFAULT false,
    is_aggregated boolean DEFAULT false,
    CONSTRAINT fk_user
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE trackers;
-- +goose StatementEnd

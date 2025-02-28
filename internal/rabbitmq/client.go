package rabbitmq

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"p2pbot/internal/config"
)

type RabbitMQ struct {
	conn         *amqp.Connection
	Ch           *amqp.Channel
	ExchangeName string
}

func NewRabbitMQ(cfg *config.Config) (*RabbitMQ, error) {
	conn, err := amqp.Dial(cfg.RabbitMQ.URL)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	return &RabbitMQ{conn, ch, ""}, nil
}

func (r *RabbitMQ) DeclareExchange(name string) error {
	r.ExchangeName = name
	return r.Ch.ExchangeDeclare(
		name,
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
}

func (r *RabbitMQ) QueueBindNDeclare() (string, error) {
	q, err := r.Ch.QueueDeclare(
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return "", err
	}

	err = r.Ch.QueueBind(
		q.Name,
		"",
		r.ExchangeName,
		false,
		nil,
	)
	if err != nil {
		return "", err
	}

	return q.Name, nil
}

func (r *RabbitMQ) Publish(body []byte) error {
	return r.Ch.Publish(
		r.ExchangeName,
		"",
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
}

func (r *RabbitMQ) StartConsuming(qName string, handlerFunc func(amqp.Delivery)) error {
	msgs, err := r.Ch.Consume(
		qName,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		for msg := range msgs {
			handlerFunc(msg)
		}
	}()
	return nil
}

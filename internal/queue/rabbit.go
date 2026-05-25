package queue

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"streaming-transcode/internal/config"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Delivery struct {
	Body    []byte
	Attempt int
	Ack     func() error
	Nack    func(requeue bool) error
}

type amqpConnection interface {
	Channel() (amqpChannel, error)
	Close() error
}

type amqpChannel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueDeclarePassive(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	Qos(prefetchCount, prefetchSize int, global bool) error
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	Close() error
}

type RabbitConsumer struct {
	conn    amqpConnection
	channel amqpChannel
	cfg     config.QueueConfig
	logger  *log.Logger
}

var amqpDial = func(url string) (amqpConnection, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	return &rabbitConnection{conn: conn}, nil
}

var sleep = time.Sleep

var rabbitDialAttempts = 12

type rabbitConnection struct {
	conn *amqp.Connection
}

func (c *rabbitConnection) Channel() (amqpChannel, error) {
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, err
	}
	return &rabbitChannel{channel: ch}, nil
}

func (c *rabbitConnection) Close() error {
	return c.conn.Close()
}

type rabbitChannel struct {
	channel *amqp.Channel
}

func (c *rabbitChannel) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	return c.channel.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, args)
}

func (c *rabbitChannel) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	return c.channel.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
}

func (c *rabbitChannel) QueueDeclarePassive(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	return c.channel.QueueDeclarePassive(name, durable, autoDelete, exclusive, noWait, args)
}

func (c *rabbitChannel) QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error {
	return c.channel.QueueBind(name, key, exchange, noWait, args)
}

func (c *rabbitChannel) Qos(prefetchCount, prefetchSize int, global bool) error {
	return c.channel.Qos(prefetchCount, prefetchSize, global)
}

func (c *rabbitChannel) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	return c.channel.Consume(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
}

func (c *rabbitChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	return c.channel.PublishWithContext(ctx, exchange, key, mandatory, immediate, msg)
}

func (c *rabbitChannel) Close() error {
	return c.channel.Close()
}

func NewRabbitConsumer(url string, cfg config.QueueConfig, logger *log.Logger) (*RabbitConsumer, error) {
	conn, err := dialRabbitMQ(url)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}
	if err := ch.ExchangeDeclare(cfg.Exchange, "topic", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("declare exchange: %w", err)
	}
	if _, err := ch.QueueDeclare(cfg.DeadName, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("declare dead queue: %w", err)
	}
	retryArgs := amqp.Table{
		"x-dead-letter-exchange":    cfg.Exchange,
		"x-dead-letter-routing-key": cfg.BindingKey,
		"x-message-ttl":             int32(cfg.RetryDelaySeconds * 1000),
	}
	if _, err := ch.QueueDeclare(cfg.RetryName, true, false, false, false, retryArgs); err != nil {
		passiveQueue, passiveErr := ch.QueueDeclarePassive(cfg.RetryName, true, false, false, false, nil)
		if passiveErr != nil {
			_ = ch.Close()
			_ = conn.Close()
			return nil, fmt.Errorf("declare retry queue: %w", err)
		}
		logger.Printf("retry queue already exists with current broker arguments, reusing queue=%s current=%s", cfg.RetryName, passiveQueue.Name)
	}
	if _, err := ch.QueueDeclare(cfg.Name, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("declare queue: %w", err)
	}
	if err := ch.QueueBind(cfg.Name, cfg.BindingKey, cfg.Exchange, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("bind queue: %w", err)
	}
	if err := ch.Qos(1, 0, false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("set qos: %w", err)
	}
	return &RabbitConsumer{conn: conn, channel: ch, cfg: cfg, logger: logger}, nil
}

func dialRabbitMQ(url string) (amqpConnection, error) {
	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= rabbitDialAttempts; attempt++ {
		conn, err := amqpDial(url)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if attempt < rabbitDialAttempts {
			sleep(backoff)
			if backoff < 5*time.Second {
				backoff *= 2
				if backoff > 5*time.Second {
					backoff = 5 * time.Second
				}
			}
		}
	}
	return nil, lastErr
}

func (c *RabbitConsumer) Run(ctx context.Context, handler func(context.Context, Delivery) error) error {
	deliveries, err := c.channel.Consume(c.cfg.Name, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume queue: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}
			attempt := deliveryAttempt(msg.Headers)
			delivery := Delivery{
				Body:    msg.Body,
				Attempt: attempt,
				Ack: func() error {
					return msg.Ack(false)
				},
				Nack: func(requeue bool) error {
					return msg.Nack(false, requeue)
				},
			}
			if err := handler(ctx, delivery); err != nil {
				c.logger.Printf("job failed: %v", err)
				if shouldDeadLetter(err, attempt, c.cfg.MaxAttempts) {
					if publishErr := c.publishToQueue(ctx, c.cfg.DeadName, msg.Body, attempt, err); publishErr != nil {
						c.logger.Printf("dead-letter publish failed: %v", publishErr)
						_ = delivery.Nack(true)
						continue
					}
				} else if publishErr := c.publishToQueue(ctx, c.cfg.RetryName, msg.Body, attempt+1, err); publishErr != nil {
					c.logger.Printf("retry publish failed: %v", publishErr)
					_ = delivery.Nack(true)
					continue
				}
				_ = delivery.Ack()
				continue
			}
			_ = delivery.Ack()
		}
	}
}

func (c *RabbitConsumer) publishToQueue(ctx context.Context, queueName string, body []byte, attempt int, cause error) error {
	publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.channel.PublishWithContext(publishCtx, "", queueName, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
		Headers: amqp.Table{
			"x-transcode-attempt": attempt,
			"x-transcode-error":   cause.Error(),
		},
	})
}

func (c *RabbitConsumer) Close() {
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

type terminalError interface {
	Terminal() bool
}

func shouldDeadLetter(err error, attempt, maxAttempts int) bool {
	var terminal terminalError
	if errors.As(err, &terminal) && terminal.Terminal() {
		return true
	}
	return attempt >= maxAttempts
}

func deliveryAttempt(headers amqp.Table) int {
	if headers == nil {
		return 1
	}
	switch value := headers["x-transcode-attempt"].(type) {
	case int:
		if value > 0 {
			return value
		}
	case int32:
		if value > 0 {
			return int(value)
		}
	case int64:
		if value > 0 {
			return int(value)
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	}
	return 1
}

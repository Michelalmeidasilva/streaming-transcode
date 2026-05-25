package queue

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"streaming-transcode/internal/config"

	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeConnection struct {
	channel amqpChannel
	err     error
	closed  bool
}

func (c *fakeConnection) Channel() (amqpChannel, error) {
	return c.channel, c.err
}

func (c *fakeConnection) Close() error {
	c.closed = true
	return nil
}

type fakeChannel struct {
	exchangeErr      error
	queueErr         error
	queueErrByName   map[string]error
	passiveErr       error
	passiveErrByName map[string]error
	bindErr          error
	qosErr           error
	consumeErr       error
	deliveries       chan amqp.Delivery
	closed           bool
	published        []amqp.Publishing
	publishKeys      []string
}

func (c *fakeChannel) ExchangeDeclare(string, string, bool, bool, bool, bool, amqp.Table) error {
	return c.exchangeErr
}

func (c *fakeChannel) QueueDeclare(name string, _, _, _, _ bool, _ amqp.Table) (amqp.Queue, error) {
	if err, ok := c.queueErrByName[name]; ok {
		return amqp.Queue{Name: name}, err
	}
	return amqp.Queue{Name: name}, c.queueErr
}

func (c *fakeChannel) QueueDeclarePassive(name string, _, _, _, _ bool, _ amqp.Table) (amqp.Queue, error) {
	if err, ok := c.passiveErrByName[name]; ok {
		return amqp.Queue{Name: name}, err
	}
	return amqp.Queue{Name: name}, c.passiveErr
}

func (c *fakeChannel) QueueBind(string, string, string, bool, amqp.Table) error {
	return c.bindErr
}

func (c *fakeChannel) Qos(int, int, bool) error {
	return c.qosErr
}

func (c *fakeChannel) Consume(string, string, bool, bool, bool, bool, amqp.Table) (<-chan amqp.Delivery, error) {
	return c.deliveries, c.consumeErr
}

func (c *fakeChannel) PublishWithContext(_ context.Context, _ string, key string, _, _ bool, msg amqp.Publishing) error {
	c.publishKeys = append(c.publishKeys, key)
	c.published = append(c.published, msg)
	return nil
}

func (c *fakeChannel) Close() error {
	c.closed = true
	return nil
}

func TestNewRabbitConsumerAndClose(t *testing.T) {
	originalDial := amqpDial
	originalAttempts := rabbitDialAttempts
	originalSleep := sleep
	t.Cleanup(func() { amqpDial = originalDial })
	t.Cleanup(func() { rabbitDialAttempts = originalAttempts })
	t.Cleanup(func() { sleep = originalSleep })

	channel := &fakeChannel{}
	conn := &fakeConnection{channel: channel}
	amqpDial = func(string) (amqpConnection, error) { return conn, nil }
	rabbitDialAttempts = 1
	sleep = func(time.Duration) {}

	consumer, err := NewRabbitConsumer("amqp://guest:guest@localhost:5672/", config.QueueConfig{
		Exchange:          "video_events",
		Name:              "transcode.jobs",
		RetryName:         "transcode.retry",
		DeadName:          "transcode.dead",
		BindingKey:        "video.upload.completed",
		RetryDelaySeconds: 1,
		MaxAttempts:       3,
	}, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("NewRabbitConsumer() error = %v", err)
	}
	consumer.Close()
	if !channel.closed || !conn.closed {
		t.Fatalf("Close() did not close both channel and connection")
	}
}

func TestNewRabbitConsumerReturnsErrors(t *testing.T) {
	originalDial := amqpDial
	originalAttempts := rabbitDialAttempts
	originalSleep := sleep
	t.Cleanup(func() { amqpDial = originalDial })
	t.Cleanup(func() { rabbitDialAttempts = originalAttempts })
	t.Cleanup(func() { sleep = originalSleep })

	rabbitDialAttempts = 1
	sleep = func(time.Duration) {}
	amqpDial = func(string) (amqpConnection, error) { return nil, errors.New("dial failed") }
	if _, err := NewRabbitConsumer("amqp://broken", config.QueueConfig{}, log.New(io.Discard, "", 0)); err == nil {
		t.Fatalf("NewRabbitConsumer() error = nil, want error")
	}

	amqpDial = func(string) (amqpConnection, error) {
		return &fakeConnection{err: errors.New("channel failed")}, nil
	}
	if _, err := NewRabbitConsumer("amqp://guest:guest@localhost:5672/", config.QueueConfig{}, log.New(io.Discard, "", 0)); err == nil {
		t.Fatalf("NewRabbitConsumer() channel error = nil, want error")
	}

	failCases := []struct {
		name    string
		channel *fakeChannel
	}{
		{name: "exchange", channel: &fakeChannel{exchangeErr: errors.New("exchange failed")}},
		{name: "queue", channel: &fakeChannel{queueErr: errors.New("queue failed"), passiveErr: errors.New("passive failed")}},
		{name: "bind", channel: &fakeChannel{bindErr: errors.New("bind failed")}},
		{name: "qos", channel: &fakeChannel{qosErr: errors.New("qos failed")}},
	}
	for _, tc := range failCases {
		t.Run(tc.name, func(t *testing.T) {
			conn := &fakeConnection{channel: tc.channel}
			amqpDial = func(string) (amqpConnection, error) { return conn, nil }
			rabbitDialAttempts = 1
			sleep = func(time.Duration) {}
			if _, err := NewRabbitConsumer("amqp://guest:guest@localhost:5672/", config.QueueConfig{}, log.New(io.Discard, "", 0)); err == nil {
				t.Fatalf("NewRabbitConsumer() error = nil, want error")
			}
		})
	}
}

func TestNewRabbitConsumerReusesExistingRetryQueue(t *testing.T) {
	originalDial := amqpDial
	originalAttempts := rabbitDialAttempts
	originalSleep := sleep
	t.Cleanup(func() { amqpDial = originalDial })
	t.Cleanup(func() { rabbitDialAttempts = originalAttempts })
	t.Cleanup(func() { sleep = originalSleep })

	channel := &fakeChannel{
		passiveErr: nil,
	}
	conn := &fakeConnection{channel: channel}
	amqpDial = func(string) (amqpConnection, error) { return conn, nil }
	rabbitDialAttempts = 1
	sleep = func(time.Duration) {}

	channel.queueErrByName = map[string]error{
		"transcode.retry": errors.New("PRECONDITION_FAILED - inequivalent arg 'x-message-ttl'"),
	}
	consumer, err := NewRabbitConsumer("amqp://guest:guest@localhost:5672/", config.QueueConfig{
		Exchange:          "video_events",
		Name:              "transcode.jobs",
		RetryName:         "transcode.retry",
		DeadName:          "transcode.dead",
		BindingKey:        "video.upload.completed",
		RetryDelaySeconds: 60,
	}, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("NewRabbitConsumer() error = %v", err)
	}
	if consumer == nil {
		t.Fatalf("NewRabbitConsumer() consumer = nil")
	}
}

func TestDialRabbitMQRetriesBeforeSuccess(t *testing.T) {
	originalDial := amqpDial
	originalAttempts := rabbitDialAttempts
	originalSleep := sleep
	t.Cleanup(func() { amqpDial = originalDial })
	t.Cleanup(func() { rabbitDialAttempts = originalAttempts })
	t.Cleanup(func() { sleep = originalSleep })

	var calls int
	amqpDial = func(string) (amqpConnection, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("temporary")
		}
		return &fakeConnection{channel: &fakeChannel{}}, nil
	}
	rabbitDialAttempts = 5
	sleep = func(time.Duration) {}

	conn, err := dialRabbitMQ("amqp://guest:guest@localhost:5672/")
	if err != nil {
		t.Fatalf("dialRabbitMQ() error = %v", err)
	}
	if conn == nil {
		t.Fatalf("dialRabbitMQ() conn = nil")
	}
	if calls != 3 {
		t.Fatalf("dialRabbitMQ() calls = %d, want 3", calls)
	}
}

func TestRabbitConsumerRun(t *testing.T) {
	channel := &fakeChannel{deliveries: make(chan amqp.Delivery)}
	consumer := &RabbitConsumer{
		channel: channel,
		cfg:     config.QueueConfig{Name: "transcode.jobs", RetryName: "transcode.retry", DeadName: "transcode.dead", MaxAttempts: 3},
		logger:  log.New(io.Discard, "", 0),
	}

	done := make(chan error, 1)
	go func() {
		done <- consumer.Run(context.Background(), func(context.Context, Delivery) error { return nil })
	}()
	close(channel.deliveries)
	if err := <-done; err == nil {
		t.Fatalf("Run() error = nil, want delivery channel closed")
	}
}

func TestRabbitConsumerRunConsumeError(t *testing.T) {
	consumer := &RabbitConsumer{
		channel: &fakeChannel{consumeErr: errors.New("consume failed")},
		cfg:     config.QueueConfig{Name: "transcode.jobs", RetryName: "transcode.retry", DeadName: "transcode.dead", MaxAttempts: 3},
		logger:  log.New(io.Discard, "", 0),
	}
	if err := consumer.Run(context.Background(), func(context.Context, Delivery) error { return nil }); err == nil {
		t.Fatalf("Run() error = nil, want error")
	}
}

func TestRabbitConsumerRunPublishesRetryThenAcks(t *testing.T) {
	channel := &fakeChannel{deliveries: make(chan amqp.Delivery)}
	consumer := &RabbitConsumer{
		channel: channel,
		cfg:     config.QueueConfig{Name: "transcode.jobs", RetryName: "transcode.retry", DeadName: "transcode.dead", MaxAttempts: 3},
		logger:  log.New(io.Discard, "", 0),
	}

	done := make(chan error, 1)
	go func() {
		done <- consumer.Run(context.Background(), func(context.Context, Delivery) error {
			return errors.New("temporary")
		})
	}()
	channel.deliveries <- amqp.Delivery{Body: []byte(`{"videoId":"v1"}`)}
	close(channel.deliveries)
	_ = <-done

	if len(channel.publishKeys) != 1 || channel.publishKeys[0] != "transcode.retry" {
		t.Fatalf("publishKeys = %v", channel.publishKeys)
	}
	if got := channel.published[0].Headers["x-transcode-attempt"]; got != 2 {
		t.Fatalf("attempt header = %v", got)
	}
}

type fakeTerminalError struct{}

func (fakeTerminalError) Error() string  { return "terminal" }
func (fakeTerminalError) Terminal() bool { return true }

func TestRabbitConsumerRunPublishesDeadForTerminalError(t *testing.T) {
	channel := &fakeChannel{deliveries: make(chan amqp.Delivery)}
	consumer := &RabbitConsumer{
		channel: channel,
		cfg:     config.QueueConfig{Name: "transcode.jobs", RetryName: "transcode.retry", DeadName: "transcode.dead", MaxAttempts: 3},
		logger:  log.New(io.Discard, "", 0),
	}

	done := make(chan error, 1)
	go func() {
		done <- consumer.Run(context.Background(), func(context.Context, Delivery) error {
			return fakeTerminalError{}
		})
	}()
	channel.deliveries <- amqp.Delivery{Body: []byte(`{"videoId":"v1"}`)}
	close(channel.deliveries)
	_ = <-done

	if len(channel.publishKeys) != 1 || channel.publishKeys[0] != "transcode.dead" {
		t.Fatalf("publishKeys = %v", channel.publishKeys)
	}
}

func TestRabbitConsumerRunPublishesDeadAfterMaxAttempts(t *testing.T) {
	channel := &fakeChannel{deliveries: make(chan amqp.Delivery)}
	consumer := &RabbitConsumer{
		channel: channel,
		cfg:     config.QueueConfig{Name: "transcode.jobs", RetryName: "transcode.retry", DeadName: "transcode.dead", MaxAttempts: 3},
		logger:  log.New(io.Discard, "", 0),
	}

	done := make(chan error, 1)
	go func() {
		done <- consumer.Run(context.Background(), func(context.Context, Delivery) error {
			return errors.New("temporary")
		})
	}()
	channel.deliveries <- amqp.Delivery{
		Body:    []byte(`{"videoId":"v1"}`),
		Headers: amqp.Table{"x-transcode-attempt": int32(3)},
	}
	close(channel.deliveries)
	_ = <-done

	if len(channel.publishKeys) != 1 || channel.publishKeys[0] != "transcode.dead" {
		t.Fatalf("publishKeys = %v", channel.publishKeys)
	}
	if got := channel.published[0].Headers["x-transcode-attempt"]; got != 3 {
		t.Fatalf("attempt header = %v", got)
	}
}

func TestDeliveryAttempt(t *testing.T) {
	tests := []struct {
		name    string
		headers amqp.Table
		want    int
	}{
		{name: "nil", headers: nil, want: 1},
		{name: "int", headers: amqp.Table{"x-transcode-attempt": int(2)}, want: 2},
		{name: "int32", headers: amqp.Table{"x-transcode-attempt": int32(4)}, want: 4},
		{name: "int64", headers: amqp.Table{"x-transcode-attempt": int64(5)}, want: 5},
		{name: "float64", headers: amqp.Table{"x-transcode-attempt": float64(6)}, want: 6},
		{name: "zero", headers: amqp.Table{"x-transcode-attempt": 0}, want: 1},
		{name: "invalid", headers: amqp.Table{"x-transcode-attempt": "bad"}, want: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := deliveryAttempt(tc.headers); got != tc.want {
				t.Fatalf("deliveryAttempt() = %d, want %d", got, tc.want)
			}
		})
	}
}

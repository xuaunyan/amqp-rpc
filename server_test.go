package amqprpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/streadway/amqp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendWithReply(t *testing.T) {
	cert := Certificates{}

	s := NewServer(serverTestURL).WithDialConfig(amqp.Config{
		TLSClientConfig: cert.TLSConfig(),
	})

	assert.NotNil(t, s.dialconfig.TLSClientConfig, "dialconfig on server is set")

	s.Bind(DirectBinding("myqueue", func(ctx context.Context, rw *ResponseWriter, d amqp.Delivery) {
		fmt.Fprintf(rw, "Got message: %s", d.Body)
	}))

	stop := startAndWait(s)
	defer stop()

	c := NewClient(serverTestURL)
	defer c.Stop()

	request := NewRequest().WithRoutingKey("myqueue").WithBody("this is a message")
	reply, err := c.Send(request)

	assert.Nil(t, err, "client exist")
	assert.Equal(t, []byte("Got message: this is a message"), reply.Body, "got reply")
}

func TestNoAutomaticAck(t *testing.T) {
	deleteQueue("no-auto-ack") // Ensure queue is clean from the start.

	s := NewServer(serverTestURL).WithAutoAck(false)

	calls := make(chan struct{}, 2)

	s.Bind(DirectBinding("no-auto-ack", func(ctc context.Context, responseWriter *ResponseWriter, d amqp.Delivery) {
		calls <- struct{}{}
	}))

	stop := startAndWait(s)

	c := NewClient(serverTestURL)
	defer c.Stop()

	request := NewRequest().WithRoutingKey("no-auto-ack").WithResponse(false)
	_, err := c.Send(request)
	require.NoError(t, err)

	// Wait for the first message to arrive.
	select {
	case <-calls:
		// We got the message, now we stop the server without having acked the
		// delivery.
		stop()
	case <-time.After(10 * time.Second):
		t.Fatal("wait time exeeded")
	}

	// Restart the server. This should make RabbitMQ deliver the delivery
	// again.
	stop = startAndWait(s)
	defer stop()

	select {
	case <-calls:
		// Nice!
	case <-time.After(10 * time.Second):
		t.Fatal("wait time exeeded")
	}
}

func TestMiddleware(t *testing.T) {
	mw := func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, rw *ResponseWriter, d amqp.Delivery) {
			if ctx.Value(CtxQueueName).(string) == "denied" {
				fmt.Fprint(rw, "routing key 'denied' is not allowed")
				return
			}

			next(ctx, rw, d)
		}
	}

	s := NewServer(serverTestURL).AddMiddleware(mw)

	s.Bind(DirectBinding("allowed", func(ctx context.Context, rw *ResponseWriter, d amqp.Delivery) {
		fmt.Fprint(rw, "this is allowed")
	}))

	s.Bind(DirectBinding("denied", func(ctx context.Context, rw *ResponseWriter, d amqp.Delivery) {
		fmt.Fprint(rw, "this is not allowed")
	}))

	stop := startAndWait(s)
	defer stop()

	c := NewClient(serverTestURL)
	defer c.Stop()

	request := NewRequest().WithRoutingKey("allowed")
	reply, err := c.Send(request)

	assert.Nil(t, err, "no error")
	assert.Equal(t, []byte("this is allowed"), reply.Body, "allowed middleware callable")

	request = NewRequest().WithRoutingKey("denied")
	reply, err = c.Send(request)

	assert.Nil(t, err, "no error")
	assert.Equal(t, []byte("routing key 'denied' is not allowed"), reply.Body, "denied middleware not callable")
}

func TestServerReconnect(t *testing.T) {
	s := NewServer(serverTestURL).
		WithAutoAck(false)

	s.Bind(DirectBinding("myqueue", func(ctx context.Context, rw *ResponseWriter, d amqp.Delivery) {
		_ = d.Ack(false)
		fmt.Fprintf(rw, "Hello")
	}))

	stop := startAndWait(s)
	defer stop()

	c := NewClient(serverTestURL)
	defer c.Stop()

	request := NewRequest().WithRoutingKey("myqueue")
	reply, err := c.Send(request)
	require.NoError(t, err)
	assert.Equal(t, []byte("Hello"), reply.Body)

	closeAllConnections()

	request = NewRequest().WithRoutingKey("myqueue")
	reply, err = c.Send(request)
	require.NoError(t, err)
	assert.Equal(t, []byte("Hello"), reply.Body)
}

func TestServerOnStarted(t *testing.T) {
	errs := make(chan string, 4)

	s := NewServer(serverTestURL)
	s.OnStarted(func(inC, outC *amqp.Connection, inCh, outCh *amqp.Channel) {
		if inC == nil {
			errs <- "inC was nil"
		}
		if outC == nil {
			errs <- "outC was nil"
		}
		if inCh == nil {
			errs <- "inCh was nil"
		}
		if outCh == nil {
			errs <- "outCh was nil"
		}

		close(errs)
	})

	stop := startAndWait(s)
	defer stop()

	select {
	case e, ok := <-errs:
		if ok {
			t.Fatal(e)
		}

	case <-time.After(time.Second):
		t.Error("OnStarted was never called")
	}
}

func TestStopWhenStarting(t *testing.T) {
	s := NewServer("amqp://guest:guest@wont-connect.com:5672")

	done := make(chan struct{})
	go func() {
		s.ListenAndServe()
		close(done)
	}()

	// Cannot use OnStarted() since we won't successfully start.
	time.Sleep(10 * time.Millisecond)
	s.Stop()

	// Block so we're sure that we actually exited.
	select {
	case <-done:
		// The done channel was closed!
		assert.Nil(t, nil, "no error")
	case <-time.After(10 * time.Second):
		// No success within 10 seconds
		t.Error("Didn't succeed to close server")
	}
}

func TestServerConfig(t *testing.T) {
	s := NewServer(serverTestURL)
	assert.NotNil(t, s)
	assert.True(t, s.exchangeDeclareSettings.Durable)
	assert.Equal(t, s.consumeSettings.QoSPrefetchCount, 10)

	qdSettings := QueueDeclareSettings{
		DeleteWhenUnused: true,
		Durable:          true,
	}
	cSettings := ConsumeSettings{
		QoSPrefetchCount: 20,
		QoSPrefetchSize:  100,
		Consumer:         "myconsumer",
	}
	eSettings := ExchangeDeclareSettings{
		Durable:    false,
		AutoDelete: true,
	}

	s.WithQueueDeclareSettings(qdSettings).
		WithConsumeSettings(cSettings).
		WithExchangeDeclareSettings(eSettings)

	assert.Equal(t, s.queueDeclareSettings, qdSettings)
	assert.Equal(t, s.consumeSettings, cSettings)
	assert.Equal(t, s.exchangeDeclareSettings, eSettings)
}

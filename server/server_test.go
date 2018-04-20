package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"testing"
	"time"

	"github.com/bombsimon/amqp-rpc/client"
	"github.com/bombsimon/amqp-rpc/logger"
	"github.com/streadway/amqp"
	. "gopkg.in/go-playground/assert.v1"
)

func noEcho() {
	silentLogger := log.New(ioutil.Discard, "", log.LstdFlags)

	logger.SetInfoLogger(silentLogger)
	logger.SetWarnLogger(silentLogger)
}

func TestPublishReply(t *testing.T) {
	noEcho()

	s := New()
	s.SetTLSConfig(new(tls.Config))

	NotEqual(t, s.dialconfig.TLSClientConfig, nil)

	s.AddHandler("myqueue", func(ctx context.Context, d *amqp.Delivery) []byte {
		return []byte(fmt.Sprintf("Got message: %s", d.Body))
	})

	go s.ListenAndServe("amqp://guest:guest@localhost:5672/")

	client := client.New("amqp://guest:guest@localhost:5672/")
	reply, err := client.Publish("myqueue", []byte("this is a message"), true)

	Equal(t, err, nil)

	Equal(t, reply.Body, []byte("Got message: this is a message"))
}

func testDialer(t *testing.T) (func(string, string) (net.Conn, error), func() net.Conn) {
	var conn net.Conn

	return func(network, addr string) (net.Conn, error) {
			var err error

			conn, err = net.DialTimeout(network, addr, 2*time.Second)
			if err != nil {
				return nil, err
			}
			// Heartbeating hasn't started yet, don't stall forever on a dead server.
			// A deadline is set for TLS and AMQP handshaking. After AMQP is established,
			// the deadline is cleared in openComplete.
			if err = conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
				return nil, err
			}

			return conn, nil
		}, func() net.Conn {
			return conn
		}

}

func TestReconnect(t *testing.T) {
	noEcho()

	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")

	Equal(t, err, nil)
	defer conn.Close()

	ch, err := conn.Channel()
	Equal(t, err, nil)
	defer ch.Close()

	ch.QueueDelete(
		"myqueue",
		false, // ifUnused
		false, // ifEmpty
		false, // noWait
	)

	s := New()
	dialer, getNetConn := testDialer(t)
	s.SetAMQPConfig(amqp.Config{
		Dial: dialer,
	})

	s.AddHandler("myqueue", func(ctx context.Context, d *amqp.Delivery) []byte {
		return []byte(fmt.Sprintf("Got message: %s", d.Body))
	})

	go s.ListenAndServe("amqp://guest:guest@localhost:5672/")

	// Sleep a bit to ensure server is started.
	time.Sleep(50 * time.Millisecond)
	client := client.New("amqp://guest:guest@localhost:5672/")

	for i := 0; i < 2; i++ {
		message := []byte(fmt.Sprintf("this is message %v", i))
		reply, err := client.Publish("myqueue", message, true)

		Equal(t, err, nil)

		getNetConn().Close()

		Equal(t, reply.Body, []byte(fmt.Sprintf("Got message: %s", message)))
	}
}
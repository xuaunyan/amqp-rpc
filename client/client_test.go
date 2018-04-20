package client

import (
	"context"
	"fmt"
	"testing"

	"github.com/bombsimon/amqp-rpc/server"
	"github.com/streadway/amqp"
	. "gopkg.in/go-playground/assert.v1"
)

func TestClient(t *testing.T) {
	server := server.New()
	server.AddHandler("myqueue", func(ctx context.Context, d *amqp.Delivery) []byte {
		return []byte(fmt.Sprintf("Got message: %s", d.Body))
	})

	go server.ListenAndServe("amqp://guest:guest@localhost:5672/")

	client := New("amqp://guest:guest@localhost:5672/")
	NotEqual(t, client, nil)

	response, err := client.Publish("myqueue", []byte("client testing"), true)
	Equal(t, err, nil)
	Equal(t, response.Body, []byte("Got message: client testing"))
}

func TestExistingConnection(t *testing.T) {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	Equal(t, err, nil)

	client := NewWithConnection(conn)
	response, err := client.Publish("non-existing-queue", []byte("ignore reply"), false)
	Equal(t, response, nil)
	Equal(t, err, nil)
}
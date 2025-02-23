package main

/*
This example demonstrates how middlewares can be used to plug code before or
after requests ar sent and/or received.

This example uses the word "password" but is meant to demonstrate a kind of
authorization mechanism with i.e. JWT which is exchanged on the server side for
each request.
*/

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	amqprpc "github.com/0x4b53/amqp-rpc/v2"
)

var (
	password string
	url      = "amqp://guest:guest@localhost:5672"
)

func main() {
	go startServer()

	c := amqprpc.NewClient(url).AddMiddleware(handlePassword)
	r := amqprpc.NewRequest().WithRoutingKey("exchanger")

	for _, i := range []int{1, 2, 3} {
		fmt.Printf("%-10s %d: password is '%s'\n", "Request", i, password)

		resp, err := c.Send(r)
		if err != nil {
			fmt.Println("Woops: ", err)
		} else {
			fmt.Printf("%-10s %d: password is '%s' (body is '%s')\n", "Response", i, resp.Headers["password"], resp.Body)
		}
	}

	r2 := amqprpc.NewRequest().WithRoutingKey("exchanger").AddMiddleware(
		func(next amqprpc.SendFunc) amqprpc.SendFunc {
			return func(r *amqprpc.Request) (*amqp.Delivery, error) {
				fmt.Println(">> I'm being executed before Send(), but only for ONE request!")
				r.Publishing.Headers["password"] = "i am custom"

				return next(r)
			}
		},
	)

	resp, err := c.Send(r2)
	if err != nil {
		fmt.Println("Whoops: ", err)
	}

	fmt.Printf("%-10s %d: this request got custom body '%s'\n", "Request", 4, resp.Body)
}

func handlePassword(next amqprpc.SendFunc) amqprpc.SendFunc {
	return func(r *amqprpc.Request) (*amqp.Delivery, error) {
		if password == "" {
			fmt.Println(">> I'm being executed before Send(), I'm ensuring you've got a password header!")
			password = uuid.New().String()
		}

		r.Publishing.Headers["password"] = password

		// This will always run the clients send function in the end.
		d, e := next(r)

		if newPassword, ok := d.Headers["password"].(string); ok {
			password = newPassword
		}

		return d, e
	}
}

// Middleware executing before or after being handled in server.
func exchangeHeader(next amqprpc.HandlerFunc) amqprpc.HandlerFunc {
	return func(ctx context.Context, rw *amqprpc.ResponseWriter, d amqp.Delivery) {
		next(ctx, rw, d)

		rw.WriteHeader("password", uuid.New().String())
	}
}

func startServer() {
	s := amqprpc.NewServer(url)

	s.AddMiddleware(exchangeHeader)

	s.Bind(amqprpc.DirectBinding("exchanger", func(c context.Context, rw *amqprpc.ResponseWriter, d amqp.Delivery) {
		fmt.Fprintf(rw, d.Headers["password"].(string))
	}))

	s.ListenAndServe()
}

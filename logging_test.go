package amqprpc

import (
	"bytes"
	"log"
	"testing"
	"time"

	"github.com/bombsimon/amqp-rpc/testhelpers"
	. "gopkg.in/go-playground/assert.v1"
)

func TestServerLogging(t *testing.T) {
	buf := bytes.Buffer{}
	logger := log.New(&buf, "TEST", log.LstdFlags)

	s := NewServer(serverTestURL)
	s.WithDebugLogger(logger.Printf)
	s.WithErrorLogger(logger.Printf)

	stop := testhelpers.StartServer(s)
	defer stop()

	NotEqual(t, buf.String(), "")
	MatchRegex(t, buf.String(), "^TEST")
}

func TestClientLogging(t *testing.T) {
	buf := bytes.Buffer{}
	logger := log.New(&buf, "TEST", log.LstdFlags)

	c := NewClient("amqp://guest:guest@localhost:5672/")
	c.WithDebugLogger(logger.Printf)
	c.WithErrorLogger(logger.Printf)

	c.Send(NewRequest("foobar").WithTimeout(time.Microsecond))

	NotEqual(t, buf.String(), "")
	MatchRegex(t, buf.String(), "^TEST")
}
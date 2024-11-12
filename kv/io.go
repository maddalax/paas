package kv

import (
	"github.com/nats-io/nats.go"
	"paas/kv/subject"
)

// NatsWriter is a structure that implements io.Writer to write to a NATS JetStream stream
type NatsWriter struct {
	js      nats.JetStreamContext
	subject subject.Subject
}

func (c *Client) NewNatsWriter(subject subject.Subject) *NatsWriter {
	return &NatsWriter{
		js:      c.js,
		subject: subject,
	}
}

// Write implements the io.Writer interface
func (nw *NatsWriter) Write(p []byte) (n int, err error) {
	// Publish the data to the NATS JetStream subject
	_, err = nw.js.Publish(nw.subject, p)

	if err != nil {
		return 0, err
	}

	// Return the length of the written data
	return len(p), nil
}

type EphemeralNatsWriter struct {
	subject subject.Subject
	c       *Client
}

func (c *Client) NewEphemeralNatsWriter(subject subject.Subject) *EphemeralNatsWriter {
	return &EphemeralNatsWriter{
		subject: subject,
		c:       c,
	}
}

func (nw *EphemeralNatsWriter) Write(p []byte) (n int, err error) {
	err = nw.c.Publish(nw.subject, p)

	if err != nil {
		return 0, err
	}

	return len(p), nil
}

package engine

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net"
	"time"
)

func decodeRequest(c net.Conn) (Request, error) {
	c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var q Request
	dec := json.NewDecoder(io.LimitReader(c, 65536))
	dec.DisallowUnknownFields()
	err := dec.Decode(&q)
	return q, err
}

func reply(c net.Conn, q Request, v Response) {
	stamp(&v, q.Version)
	c.SetWriteDeadline(time.Now().Add(time.Second))
	json.NewEncoder(c).Encode(v)
	c.Close()
}

func commitWhileServing(s *Store, connections <-chan net.Conn, pending chan<- net.Conn, save func() error) error {
	snapshot := *s
	snapshot.Runs = maps.Clone(s.Runs)
	snapshot.Receipts = maps.Clone(s.Receipts)
	snapshot.ReceiptIndex = maps.Clone(s.ReceiptIndex)
	done := make(chan error, 1)
	go func() { done <- save() }()
	for {
		select {
		case err := <-done:
			return err
		case c, ok := <-connections:
			if !ok {
				connections = nil
				continue
			}
			q, err := decodeRequest(c)
			if err != nil {
				reply(c, q, Response{Outcome: "refused", Error: "invalid request: " + err.Error()})
			} else if q.Op == "inspect" || q.Op == "transcript" {
				reply(c, q, handle(&snapshot, q))
			} else if len(pending) == cap(pending) {
				reply(c, q, Response{Outcome: "refused", Error: "storage commit queue is full; retry the same request and action_id"})
			} else {
				data, _ := json.Marshal(q)
				pending <- &queuedConnection{Conn: c, input: io.MultiReader(bytes.NewReader(append(data, '\n')), c)}
			}
		}
	}
}

type queuedConnection struct {
	net.Conn
	input io.Reader
}

func (c *queuedConnection) Read(p []byte) (int, error) { return c.input.Read(p) }

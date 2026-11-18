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
	var q Request
	if err := c.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		return q, err
	}
	dec := json.NewDecoder(io.LimitReader(c, 65536))
	dec.DisallowUnknownFields()
	err := dec.Decode(&q)
	return q, err
}

func reply(c net.Conn, q Request, v Response) {
	stamp(&v, q.Version)
	defer func() { _ = c.Close() }()
	if err := c.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return
	}
	_ = json.NewEncoder(c).Encode(v)
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
			} else if q.Op == "watch" || q.Op == "models" {
				reply(c, q, Response{Outcome: "refused", Error: "storage commit in progress; retry the read request"})
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

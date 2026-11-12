package runner

import "bytes"

type Buffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *Buffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len()+n > b.limit {
		b.exceeded = true
		return n, nil
	}
	return b.Buffer.Write(p)
}

func NewBuffer(limit int) *Buffer { return &Buffer{limit: limit} }
func (b *Buffer) Exceeded() bool  { return b.exceeded }

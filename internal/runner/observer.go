package runner

import (
	"context"
	"io"
	"time"
)

type ProcessObserver func(command []string, stdout, stderr io.Writer) (io.Writer, io.Writer, func(int, time.Duration))
type observerKey struct{}

func WithObserver(ctx context.Context, observer ProcessObserver) context.Context {
	return context.WithValue(ctx, observerKey{}, observer)
}

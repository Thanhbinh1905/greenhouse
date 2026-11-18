package forge

import "context"

type CheckState struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Conclusion *string `json:"conclusion"`
	Head       string  `json:"head"`
}

type checkObserverKey struct{}

func WithCheckObserver(ctx context.Context, observe func([]CheckState)) context.Context {
	return context.WithValue(ctx, checkObserverKey{}, observe)
}

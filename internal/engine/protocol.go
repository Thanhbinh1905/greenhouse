package engine

import (
	"errors"
	"fmt"
)

const (
	protocolMin = 1
	MaxVersion  = 2
)

type Protocol struct {
	Version int `json:"version"`
	Min     int `json:"min"`
	Max     int `json:"max"`
}

func checkVersion(q Request) error {
	if q.Version < protocolMin || q.Version > MaxVersion {
		return fmt.Errorf("unsupported protocol version %d; this daemon supports %d-%d", q.Version, protocolMin, MaxVersion)
	}
	if q.Version < 2 && q.Op == "watch" {
		return errors.New("watch requires protocol version 2")
	}
	if q.Version < 2 && (q.Host != nil || q.Grant != "" || q.Responses != nil) {
		return errors.New("host mode requires protocol version 2")
	}
	return nil
}

func stamp(v *Response, requested int) {
	v.Version = requested
	if requested < protocolMin || requested > MaxVersion {
		v.Version = MaxVersion
	}
	v.Protocol = &Protocol{Version: v.Version, Min: protocolMin, Max: MaxVersion}
}

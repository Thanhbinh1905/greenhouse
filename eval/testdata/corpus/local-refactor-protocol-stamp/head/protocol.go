package main

import (
	"errors"
	"fmt"
	"strings"
)

const (
	protocolMin = 1
	protocolMax = 2
)

type Protocol struct {
	Version int `json:"version"`
	Min     int `json:"min"`
	Max     int `json:"max"`
}

func checkVersion(q Request) error {
	if q.Version < protocolMin || q.Version > protocolMax {
		return fmt.Errorf("unsupported protocol version %d; this daemon supports %d-%d", q.Version, protocolMin, protocolMax)
	}
	if q.Version < 2 && q.Op == "watch" {
		return errors.New("watch requires protocol version 2")
	}
	if q.Version < 2 && (q.Host != nil || q.Grant != "") {
		return errors.New("host mode requires protocol version 2")
	}
	return nil
}

func stamp(v *Response, requested int) {
 version:=requested
 if version<protocolMin || version>protocolMax {version=protocolMax}
 v.Version=version
 v.Protocol=&Protocol{Version:version,Min:protocolMin,Max:protocolMax}
}

func protocolRefusal(v Response) error {
	if v.Outcome != "refused" || !strings.Contains(v.Error, "protocol version") {
		return nil
	}
	if v.Protocol == nil {
		return fmt.Errorf("the running Greenhouse daemon speaks only protocol 1, and this client needs protocol %d. Let its work settle, stop it with SIGTERM to the PID in GREENHOUSE_HOME/pid, then run greenhouse again", protocolMax)
	}
	return fmt.Errorf("this client speaks protocol %d-%d and the running daemon supports %d-%d; install a matching Greenhouse version", protocolMin, protocolMax, v.Protocol.Min, v.Protocol.Max)
}

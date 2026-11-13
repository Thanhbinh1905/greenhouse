package terminal

import (
	"fmt"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

func protocolRefusal(v engine.Response) error {
	if v.Outcome != "refused" || !strings.Contains(v.Error, "protocol version") {
		return nil
	}
	if v.Protocol == nil {
		return fmt.Errorf("the running Greenhouse daemon speaks only protocol 1, and this client needs protocol %d. Let its work settle, then stop it with SIGTERM to the PID in GREENHOUSE_HOME/pid after confirming that process is the Greenhouse daemon. Then run greenhouse again", engine.MaxVersion)
	}
	return fmt.Errorf("this client speaks protocol %d-%d and the running daemon supports %d-%d; install a matching Greenhouse version", 1, engine.MaxVersion, v.Protocol.Min, v.Protocol.Max)
}

package engine

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

type Confinement struct {
	Mechanism string   `json:"mechanism"`
	ABI       int      `json:"abi,omitempty"`
	Status    string   `json:"status"`
	Role      string   `json:"role"`
	Writable  []string `json:"writable"`
	Ports     []int    `json:"tcp_connect_ports,omitempty"`
	LockDirs  []string `json:"native_auth_lock_dirs,omitempty"`
	Network   string   `json:"network"`
	Reason    string   `json:"reason,omitempty"`
}

type confinePolicy struct {
	ABI      int      `json:"abi"`
	Writable []string `json:"writable"`
	Files    []string `json:"files"`
	Locks    []string `json:"locks,omitempty"`
}

const (
	landlockCreateRuleset = 444
	landlockAddRule       = 445
	landlockRestrictSelf  = 446

	fsExecute   = 1 << 0
	fsWriteFile = 1 << 1
	fsReadFile  = 1 << 2
	fsReadDir   = 1 << 3
	fsRefer     = 1 << 13
	fsTruncate  = 1 << 14
	netBindTCP  = 1 << 0

	oPath          = 0x200000
	prSetNoNewPriv = 38
)

func landlockABI() (int, error) {
	if runtime.GOOS != "linux" {
		return 0, errors.New("OS confinement is implemented only on Linux")
	}
	abi, _, errno := syscall.Syscall(landlockCreateRuleset, 0, 0, 1)
	if errno != 0 {
		return 0, fmt.Errorf("landlock unavailable: %v", errno)
	}
	return int(abi), nil
}

func handledFS(abi int) uint64 {
	mask := uint64(1<<13 - 1)
	if abi >= 2 {
		mask |= fsRefer
	}
	if abi >= 3 {
		mask |= fsTruncate
	}
	return mask
}

func backendDegradedReason(backend string) string {
	switch backend {
	case backendClaude:
		return "Claude Code may require writable session state in its config directory; credentials and config remain read-only"
	case backendCodex:
		return "Codex requires writable session state in CODEX_HOME; config and credentials remain read-only"
	default:
		return ""
	}
}

func confinementFor(role string, config ReviewerConfig, writable []string) (*confinePolicy, Confinement) {
	c := Confinement{Mechanism: "landlock", Role: role, Writable: []string{}}
	degraded := backendDegradedReason(backendOf(config))
	paths := append([]string{}, writable...)
	if trusted, err := trustedBackends(); err == nil {
		paths = append(paths, trusted.Writable[role]...)
	}
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.IsDir() && filepath.IsAbs(p) {
			c.Writable = append(c.Writable, filepath.Clean(p))
		}
	}
	abi, err := landlockABI()
	if err != nil {
		c.Status, c.Network, c.Reason = "degraded", "unrestricted", err.Error()+"; the agent runs without OS confinement"
		return nil, c
	}
	c.ABI = abi
	policy := &confinePolicy{ABI: abi, Writable: c.Writable, Files: []string{"/dev/null"}}
	if backendOf(config) == backendPi {
		nativeDir := os.Getenv("PI_CODING_AGENT_DIR")
		if nativeDir == "" {
			if userHome, err := os.UserHomeDir(); err == nil {
				nativeDir = filepath.Join(userHome, ".pi", "agent")
			}
		}
		if info, err := os.Stat(nativeDir); err == nil && info.IsDir() && filepath.IsAbs(nativeDir) {
			policy.Locks = []string{nativeDir}
			c.LockDirs = policy.Locks
		}
	}
	c.Status = "enforced"
	if degraded != "" {
		c.Status, c.Reason = "degraded", degraded
	}
	if abi < 4 {
		c.Status, c.Network = "degraded", "unrestricted"
		c.Reason = strings.TrimPrefix(c.Reason+fmt.Sprintf("; Landlock ABI %d cannot restrict TCP; filesystem writes are confined, network is not", abi), "; ")
	} else {
		c.Status = "degraded"
		c.Network = "unrestricted outbound TCP for native provider routing; listening sockets denied"
		c.Reason = strings.TrimPrefix(c.Reason+"; filesystem writes remain confined; provider routing belongs to the native agent", "; ")
	}
	return policy, c
}

func confinedCommand(policy *confinePolicy, exe string, args []string) (string, []string, error) {
	if policy == nil {
		return exe, args, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	encoded, _ := json.Marshal(policy)
	return self, append([]string{"_confine", string(encoded), exe}, args...), nil
}

func init() {
	if len(os.Args) > 3 && os.Args[1] == "_confine" {
		runtime.LockOSThread()
		var policy confinePolicy
		if err := json.Unmarshal([]byte(os.Args[2]), &policy); err != nil {
			fmt.Fprintln(os.Stderr, "greenhouse confinement: invalid policy")
			os.Exit(125)
		}
		if err := enforce(policy); err != nil {
			fmt.Fprintln(os.Stderr, "greenhouse confinement:", err)
			os.Exit(125)
		}
		err := syscall.Exec(os.Args[3], os.Args[3:], os.Environ())
		fmt.Fprintln(os.Stderr, "greenhouse confinement: exec:", err)
		os.Exit(126)
	}
}

func enforce(policy confinePolicy) error {
	handled := handledFS(policy.ABI)
	attr := make([]byte, 8, 16)
	binary.LittleEndian.PutUint64(attr, handled)
	if policy.ABI >= 4 {
		attr = binary.LittleEndian.AppendUint64(attr, netBindTCP)
	}
	fd, _, errno := syscall.Syscall(landlockCreateRuleset, uintptr(unsafe.Pointer(&attr[0])), uintptr(len(attr)), 0)
	if errno != 0 {
		return fmt.Errorf("create ruleset: %v", errno)
	}
	defer syscall.Close(int(fd))
	addPath := func(path string, access uint64) error {
		target, err := syscall.Open(path, oPath|syscall.O_CLOEXEC, 0)
		if err != nil {
			return fmt.Errorf("%s: %v", path, err)
		}
		defer syscall.Close(target)
		rule := make([]byte, 12)
		binary.LittleEndian.PutUint64(rule, access)
		binary.LittleEndian.PutUint32(rule[8:], uint32(target))
		if _, _, errno := syscall.Syscall6(landlockAddRule, fd, 1, uintptr(unsafe.Pointer(&rule[0])), 0, 0, 0); errno != 0 {
			return fmt.Errorf("rule %s: %v", path, errno)
		}
		return nil
	}
	if err := addPath("/", fsExecute|fsReadFile|fsReadDir); err != nil {
		return err
	}
	for _, dir := range policy.Writable {
		if err := addPath(dir, handled); err != nil {
			return err
		}
	}
	for _, dir := range policy.Locks {
		if err := addPath(dir, fsReadDir|fsReadFile|1<<7|1<<4); err != nil {
			return err
		}
	}
	fileAccess := uint64(fsReadFile | fsWriteFile)
	if policy.ABI >= 3 {
		fileAccess |= fsTruncate
	}
	for _, file := range policy.Files {
		if err := addPath(file, fileAccess); err != nil {
			return err
		}
	}
	if _, _, errno := syscall.RawSyscall6(syscall.SYS_PRCTL, prSetNoNewPriv, 1, 0, 0, 0, 0); errno != 0 {
		return fmt.Errorf("no_new_privs: %v", errno)
	}
	if _, _, errno := syscall.Syscall(landlockRestrictSelf, fd, 0, 0); errno != 0 {
		return fmt.Errorf("restrict: %v", errno)
	}
	return nil
}

func confinementFailed(exit int, stderr []byte) (string, bool) {
	line, _, _ := strings.Cut(string(stderr), "\n")
	if (exit == 125 || exit == 126) && strings.HasPrefix(line, "greenhouse confinement:") {
		return "OS confinement could not be applied; the agent did not run: " + strings.TrimPrefix(line, "greenhouse confinement: "), true
	}
	return "", false
}

func confinementLine(c *Confinement) string {
	head := fmt.Sprintf("Confinement: %s %s enforced (ABI %d)", c.Role, c.Mechanism, c.ABI)
	if c.Status != "enforced" {
		head = fmt.Sprintf("Confinement: %s DEGRADED: %s", c.Role, c.Reason)
	}
	lines := []string{head}
	if len(c.Writable) == 0 {
		lines = append(lines, "  Writable: none")
	}
	for _, path := range c.Writable {
		lines = append(lines, "  Writable: "+path)
	}
	network := c.Network
	if len(c.Ports) > 0 {
		var ports []string
		for _, p := range c.Ports {
			ports = append(ports, strconv.Itoa(p))
		}
		network = "TCP connect only to port " + strings.Join(ports, ", ")
	}
	return strings.Join(append(lines, "  Network: "+network), "\n")
}

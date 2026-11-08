package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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
	Files     []string `json:"writable_files,omitempty"`
	Ports     []int    `json:"tcp_connect_ports,omitempty"`
	Network   string   `json:"network"`
	Reason    string   `json:"reason,omitempty"`
}

type confinePolicy struct {
	ABI      int      `json:"abi"`
	Writable []string `json:"writable"`
	Files    []string `json:"files"`
	Ports    []int    `json:"ports"`
}

const (
	landlockCreateRuleset = 444
	landlockAddRule       = 445
	landlockRestrictSelf  = 446

	fsExecute    = 1 << 0
	fsWriteFile  = 1 << 1
	fsReadFile   = 1 << 2
	fsReadDir    = 1 << 3
	fsRefer      = 1 << 13
	fsTruncate   = 1 << 14
	netBindTCP   = 1 << 0
	netConnectTC = 1 << 1

	oPath          = 0x200000
	prSetNoNewPriv = 38
)

func landlockABI() (int, error) {
	if runtime.GOOS != "linux" {
		return 0, errors.New("OS confinement is implemented only on Linux")
	}
	abi, _, errno := syscall.Syscall(landlockCreateRuleset, 0, 0, 1)
	if errno != 0 {
		return 0, fmt.Errorf("Landlock unavailable: %v", errno)
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

func agentPorts(config ReviewerConfig) []int {
	set := map[int]bool{}
	add := func(raw string) {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return
		}
		port := u.Port()
		if port == "" {
			port = map[string]string{"http": "80", "https": "443", "socks5": "1080", "socks5h": "1080"}[u.Scheme]
		}
		if n, err := strconv.Atoi(port); err == nil && n > 0 && n < 65536 {
			set[n] = true
		}
	}
	add(config.Provider.BaseURL)
	if config.Backend == backendClaude || config.Backend == backendCodex {
		set[443] = true
	}
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		add(os.Getenv(key))
	}
	var ports []int
	for p := range set {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports
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
	policy := &confinePolicy{ABI: abi, Writable: c.Writable, Files: append([]string{"/dev/null"}, c.Files...)}
	c.Status = "enforced"
	if degraded != "" {
		c.Status, c.Reason = "degraded", degraded
	}
	if abi < 4 {
		c.Status, c.Network = "degraded", "unrestricted"
		c.Reason = strings.TrimPrefix(c.Reason+fmt.Sprintf("; Landlock ABI %d cannot restrict TCP; filesystem writes are confined, network is not", abi), "; ")
	} else {
		policy.Ports = agentPorts(config)
		c.Ports = policy.Ports
		c.Network = "TCP connect only to the listed ports; no listening sockets"
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
		attr = binary.LittleEndian.AppendUint64(attr, netBindTCP|netConnectTC)
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
	fileAccess := uint64(fsReadFile | fsWriteFile)
	if policy.ABI >= 3 {
		fileAccess |= fsTruncate
	}
	for _, file := range policy.Files {
		if err := addPath(file, fileAccess); err != nil {
			return err
		}
	}
	if policy.ABI >= 4 {
		for _, port := range policy.Ports {
			rule := make([]byte, 16)
			binary.LittleEndian.PutUint64(rule, netConnectTC)
			binary.LittleEndian.PutUint64(rule[8:], uint64(port))
			if _, _, errno := syscall.Syscall6(landlockAddRule, fd, 2, uintptr(unsafe.Pointer(&rule[0])), 0, 0, 0); errno != 0 {
				return fmt.Errorf("port rule %d: %v", port, errno)
			}
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
	if len(c.Writable)+len(c.Files) == 0 {
		lines = append(lines, "  Writable: none")
	}
	for _, path := range c.Writable {
		lines = append(lines, "  Writable: "+path)
	}
	for _, path := range c.Files {
		lines = append(lines, "  Writable file: "+path)
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

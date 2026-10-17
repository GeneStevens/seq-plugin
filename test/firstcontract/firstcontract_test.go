// Package firstcontract plays a first contract in Thornwick the way Claude Code
// would: it starts a local SEQ server on the seq-world Thornwick packs, runs the
// plugin's own seq-mcp command (read from mcp/seq.json, built from source at the
// pinned seq commit) over stdio, and calls the tools.
package firstcontract

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GeneStevens/seq/testserver"
)

type obj = map[string]any

func repoRoot(t *testing.T) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// pins reads scripts/pin.env.
func pins(t *testing.T, root string) map[string]string {
	b, err := os.ReadFile(filepath.Join(root, "scripts", "pin.env"))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok && !strings.HasPrefix(k, "#") {
			m[k] = v
		}
	}
	return m
}

func run(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, errb.String())
	}
	return strings.TrimSpace(out.String())
}

// worldPacks is the seq-world checkout at the pinned commit, from the module cache.
func worldPacks(t *testing.T, root string, pin map[string]string) string {
	out := run(t, root, nil, "go", "mod", "download", "-json", pin["SEQ_WORLD_MODULE"]+"@"+pin["SEQ_WORLD_COMMIT"])
	var m struct{ Dir string }
	if err := json.Unmarshal([]byte(out), &m); err != nil || m.Dir == "" {
		t.Fatalf("go mod download: %v: %s", err, out)
	}
	return filepath.Join(m.Dir, "packs")
}

// mcp is a stdio MCP client on the plugin's command.
type mcp struct {
	t     *testing.T
	s     *testserver.Server
	in    io.WriteCloser
	out   *bufio.Reader
	id    int
	log   *strings.Builder
	tools []string
}

func (c *mcp) rpc(method string, params any) json.RawMessage {
	c.t.Helper()
	c.id++
	req, _ := json.Marshal(obj{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	// The server's clock is the test's: game time moves while a call is open.
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				c.s.Advance(200 * time.Millisecond)
				time.Sleep(time.Millisecond)
			}
		}
	}()
	defer func() { close(stop); <-done }()
	if _, err := c.in.Write(append(req, '\n')); err != nil {
		c.t.Fatal(err)
	}
	line, err := c.out.ReadBytes('\n')
	if err != nil {
		c.t.Fatalf("%s: %v", method, err)
	}
	var resp struct {
		Result json.RawMessage
		Error  *struct{ Message string }
	}
	if err := json.Unmarshal(line, &resp); err != nil || resp.Error != nil {
		c.t.Fatalf("%s: bad response %s (%v)", method, line, err)
	}
	return resp.Result
}

// call runs a tool and fails the test on an error answer or a missing phrase.
func (c *mcp) call(name string, args obj, phrases ...string) string {
	c.t.Helper()
	var res struct {
		Content []struct{ Text string }
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(c.rpc("tools/call", obj{"name": name, "arguments": args}), &res); err != nil || len(res.Content) == 0 {
		c.t.Fatalf("%s: no answer (%v)", name, err)
	}
	text := strings.TrimSpace(res.Content[0].Text)
	a, _ := json.Marshal(args)
	fmt.Fprintf(c.log, "> %s %s\n%s\n\n", name, a, text)
	if res.IsError {
		c.t.Fatalf("%s %s: an error: %s", name, a, text)
	}
	for _, p := range phrases {
		if !strings.Contains(text, p) {
			c.t.Fatalf("%s %s lacks %q:\n%s", name, a, p, text)
		}
	}
	return text
}

// serverCert reads the self-signed certificate the test server presents, as a
// player would be given it, so the adapter can be told to trust it.
func serverCert(t *testing.T, url string) []byte {
	t.Helper()
	conn, err := tls.Dial("tcp", strings.TrimPrefix(strings.SplitN(url, "/", 4)[2], ""), &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // only to fetch the certificate
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	return conn.ConnectionState().PeerCertificates[0].Raw
}

func TestFirstContractInThornwick(t *testing.T) {
	began := time.Now()
	root := repoRoot(t)
	pin := pins(t, root)

	// The plugin builds seq-mcp from source at the pinned commit.
	binDir := t.TempDir()
	built := run(t, root, []string{"SEQ_PLUGIN_BIN_DIR=" + binDir}, filepath.Join(root, "scripts", "install-seq-mcp.sh"))
	t.Logf("seq-mcp built at %s in %v", pin["SEQ_COMMIT"][:12], time.Since(began).Round(time.Millisecond))
	if !strings.HasPrefix(built, binDir) {
		t.Fatalf("the install script wrote %q, not under %s", built, binDir)
	}

	s := testserver.Start(t, testserver.Options{
		Packs:    []string{worldPacks(t, root, pin)},
		Zones:    []string{"thornwick", "thornwick-downs"},
		Accounts: []testserver.Account{{Name: "ann", Characters: []string{"c1"}}},
	})

	// What a player holds: a credential file, and the server's certificate.
	dir := t.TempDir()
	credFile := filepath.Join(dir, "credential")
	if err := os.WriteFile(credFile, []byte(s.AgentSecret("ann", "c1")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cert := serverCert(t, s.URL())
	caFile := filepath.Join(dir, "server.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0o600); err != nil {
		t.Fatal(err)
	}

	// The command and environment exactly as mcp/seq.json gives them, with the
	// substitutions Claude Code makes for the plugin root and the user's options.
	raw, err := os.ReadFile(filepath.Join(root, "mcp", "seq.json"))
	if err != nil {
		t.Fatal(err)
	}
	subst := strings.NewReplacer(
		"${CLAUDE_PLUGIN_ROOT}", root,
		"${user_config.server_url}", s.URL(),
		"${user_config.credential_file}", credFile,
		"${user_config.account}", "",
		"${user_config.ca_file}", caFile,
	)
	var cfg struct {
		MCPServers map[string]struct {
			Command string
			Args    []string
			Env     map[string]string
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(subst.Replace(string(raw))), &cfg); err != nil || cfg.MCPServers["seq"].Command == "" {
		t.Fatalf("mcp/seq.json: %v", err)
	}
	srv := cfg.MCPServers["seq"]
	if strings.Contains(string(raw), "SEQ_AGENT_CREDENTIAL") || strings.Contains(string(raw), "seq_ag_") {
		t.Fatal("mcp/seq.json must not hold a credential")
	}
	cmd := exec.Command(srv.Command, srv.Args...)
	cmd.Env = append(os.Environ(), "SEQ_PLUGIN_BIN_DIR="+binDir)
	for k, v := range srv.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { in.Close(); _ = cmd.Wait() }) })

	c := &mcp{t: t, s: s, in: in, out: bufio.NewReaderSize(out, 1<<20), log: &strings.Builder{}}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("transcript so far:\n%s\nseq-mcp stderr:\n%s", c.log, stderr.String())
		}
	})
	c.rpc("initialize", obj{"protocolVersion": "2025-06-18", "capabilities": obj{}, "clientInfo": obj{"name": "first-contract-test", "version": "1"}})
	if _, err := in.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	var list struct{ Tools []struct{ Name string } }
	if err := json.Unmarshal(c.rpc("tools/list", obj{}), &list); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, tl := range list.Tools {
		have[tl.Name] = true
		c.tools = append(c.tools, tl.Name)
	}
	for _, want := range []string{"new_character", "look", "go_to", "contracts", "take_contract", "use_door", "turn_in", "release"} {
		if !have[want] {
			t.Fatalf("tools/list lacks %s: %v", want, c.tools)
		}
	}
	fmt.Fprintf(c.log, "tools/list: %s\n\n", strings.Join(c.tools, ", "))

	// A new character walks to the board, takes the scout job, does it, hands it in.
	c.call("new_character", obj{"name": "Aria", "class": "warrior"}, "Made Aria", "level 1 warrior", "Thornwick")
	c.call("contracts", obj{}, "You hold no contracts", "Word from Wickett Mill")
	c.call("go_to", obj{"place": "the contract board"}, "Contract Board")
	c.call("take_contract", obj{"contract": "Word from Wickett Mill"}, "You take Word from Wickett Mill")
	c.call("use_door", obj{"door": "east-gate"}, "Thornwick Downs", "scout 0/1")
	c.call("go_to", obj{"place": "wickett-mill"}, "Wickett Mill")
	c.call("contracts", obj{}, "done: turn it in")
	c.call("use_door", obj{"door": "town-gate"}, "Thornwick")
	c.call("turn_in", obj{}, "You hand in Word from Wickett Mill", "20 xp", "8 crowns")
	look := c.call("look", obj{}, "warrior")
	if strings.Contains(look, "Contract:") {
		t.Errorf("the handed-in contract is still shown:\n%s", look)
	}
	if got, ok := s.Character("aria"); !ok || got.Entity.Crowns < 8 {
		t.Errorf("the server's Aria has %d crowns, want at least 8", got.Entity.Crowns)
	}
	c.call("logout", obj{})

	once.Do(func() { in.Close(); _ = cmd.Wait() })
	took := time.Since(began)
	if p := os.Getenv("FIRSTCONTRACT_TRANSCRIPT"); p != "" {
		_ = os.WriteFile(p, []byte(c.log.String()), 0o644)
	}
	t.Logf("transcript:\n%s", c.log)
	t.Logf("first contract: %v", took.Round(time.Millisecond))
	if took > 30*time.Second {
		t.Errorf("took %v, over the 30 s budget", took)
	}
}

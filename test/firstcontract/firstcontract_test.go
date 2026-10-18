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

// checkSessionLog expects one session directory in logs, with record.jsonl,
// tools.jsonl and session.json, all naming the same session id, and none of the
// credential in any file.
func checkSessionLog(t *testing.T, logs, secret string) {
	t.Helper()
	dirs, err := os.ReadDir(logs)
	if err != nil || len(dirs) != 1 || !dirs[0].IsDir() {
		t.Fatalf("want one session directory under %s, got %v (%v)", logs, dirs, err)
	}
	sd := filepath.Join(logs, dirs[0].Name())
	var meta struct{ Session string }
	for _, name := range []string{"record.jsonl", "tools.jsonl", "session.json"} {
		b, err := os.ReadFile(filepath.Join(sd, name))
		if err != nil || len(b) == 0 {
			t.Fatalf("session log %s: %v (%d bytes)", name, err, len(b))
		}
		if bytes.Contains(b, []byte(secret)) || bytes.Contains(b, []byte("seq_ag_")) || bytes.Contains(b, []byte("seq_st_")) {
			t.Errorf("%s holds a credential", name)
		}
		if name == "session.json" {
			if err := json.Unmarshal(b, &meta); err != nil || meta.Session == "" {
				t.Fatalf("session.json: no session id (%v): %s", err, b)
			}
		}
	}
	if !strings.HasSuffix(dirs[0].Name(), "-"+meta.Session) {
		t.Errorf("directory %s is not named for session %s", dirs[0].Name(), meta.Session)
	}
	for _, name := range []string{"record.jsonl", "tools.jsonl", "session.json"} {
		b, _ := os.ReadFile(filepath.Join(sd, name))
		if !bytes.Contains(b, []byte(meta.Session)) {
			t.Errorf("%s does not name session %s", name, meta.Session)
		}
	}
	t.Logf("session log %s: record.jsonl, tools.jsonl, session.json, session %s", dirs[0].Name(), meta.Session)
}

// TestLauncherLogLocation: the launcher turns logging on by default per client
// and a user setting turns it off.
func TestLauncherLogLocation(t *testing.T) {
	root := repoRoot(t)
	pin := pins(t, root)
	bin := t.TempDir()
	fake := filepath.Join(bin, "seq-mcp-"+pin["SEQ_COMMIT"])
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho \"log=${SEQ_LOG_DIR-unset}\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	launch := func(env ...string) string {
		return run(t, root, append([]string{"SEQ_PLUGIN_BIN_DIR=" + bin, "SEQ_SERVER_URL=wss://x/seq/v1", "SEQ_CREDENTIAL_FILE=/dev/null", "SEQ_LOG_DIR=", "SEQ_LOGGING=", "SEQ_CLIENT=", "CLAUDE_PLUGIN_DATA=", "XDG_DATA_HOME=", "HOME=/h"}, env...), filepath.Join(root, "scripts", "seq-mcp-run.sh"))
	}
	for _, c := range []struct {
		name string
		env  []string
		want string
	}{
		{"claude", []string{"CLAUDE_PLUGIN_DATA=/d"}, "log=/d/logs"},
		{"claude named", []string{"CLAUDE_PLUGIN_DATA=/d", "SEQ_CLIENT=claude"}, "log=/d/logs"},
		{"codex", []string{"SEQ_CLIENT=codex"}, "log=/h/.local/share/seq/logs/codex"},
		{"gemini", []string{"SEQ_CLIENT=gemini"}, "log=/h/.local/share/seq/logs/gemini"},
		{"unknown client", []string{"SEQ_CLIENT=zed"}, "log=/h/.local/share/seq/logs/other"},
		{"no client", nil, "log=/h/.local/share/seq/logs/other"},
		{"xdg", []string{"SEQ_CLIENT=codex", "XDG_DATA_HOME=/x"}, "log=/x/seq/logs/codex"},
		{"user directory", []string{"CLAUDE_PLUGIN_DATA=/d", "SEQ_LOG_DIR=/mine"}, "log=/mine"},
		{"user directory codex", []string{"SEQ_CLIENT=codex", "SEQ_LOG_DIR=/mine"}, "log=/mine"},
		{"logging true", []string{"CLAUDE_PLUGIN_DATA=/d", "SEQ_LOGGING=true"}, "log=/d/logs"},
		{"logging off claude", []string{"CLAUDE_PLUGIN_DATA=/d", "SEQ_LOGGING=off"}, "log=unset"},
		{"logging false claude", []string{"CLAUDE_PLUGIN_DATA=/d", "SEQ_LOGGING=false"}, "log=unset"},
		{"logging 0 codex", []string{"SEQ_CLIENT=codex", "SEQ_LOGGING=0"}, "log=unset"},
		{"logging off gemini", []string{"SEQ_CLIENT=gemini", "SEQ_LOGGING=off"}, "log=unset"},
		{"logging off beats directory", []string{"SEQ_LOGGING=off", "SEQ_LOG_DIR=/mine"}, "log=unset"},
		{"legacy off", []string{"CLAUDE_PLUGIN_DATA=/d", "SEQ_LOG_DIR=off"}, "log=unset"},
		{"legacy off codex", []string{"SEQ_CLIENT=codex", "SEQ_LOG_DIR=off"}, "log=unset"},
	} {
		if got := launch(c.env...); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// startPlugin starts a local server on the pinned Thornwick packs and the
// plugin's seq-mcp over stdio, initialised and ready for tool calls. It returns
// the client, the plugin's data directory, the credential and a stop function.
func startPlugin(t *testing.T, began time.Time) (*mcp, string, string, func()) {
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
	secret := s.AgentSecret("ann", "c1")
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
		"${user_config.log_dir}", "",
		"${user_config.logging}", "true",
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
	// Claude Code gives the plugin a data directory; logging is on by default
	// there, and the test sets no log option.
	dataDir := t.TempDir()
	cmd.Env = append(os.Environ(), "SEQ_PLUGIN_BIN_DIR="+binDir, "CLAUDE_PLUGIN_DATA="+dataDir, "SEQ_LOG_DIR=", "SEQ_CLIENT=")
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
	return c, dataDir, string(secret), func() { once.Do(func() { in.Close(); _ = cmd.Wait() }) }
}

func TestFirstContractInThornwick(t *testing.T) {
	began := time.Now()
	c, dataDir, secret, stop := startPlugin(t, began)

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
	if got, ok := c.s.Character("aria"); !ok || got.Entity.Crowns < 8 {
		t.Errorf("the server's Aria has %d crowns, want at least 8", got.Entity.Crowns)
	}
	c.call("logout", obj{})

	stop()
	checkSessionLog(t, filepath.Join(dataDir, "logs"), secret)
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

// TestFirstStepsInThornwick: a new character is offered First Steps and walks
// it as the play guide describes: greeter, own trainer (15 crowns), any board,
// then through the east gate, which pays 20 xp on crossing (D62, D68).
func TestFirstStepsInThornwick(t *testing.T) {
	c, _, _, stop := startPlugin(t, time.Now())
	defer stop()
	c.call("new_character", obj{"name": "Bram", "class": "warrior"}, "Made Bram", "Thornwick")
	c.call("look", obj{}, "Quest: First Steps, step 1 of 4")
	c.call("quests", obj{}, "First Steps, step 1 of 4", "talk_to", "nothing in the game waits for it")
	c.call("go_to", obj{"place": "the greeter"})
	c.call("talk_to", obj{"npc": "the greeter"})
	c.call("quests", obj{}, "step 2 of 4", "15 crowns")
	c.call("go_to", obj{"place": "Gorran Stoutarm"})
	c.call("talk_to", obj{"npc": "Gorran Stoutarm"})
	c.call("quests", obj{}, "step 3 of 4", "take_contract")
	if got, _ := c.s.Character("bram"); got.Entity.Crowns < 15 {
		t.Errorf("Bram has %d crowns after the trainer step, want at least 15", got.Entity.Crowns)
	}
	c.call("go_to", obj{"place": "the contract board"}, "Contract Board")
	c.call("take_contract", obj{"contract": "Word from Wickett Mill"}, "You take Word from Wickett Mill")
	c.call("quests", obj{}, "step 4 of 4", "East Gate")
	c.call("go_to", obj{"place": "east-gate"})
	c.call("quests", obj{}, "step 4 of 4")
	if got, _ := c.s.Character("bram"); got.Entity.XP != 0 {
		t.Errorf("Bram has %d xp on reaching the gate, want 0 until he crosses", got.Entity.XP)
	}
	c.call("use_door", obj{"door": "east-gate"}, "Thornwick Downs")
	c.call("quests", obj{}, "no quest chain")
	if got, _ := c.s.Character("bram"); got.Entity.XP < 20 {
		t.Errorf("Bram has %d xp after crossing, want at least 20", got.Entity.XP)
	}
}

// TestDismissQuest: dismiss_quest ends the chain for good.
func TestDismissQuest(t *testing.T) {
	c, _, _, stop := startPlugin(t, time.Now())
	defer stop()
	c.call("new_character", obj{"name": "Cale", "class": "warrior"}, "Made Cale")
	c.call("quests", obj{}, "First Steps")
	c.call("dismiss_quest", obj{})
	c.call("quests", obj{}, "no quest chain")
	if look := c.call("look", obj{}); strings.Contains(look, "Quest:") {
		t.Errorf("a dismissed chain is still shown:\n%s", look)
	}
}

// TestLauncherCredentialFileNames: the credential file may be given as
// SEQ_ACCOUNT_FILE (a name Gemini CLI does not strip) or SEQ_CREDENTIAL_FILE.
func TestLauncherCredentialFileNames(t *testing.T) {
	root := repoRoot(t)
	pin := pins(t, root)
	bin := t.TempDir()
	fake := filepath.Join(bin, "seq-mcp-"+pin["SEQ_COMMIT"])
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho \"cred=${SEQ_CREDENTIAL_FILE-unset} account_file=${SEQ_ACCOUNT_FILE-unset}\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cred := filepath.Join(t.TempDir(), "credential")
	other := filepath.Join(t.TempDir(), "other")
	for _, p := range []string{cred, other} {
		if err := os.WriteFile(p, []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name string
		env  []string
		want string
	}{
		{"new name", []string{"SEQ_ACCOUNT_FILE=" + cred}, "cred=" + cred + " account_file=unset"},
		{"old name", []string{"SEQ_CREDENTIAL_FILE=" + cred}, "cred=" + cred + " account_file=unset"},
		{"old name wins", []string{"SEQ_ACCOUNT_FILE=" + other, "SEQ_CREDENTIAL_FILE=" + cred}, "cred=" + cred + " account_file=unset"},
		{"empty old name", []string{"SEQ_ACCOUNT_FILE=" + cred, "SEQ_CREDENTIAL_FILE="}, "cred=" + cred + " account_file=unset"},
	} {
		env := append([]string{"SEQ_PLUGIN_BIN_DIR=" + bin, "SEQ_SERVER_URL=wss://x/seq/v1", "SEQ_ACCOUNT_FILE=", "SEQ_CREDENTIAL_FILE=", "SEQ_AGENT_CREDENTIAL="}, c.env...)
		if got := run(t, root, env, filepath.Join(root, "scripts", "seq-mcp-run.sh")); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// TestLauncherNamesWhatIsMissing: with no server URL or no credential, the first
// tool call, in any client, is answered with exactly which setting is missing and how
// to set it for that client. The old message named SEQ_AGENT_CREDENTIAL, which a
// player never sets.
func TestLauncherNamesWhatIsMissing(t *testing.T) {
	root := repoRoot(t)
	cred := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(cred, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		client string
		env    []string
		want   []string
	}{
		{"claude no url", "", []string{"SEQ_CREDENTIAL_FILE=" + cred}, []string{"SEQ_SERVER_URL", "/plugin configure seq@seq"}},
		{"claude nothing", "", nil, []string{"SEQ_SERVER_URL", "SEQ_ACCOUNT_FILE", "/plugin configure seq@seq"}},
		{"codex no credential", "codex", []string{"SEQ_SERVER_URL=wss://x/seq/v1"}, []string{"SEQ_ACCOUNT_FILE", "codex-install.sh"}},
		{"gemini no credential", "gemini", []string{"SEQ_SERVER_URL=wss://x/seq/v1"}, []string{"SEQ_ACCOUNT_FILE", "gemini extensions config seq"}},
		{"gemini no url", "gemini", []string{"SEQ_ACCOUNT_FILE=" + cred}, []string{"SEQ_SERVER_URL", "gemini extensions config seq"}},
		{"unreadable credential file", "codex", []string{"SEQ_SERVER_URL=wss://x/seq/v1", "SEQ_CREDENTIAL_FILE=/no/such/file"}, []string{"/no/such/file", "credential file"}},
	} {
		cmd := exec.Command(filepath.Join(root, "scripts", "seq-mcp-run.sh"))
		cmd.Env = append(os.Environ(), "SEQ_SERVER_URL=", "SEQ_ACCOUNT_FILE=", "SEQ_CREDENTIAL_FILE=", "SEQ_AGENT_CREDENTIAL=", "SEQ_CLIENT="+c.client)
		cmd.Env = append(cmd.Env, c.env...)
		cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"look","arguments":{}}}` + "\n")
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			t.Errorf("%s: %v\n%s", c.name, err, errb.String())
			continue
		}
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		if len(lines) != 3 {
			t.Errorf("%s: %d answers, want 3:\n%s", c.name, len(lines), out.String())
			continue
		}
		var res struct {
			ID     int
			Result struct {
				IsError bool
				Content []struct{ Text string }
			}
		}
		if err := json.Unmarshal([]byte(lines[2]), &res); err != nil || res.ID != 3 || !res.Result.IsError || len(res.Result.Content) == 0 {
			t.Errorf("%s: the tool call was not answered with an error: %s (%v)", c.name, lines[2], err)
			continue
		}
		text := res.Result.Content[0].Text
		for _, w := range c.want {
			if !strings.Contains(text, w) {
				t.Errorf("%s: the message lacks %q: %s", c.name, w, text)
			}
		}
		if strings.Contains(text, "SEQ_AGENT_CREDENTIAL") {
			t.Errorf("%s: the message names SEQ_AGENT_CREDENTIAL: %s", c.name, text)
		}
		if !strings.Contains(errb.String(), "SEQ is not set up") {
			t.Errorf("%s: nothing on stderr: %q", c.name, errb.String())
		}
	}
}

// Package clients makes a real game call through each installed client's own
// configuration. scripts/check-clients.sh runs it (SEQ_CHECK_CLIENTS=1); a plain
// go test skips it.
//
// For each client that is installed it installs this repository into a throwaway
// config directory against a local SEQ server on the Thornwick packs, takes the
// command and environment the client would start the seq server with, starts it
// and calls new_character (or play_as), look and logout over stdio, expecting a game answer ("You are
// ...").
//
//   - Codex CLI: `codex mcp get seq --json` gives the command and environment.
//   - Gemini CLI: Gemini itself starts the server (`gemini mcp list`) with
//     SEQ_LAUNCH_REPORT set, and the launcher writes the SEQ_ settings that
//     reached it. The test then starts the launcher with exactly that environment.
//     So Gemini's own redaction of names like CREDENTIAL decides what the server
//     gets. The old setup, the credential in SEQ_CREDENTIAL_FILE only, must fail.
package clients

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
	"testing"
	"time"

	"github.com/GeneStevens/seq/testserver"
)

type obj = map[string]any

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func run(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v: %v\n%s%s", name, args, err, out.String(), errb.String())
	}
	return strings.TrimSpace(out.String())
}

func pinVar(t *testing.T, root, key string) string {
	b, err := os.ReadFile(filepath.Join(root, "scripts", "pin.env"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok && k == key {
			return v
		}
	}
	t.Fatalf("pin.env has no %s", key)
	return ""
}

type fixture struct {
	srv      *testserver.Server
	url      string
	credFile string
	caFile   string
	bin      string // SEQ_PLUGIN_BIN_DIR
	made     bool   // the character exists: later calls play it
}

func start(t *testing.T, root string) *fixture {
	t.Helper()
	bin := t.TempDir()
	run(t, root, []string{"SEQ_PLUGIN_BIN_DIR=" + bin}, filepath.Join(root, "scripts", "install-seq-mcp.sh"))
	out := run(t, root, nil, "go", "mod", "download", "-json", pinVar(t, root, "SEQ_WORLD_MODULE")+"@"+pinVar(t, root, "SEQ_WORLD_COMMIT"))
	var m struct{ Dir string }
	if err := json.Unmarshal([]byte(out), &m); err != nil || m.Dir == "" {
		t.Fatalf("go mod download: %v: %s", err, out)
	}
	s := testserver.Start(t, testserver.Options{
		Packs:    []string{filepath.Join(m.Dir, "packs")},
		Zones:    []string{"thornwick", "thornwick-downs"},
		Accounts: []testserver.Account{{Name: "ann", Characters: []string{"c1"}}},
	})
	dir := t.TempDir()
	f := &fixture{srv: s, url: s.URL(), credFile: filepath.Join(dir, "credential"), caFile: filepath.Join(dir, "server.pem"), bin: bin}
	if err := os.WriteFile(f.credFile, []byte(s.AgentSecret("ann", "c1")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	conn, err := tls.Dial("tcp", strings.SplitN(f.url, "/", 4)[2], &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // only to fetch the test server's certificate
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: conn.ConnectionState().PeerCertificates[0].Raw})
	if err := os.WriteFile(f.caFile, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

// call starts command with env (nothing else of this process's environment but
// PATH and HOME), initializes MCP and calls the tools in order. It returns the
// text and error flag of each answer.
func (f *fixture) call(t *testing.T, command string, args, env []string, tools ...string) (texts []string, isErr []bool) {
	t.Helper()
	cmd := exec.Command(command, args...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "SEQ_PLUGIN_BIN_DIR=" + f.bin, "XDG_DATA_HOME=" + t.TempDir()}, env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	in, _ := cmd.StdinPipe()
	outp, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { in.Close(); _ = cmd.Wait() }()
	r := bufio.NewReaderSize(outp, 1<<20)
	id := 0
	rpc := func(method string, params any) json.RawMessage {
		id++
		req, _ := json.Marshal(obj{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		stop, done := make(chan struct{}), make(chan struct{})
		go func() { // the server's clock is the test's
			defer close(done)
			for {
				select {
				case <-stop:
					return
				default:
					f.srv.Advance(200 * time.Millisecond)
					time.Sleep(time.Millisecond)
				}
			}
		}()
		defer func() { close(stop); <-done }()
		if _, err := in.Write(append(req, '\n')); err != nil {
			t.Fatal(err)
		}
		line, err := r.ReadBytes('\n')
		if err != nil {
			t.Fatalf("%s: %v\nstderr: %s", method, err, stderr.String())
		}
		var resp struct {
			Result json.RawMessage
			Error  *struct{ Message string }
		}
		if err := json.Unmarshal(line, &resp); err != nil || resp.Error != nil {
			t.Fatalf("%s: bad response %s (%v)", method, line, err)
		}
		return resp.Result
	}
	rpc("initialize", obj{"protocolVersion": "2025-06-18", "capabilities": obj{}, "clientInfo": obj{"name": "check-clients", "version": "1"}})
	if _, err := io.WriteString(in, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		args := obj{}
		switch tool {
		case "new_character":
			args = obj{"name": "Aria", "class": "warrior"}
		case "play_as":
			args = obj{"character": "Aria"}
		}
		var res struct {
			Content []struct{ Text string }
			IsError bool `json:"isError"`
		}
		if err := json.Unmarshal(rpc("tools/call", obj{"name": tool, "arguments": args}), &res); err != nil || len(res.Content) == 0 {
			t.Fatalf("%s: no answer (%v)", tool, err)
		}
		texts = append(texts, strings.TrimSpace(res.Content[0].Text))
		isErr = append(isErr, res.IsError)
	}
	return texts, isErr
}

// plays expects a game answer through the given command and environment.
func (f *fixture) plays(t *testing.T, what, command string, args, env []string) {
	t.Helper()
	enter := "play_as"
	if !f.made {
		enter, f.made = "new_character", true
	}
	// Logging out frees the account's place for the next call.
	tools := []string{enter, "look", "logout"}
	texts, isErr := f.call(t, command, args, env, tools...)
	for i, name := range tools {
		if isErr[i] {
			t.Fatalf("%s: %s answered with an error: %s", what, name, texts[i])
		}
	}
	if !strings.HasPrefix(texts[1], "You are ") {
		t.Fatalf("%s: not a game answer:\n%s\n%s", what, texts[0], texts[1])
	}
	t.Logf("%s: look answered %q", what, strings.SplitN(texts[1], "\n", 2)[0])
}

func TestClientsMakeARealGameCall(t *testing.T) {
	if os.Getenv("SEQ_CHECK_CLIENTS") != "1" {
		t.Skip("run by scripts/check-clients.sh")
	}
	root := repoRoot()
	_, haveCodex := exec.LookPath("codex")
	_, haveGemini := exec.LookPath("gemini")
	if haveCodex != nil && haveGemini != nil {
		t.Skip("neither codex nor gemini is installed")
	}
	f := start(t, root)
	if haveCodex == nil {
		t.Run("codex", func(t *testing.T) { f.codex(t, root) })
	} else {
		t.Log("codex not installed: skipping its call")
	}
	if haveGemini == nil {
		t.Run("gemini", func(t *testing.T) { f.gemini(t, root) })
	} else {
		t.Log("gemini not installed: skipping its call")
	}
}

func (f *fixture) codex(t *testing.T, root string) {
	home := t.TempDir()
	run(t, root, []string{"CODEX_HOME=" + home, "SEQ_SERVER_URL=" + f.url, "SEQ_CREDENTIAL_FILE=" + f.credFile, "SEQ_CA_FILE=" + f.caFile, "SEQ_ACCOUNT=", "SEQ_LOG_DIR=", "SEQ_LOGGING="}, filepath.Join(root, "scripts", "codex-install.sh"))
	out := run(t, root, []string{"CODEX_HOME=" + home}, "codex", "mcp", "get", "seq", "--json")
	var cfg struct {
		Transport struct {
			Command string
			Args    []string
			Env     map[string]string
		}
	}
	if err := json.Unmarshal([]byte(out), &cfg); err != nil || cfg.Transport.Command == "" {
		t.Fatalf("codex mcp get seq --json: %v\n%s", err, out)
	}
	var env []string
	for k, v := range cfg.Transport.Env {
		env = append(env, k+"="+v)
	}
	f.plays(t, "codex", cfg.Transport.Command, cfg.Transport.Args, env)
}

// prompts answers the extension-settings questions `gemini extensions link` asks,
// each when its text shows up.
func link(t *testing.T, root, home string, answers []string) {
	t.Helper()
	cmd := exec.Command("gemini", "extensions", "link", "--consent", root)
	cmd.Env = append(os.Environ(), "HOME="+home, "GEMINI_CLI_HOME="+home)
	in, _ := cmd.StdinPipe()
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait(); pw.Close() }()
	var seen strings.Builder
	var mu = make(chan struct{}, 1)
	go func() {
		defer close(mu)
		buf := make([]byte, 4096)
		next := 0
		for {
			n, err := pr.Read(buf)
			seen.Write(buf[:n])
			for next < len(answers) && strings.Count(seen.String(), "›") > next {
				time.Sleep(150 * time.Millisecond)
				_, _ = io.WriteString(in, answers[next]+"\r")
				next++
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case err := <-done:
		<-mu
		if err != nil || !strings.Contains(seen.String(), "linked successfully") {
			t.Fatalf("gemini extensions link: %v\n%s", err, seen.String())
		}
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("gemini extensions link timed out:\n%s", seen.String())
	}
}

// The environment a gemini process is started with. Anything in extra is set in
// the shell that starts gemini.
func geminiEnv(home string, extra ...string) []string {
	return append([]string{"HOME=" + home, "GEMINI_CLI_HOME=" + home}, extra...)
}

// report has gemini start the extension's server and returns the SEQ_ settings
// the launcher received, as KEY=VALUE lines.
func report(t *testing.T, root, home string, extra ...string) []string {
	t.Helper()
	rep := filepath.Join(t.TempDir(), "report")
	cmd := exec.Command("gemini", "mcp", "list")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), geminiEnv(home, append([]string{"SEQ_LAUNCH_REPORT=" + rep}, extra...)...)...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("gemini mcp list: %v\n%s", err, out.String())
		}
	case <-time.After(90 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("gemini mcp list timed out:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Connected") {
		t.Fatalf("gemini: seq is not connected:\n%s", out.String())
	}
	b, err := os.ReadFile(rep)
	if err != nil {
		t.Fatalf("the launcher wrote no report (gemini did not start it?): %v\n%s", err, out.String())
	}
	var env []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l != "" {
			env = append(env, l)
		}
	}
	return env
}

func (f *fixture) gemini(t *testing.T, root string) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(root)
	if err := os.WriteFile(filepath.Join(home, ".gemini", "trustedFolders.json"), []byte(fmt.Sprintf("{%q:\"TRUST_FOLDER\"}\n", real)), 0o644); err != nil {
		t.Fatal(err)
	}
	// The player answers the extension's four questions at install: URL, credential
	// file, account (none), certificate.
	link(t, root, home, []string{f.url, f.credFile, "", f.caFile})
	cmd := exec.Command("gemini", "extensions", "list")
	cmd.Env = append(os.Environ(), geminiEnv(home)...)
	if list, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(list), "GEMINI.md") || !strings.Contains(string(list), "0.1.0") {
		t.Fatalf("gemini extensions list does not show seq with its context file: %v\n%s", err, list)
	}
	envFile := filepath.Join(home, ".gemini", "extensions", "seq", ".env")
	settings, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("the extension's settings were not stored: %v", err)
	}
	launcher := filepath.Join(real, "scripts", "seq-mcp-run.sh")
	has := func(env []string, key string) bool {
		for _, e := range env {
			if strings.HasPrefix(e, key+"=") {
				return true
			}
		}
		return false
	}

	t.Run("manifest settings", func(t *testing.T) {
		env := report(t, root, home)
		for _, k := range []string{"SEQ_SERVER_URL", "SEQ_CREDENTIAL_FILE", "SEQ_CA_FILE"} {
			if !has(env, k) {
				t.Fatalf("gemini started the server without %s: %v", k, env)
			}
		}
		f.plays(t, "gemini, settings from the manifest", launcher, nil, append(env, "SEQ_CLIENT=gemini"))
	})

	if err := os.Remove(envFile); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.WriteFile(envFile, settings, 0o600) })

	t.Run("exported variables", func(t *testing.T) {
		env := report(t, root, home, "SEQ_SERVER_URL="+f.url, "SEQ_ACCOUNT_FILE="+f.credFile, "SEQ_CA_FILE="+f.caFile)
		if !has(env, "SEQ_CREDENTIAL_FILE") {
			t.Fatalf("SEQ_ACCOUNT_FILE did not reach the server: %v", env)
		}
		f.plays(t, "gemini, exported SEQ_ACCOUNT_FILE", launcher, nil, append(env, "SEQ_CLIENT=gemini"))
	})

	t.Run("the old setup fails", func(t *testing.T) {
		// What the README used to say: export SEQ_CREDENTIAL_FILE. Gemini strips it.
		env := report(t, root, home, "SEQ_SERVER_URL="+f.url, "SEQ_CREDENTIAL_FILE="+f.credFile, "SEQ_CA_FILE="+f.caFile)
		if has(env, "SEQ_CREDENTIAL_FILE") {
			t.Fatalf("Gemini no longer strips SEQ_CREDENTIAL_FILE; this check's reason is gone: %v", env)
		}
		texts, isErr := f.call(t, launcher, nil, append(env, "SEQ_CLIENT=gemini"), "new_character")
		if !isErr[0] || !strings.Contains(texts[0], "SEQ_ACCOUNT_FILE") {
			t.Fatalf("the old setup should fail naming SEQ_ACCOUNT_FILE, got (error=%v): %s", isErr[0], texts[0])
		}
		t.Logf("the old setup fails, as it must: %s", texts[0])
	})
}

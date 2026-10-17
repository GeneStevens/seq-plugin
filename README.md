# seq-plugin

Play SEQ (Spiel Engine Quest) from Claude Code, Codex CLI or Gemini CLI. SEQ is a multiplayer fantasy RPG
with a server that keeps the world; you play by telling your AI client what to do
("make a warrior called Aria, take the wolf hunt, and finish it"), and the
client plays your character through the game's tools. Release 1 is the **Thornwick
alpha**.

**This is an invite-only alpha.** You need a server address and a credential
from the person who invited you. The engine and the world content are private
for now; this repository holds only the plugin: the manifest, the MCP
configuration, a how-to-play skill and a test.

## What you get

- The `seq` MCP server (`seq-mcp`), with tools to `look`, `go_to`, `talk_to`,
  `take_contract`, `attack`, `cast`, `loot`, `turn_in`, `use_door` and
  `release`, and to chat and party up with other players.
- The `play-seq` guide (a skill in Claude Code and Codex, the context file in Gemini CLI), which teaches the model how to play and when to ask you
  first (spending crowns, trading, anything you can't take back).

## Choose your client

SEQ works the same from **Claude Code**, **Codex CLI** and **Gemini CLI**: each
runs the same `seq-mcp` server and teaches the model the same how-to-play guide
(one source, `guide/PLAY.md`). Pick one; every path below is complete.

All three need Go (to build `seq-mcp`), a clone of this repository, and **read
access to the private `seq` repository on GitHub** from this machine: in M10
`seq-mcp` is built from source at a pinned commit (`scripts/pin.env`) on first
start. Prebuilt binaries come later. Check with `git ls-remote
https://github.com/GeneStevens/seq` (or `gh auth status`). The launcher sets
`GOPRIVATE` itself when it builds.

The four settings, for every client:

| Setting | What |
| --- | --- |
| server URL | the server's gateway, like `wss://host:port/seq/v1` (required) |
| credential file | a file only you can read, holding your `seq_ag_...` credential on one line (required) |
| account | only if that file has several `account=credential` lines |
| CA file | a PEM file for the server's certificate, if it uses a self-signed one |

Make the credential file with `umask 077`, and keep it out of any repository.
Nothing in this plugin holds a credential: only the file's path is configured.
No invitation? See "Play on a local server" below.

```sh
git clone https://github.com/GeneStevens/seq-plugin
```

### Claude Code

A plugin installs into the config directory of the `claude` that runs the
install. If you keep several Claude accounts, each with its own
`CLAUDE_CONFIG_DIR`, run every `claude plugin` command under the one you will
play in, or it installs somewhere you never look:

```sh
export CLAUDE_CONFIG_DIR=...        # the account you play in; skip it for the default one
cd ~                                # not inside the checkout (see below)
claude plugin marketplace add /path/to/seq-plugin
claude plugin install seq@seq
claude plugin list                  # seq@seq should be listed, under the same CLAUDE_CONFIG_DIR
```

(Or per command: `env CLAUDE_CONFIG_DIR=... claude plugin install seq@seq`.) If
`claude plugin list` under that directory does not show `seq@seq`, the install
went to another account's directory. Or, inside Claude Code: `/plugin marketplace
add GeneStevens/seq-plugin`, then `/plugin install seq@seq`. To try a checkout
without installing it: `claude --plugin-dir /path/to/seq-plugin`.

Then `/plugin configure seq@seq` (or pass `--config KEY=VALUE` to `claude plugin
install`) sets `server_url`, `credential_file`, `account` and `ca_file`.

Check: `claude mcp list` shows `plugin:seq:seq ... Connected`, and in a session
`/mcp` lists the server's tools. First prompt: *"Play SEQ: make a warrior
called Aria and look around."*

The first start builds `seq-mcp` from source (kept in the plugin's data
directory, one binary per pinned commit), which takes a while; Claude Code may
show the server as failed until it finishes, and `/mcp` then reconnects it.
`scripts/install-seq-mcp.sh` run by hand does not help: it builds into
`~/.cache/seq-plugin`, not the plugin's data directory, so it only warms Go's cache.

### Codex CLI

Codex keeps everything under `CODEX_HOME` (default `~/.codex`). Set it first if
you use another directory. The helper installs the skill (this repository is a
Codex plugin, `plugin.json` with `.agents/plugins/marketplace.json`) and adds the
`seq` MCP server to Codex's `config.toml`, from the four settings as environment
variables:

```sh
export CODEX_HOME=...               # skip it for the default ~/.codex
export SEQ_SERVER_URL=wss://host:port/seq/v1
export SEQ_CREDENTIAL_FILE=~/.seq-credential
# optional: export SEQ_ACCOUNT=...  SEQ_CA_FILE=/path/to/cert.pem
/path/to/seq-plugin/scripts/codex-install.sh
```

That runs `codex plugin marketplace add`, `codex plugin add seq@seq` and `codex
mcp add seq --env ... -- /path/to/seq-plugin/scripts/seq-mcp-run.sh`. Only the
URL and file paths go into `config.toml`, never the credential. The server
entry points at your checkout, so keep the checkout where it is.

Check: `codex mcp list` shows `seq` (enabled), `codex mcp get seq` its command and
settings, and `codex plugin list` shows `seq@seq installed, enabled`. Then start
`codex` and say: *"Play SEQ: make a warrior called Aria and look around."* (the
`play-seq` skill is also reachable as `$play-seq`). Codex starts the server on
first use; the first start builds `seq-mcp`, so if the first call fails, run
`scripts/install-seq-mcp.sh` once and start `codex` again.

### Gemini CLI

Gemini keeps its config in `~/.gemini` (under `HOME`; `GEMINI_CLI_HOME` moves it).
This repository is a Gemini extension (`gemini-extension.json`, with the guide in
`GEMINI.md`). Gemini starts the extension's server with your shell's environment,
so export the four settings where you start `gemini`:

```sh
gemini extensions link /path/to/seq-plugin     # or: gemini extensions install https://github.com/GeneStevens/seq-plugin
export SEQ_SERVER_URL=wss://host:port/seq/v1
export SEQ_CREDENTIAL_FILE=~/.seq-credential
# optional: export SEQ_ACCOUNT=...  SEQ_CA_FILE=/path/to/cert.pem
gemini
```

Add `--consent` to skip the install prompt. Check: `gemini extensions list` shows
`seq` with its context file and server, and `gemini mcp list` (with the variables
exported) shows `seq (from seq) ... Connected`. In a session, `/mcp` lists the
tools. First prompt: *"Play SEQ: make a warrior called Aria and look around."*
Run `scripts/install-seq-mcp.sh` once first so the build does not delay the
connection.

## Your logs

`seq-mcp` keeps a log of every play session on your machine, on by default. One
folder per session, named `<yyyymmdd-hhmmss>-<session id>`, holding:

- `record.jsonl`: every message the server sent, so a session can be replayed;
- `tools.jsonl`: each tool call, its arguments, a short answer, and the ids of the
  intents it sent;
- `session.json`: the session id, the server (without credentials), the character
  and the times.

The session id is the same in all three files and in the server's own log of the
session. Your credential is never written. Each session is capped at 32 MB and the
whole folder at 256 MB (the oldest sessions go first).

| Client | Where the logs are |
| --- | --- |
| Claude Code | the plugin's data folder, `logs/`, under your `CLAUDE_CONFIG_DIR`'s `plugins/data`; `/plugin configure seq@seq` sets another with "Session log directory" |
| Codex CLI | `~/.local/share/seq/logs` (or `$XDG_DATA_HOME/seq/logs`); `SEQ_LOG_DIR` at install time moves it |
| Gemini CLI | the same, `~/.local/share/seq/logs`; export `SEQ_LOG_DIR` where you start `gemini` |

Codex and Gemini give a plugin no data folder of its own, so they share one place.

**Replay one** in the SEQ terminal client: `seq-tui -replay
~/.local/share/seq/logs/<session folder>/record.jsonl` (add `-text` for plain text).

**The server's log of your sessions.** Ask your AI client: *"List my SEQ sessions"*
and *"Show me the log of this session"* or *"Why did I die?"*. It uses the
`my_sessions` and `session_log` tools, which answer only for your own account; the
log has what you asked for, the answers you were given, chat as shown, moves between
zones, errors and the disconnect.

**Share a session for support** by sending the session's folder (the three files)
and its session id, which is in the folder name. Look through them first:
`tools.jsonl` and `record.jsonl` hold what the game said and the chat you saw, and
the answers your AI client was given, which is your conversation with the game.
The credential is not in them. The server's side is found by the session id.

**Turn logging off.** Claude Code: `/plugin configure seq@seq` and type `off` for
the session log directory. Codex CLI: `SEQ_LOG_DIR=off` when you run
`scripts/codex-install.sh`. Gemini CLI: `export SEQ_LOG_DIR=off` where you start
`gemini`. Delete the folder at any time; nothing needs it.

## Update the guide

Edit `guide/PLAY.md` only, then run `scripts/gen-guide.sh`: it rewrites the Claude
Code and Codex skill (`skills/play-seq/SKILL.md`) and Gemini's `GEMINI.md`.
`make verify` fails if either copy is out of date.

## Play on a local server

To try the plugin without an invitation, run your own server on the game's
content. Needs Go, openssl (RSA keys; LibreSSL's EC keys are refused) and the
same read access as above. `scripts/local-server.sh` does all of this (a
certificate in `~/.seq-local`, `seqd` at the pinned commit on the pinned `seq-world`
packs, the credential file) and prints the `--config` values for the plugin:

```sh
./scripts/local-server.sh
```

By hand, with `$SEQ` a `seq` checkout and `$WORLD` a `seq-world` checkout (at the
commits in `scripts/pin.env`):

```sh
umask 077; mkdir -p ~/.seq-local; cd ~/.seq-local
openssl req -x509 -newkey rsa:2048 -nodes -keyout key.pem -out cert.pem -days 30 \
  -subj /CN=localhost -addext subjectAltName=DNS:localhost,IP:127.0.0.1
(cd "$SEQ" && go build -o ~/.seq-local/seqd ./cmd/seqd)
./seqd -listen 127.0.0.1:8443 -cert cert.pem -key key.pem -packs "$WORLD/packs" \
  -dev-account me -dev-agent claude
# prints: agent credential claude for me-1, me-2: seq_ag_...
```

Put that `seq_ag_...` value alone on a line in `~/.seq-local/credential` (made
under `umask 077`). Then set the server URL to `wss://127.0.0.1:8443/seq/v1`,
the credential file to that file and the CA file to `cert.pem`. The server keeps its
accounts in memory: **after a restart the old credential is dead**, so put the new
one in the file and restart the seq server in your client (`/mcp` in Claude Code and Gemini).

## Check it

`make verify` needs Go and the same read access (it also fetches the pinned
`seq-world` content, read-only). It checks formatting, the repository hygiene
(no credential-shaped string and no absolute home path in any file), that
the pins agree, the plugin manifest (`claude plugin validate`, if Claude Code is
installed), that the guide copies match `guide/PLAY.md`, that Codex CLI and Gemini CLI
(each skipped if not installed) accept the packaging in a throwaway config directory
(`scripts/check-clients.sh`: no login, no chat) and then plays a first contract in Thornwick: a local server on the
Thornwick packs, the plugin's own `seq-mcp` command over stdio, a new character
who takes "Word from Wickett Mill", walks there, comes back and is paid, and the
session's log files (one session id, no credential). The launcher's log locations are
checked too. It takes a
few seconds and fetches nothing but the pinned modules.

## Layout

```
.claude-plugin/plugin.json        manifest and the options above
.claude-plugin/marketplace.json   lets this repository be added as a marketplace
plugin.json, .agents/plugins/      the Codex plugin (the skill) and its marketplace
gemini-extension.json, GEMINI.md  the Gemini extension and its generated context file
guide/PLAY.md                     the one source of the how-to-play guide
mcp/seq.json                      the seq-mcp server (server, credential file and certificate from the options)
scripts/                          build and start seq-mcp, Codex install, guide generator, local server, checks
skills/play-seq/SKILL.md          the guide as a skill (generated)
test/firstcontract/               the end-to-end test
```

The MCP config lives in `mcp/`, not a root `.mcp.json`, so Claude Code does not
also read it as a project config when run inside a checkout.

## Content

Names and text in the game follow the project's provenance rules: everything
is original to SEQ. This repository contains no game content and no engine
source.

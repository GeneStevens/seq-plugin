# seq-plugin

Play SEQ (Spiel Engine Quest) from Claude Code, Codex CLI or Gemini CLI (experimental). SEQ is a multiplayer fantasy RPG
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
  `release`, to see your characters (`list_characters`), and to play with friends:
  `party`, `chat`, `inbox`, `who`, `friends` and `mute`.
- The `play-seq` guide (a skill in Claude Code and Codex, the context file in Gemini CLI, which is experimental), which teaches the model how to play and when to ask you
  first (spending crowns, trading, anything you can't take back).

## Choose your client

SEQ works from **Claude Code** and **Codex CLI**, which are supported, and from
**Gemini CLI**, which is **experimental** (see its section: it needs more setup,
asks you to approve tools, and is slow in real time). Each runs the same `seq-mcp`
server and teaches the model the same how-to-play guide (one source,
`guide/PLAY.md`). Pick one; every path below is complete.

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
| credential file | a file only you can read, holding your `seq_ag_...` credential on one line (required). Set as `SEQ_ACCOUNT_FILE`; the older name `SEQ_CREDENTIAL_FILE` works too, except in Gemini CLI, which hides it |
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

A new character in Thornwick is offered First Steps, a short guided walk (the
greeter, your class trainer, a contract board, the east gate). It is only a guide:
ask the agent to skip it, and it uses `dismiss_quest` to end it for good.

Longer quest chains (for example Barleycross, which starts with the greeter and
goes on with Goodman Hatch in the Downs) are followed on request. The agent reads
`quests` for the current step, `observe` for what each person offers and why a
quest is locked (a level, or a quest to finish first), takes an offer by talking
to its giver, and does each kind of step: talk, go to a place, take a door to
another zone, watch a place for a time without fighting, kill, collect and
deliver. Quest items cannot be sold or traded, and the agent knows it.

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
`play-seq` skill is also reachable as `$play-seq`). The play guide tells Codex to
keep your character logged in between your messages and to log out only when you
ask; without it Codex logs out at the end of every turn, and each turn becomes a
separate session. Codex starts the server on
first use; the first start builds `seq-mcp`, so if the first call fails, run
`scripts/install-seq-mcp.sh` once and start `codex` again.

### Gemini CLI (experimental)

Gemini CLI support is **experimental** (D67). It works, but expect rough edges:

- Gemini asks you to approve each tool call. Choose "Allow all server tools for
  this session" at the first prompt, or you will be asked on every move. (An
  extension cannot mark its own server as trusted: Gemini ignores `trust` in an
  extension's manifest.)
- Use a Flash model (`gemini -m <a Flash model your account has>`). Quota and latency make real-time play slow: a fight can outrun the model,
  and calls can take minutes between tools on Gemini's side (the seq tools answer in
  well under a second). A free quota can run out mid-session.

Gemini keeps its config in `~/.gemini` (under `HOME`; `GEMINI_CLI_HOME` moves it).
This repository is a Gemini extension (`gemini-extension.json`, with the guide in
`GEMINI.md`).

**Gemini does not hand your shell's environment to the extension's server.** It
removes any variable whose name contains `TOKEN`, `SECRET`, `PASSWORD`, `KEY`,
`AUTH`, `CREDENTIAL`, `CERT` and a few others, so `SEQ_CREDENTIAL_FILE` never
arrives (the server then looks "Connected" but cannot play). The extension avoids
this in two ways:

1. **Extension settings** (recommended). `gemini extensions link` or `install`
   asks for the server URL, the credential file, the account (optional) and the
   certificate file (optional), and Gemini passes them to the server. Change them
   later with `gemini extensions config seq`.
2. **Environment variables with names Gemini keeps:** `SEQ_SERVER_URL`,
   `SEQ_ACCOUNT_FILE` (the credential file; not `SEQ_CREDENTIAL_FILE`),
   `SEQ_ACCOUNT` and `SEQ_CA_FILE`, exported where you start `gemini`.

```sh
gemini extensions link /path/to/seq-plugin     # or: gemini extensions install https://github.com/GeneStevens/seq-plugin
# it asks for the settings above; or skip them and export:
#   export SEQ_SERVER_URL=wss://host:port/seq/v1
#   export SEQ_ACCOUNT_FILE=~/.seq-credential
gemini -m <a Flash model>
```

Add `--consent` to skip the install prompt. If a setting is missing, the first tool
call says which one and how to set it. Check: `gemini extensions list` shows `seq`
with its context file and server, and `gemini mcp list` shows `seq (from seq) ...
Connected`; in a session, `/mcp` lists the tools. (`Connected` only means the server
started; `make verify` makes a real call.) First prompt: *"Play SEQ: make a warrior
called Aria and look around."* Run `scripts/install-seq-mcp.sh` once first so the
build does not delay the connection.

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
| Codex CLI | `~/.local/share/seq/logs/codex` (or `$XDG_DATA_HOME/seq/logs/codex`); `SEQ_LOG_DIR` at install time moves it |
| Gemini CLI | `~/.local/share/seq/logs/gemini` (or under `$XDG_DATA_HOME`); export `SEQ_LOG_DIR` where you start `gemini` |

Codex and Gemini give a plugin no data folder of its own, so each gets its own
subfolder of `seq/logs`; an unknown client (the launcher is told which one by
`SEQ_CLIENT`) uses `seq/logs/other`. `SEQ_LOG_DIR` is only a location.

**Replay one** in the SEQ terminal client: `seq-tui -replay
~/.local/share/seq/logs/<session folder>/record.jsonl` (add `-text` for plain text).

**The server's log of your sessions.** On the server it is one file per session,
`<log dir>/sessions/<account>/<session id>.jsonl` (files written before SEQ-066
have a leading underscore on both names, `sessions/_<account>/_<session id>.jsonl`,
and are still read). Ask your AI client: *"List my SEQ sessions"*
and *"Show me the log of this session"* or *"Why did I die?"*. It uses the
`my_sessions` and `session_log` tools, which answer only for your own account; the
log has what you asked for, the answers you were given, chat as shown, moves between
zones, errors and the disconnect.

**Share a session for support** by sending the session's folder (the three files)
and its session id, which is in the folder name. Look through them first:
`tools.jsonl` and `record.jsonl` hold what the game said and the chat you saw, and
the answers your AI client was given, which is your conversation with the game.
The credential is not in them. The server's side is found by the session id.

**Turn logging off.** It is its own setting, `SEQ_LOGGING` (`off`, `false` or `0`
turns it off). Claude Code: `/plugin configure seq@seq` and untick "Keep session
logs". Codex CLI: `SEQ_LOGGING=off` when you run `scripts/codex-install.sh`.
Gemini CLI: `export SEQ_LOGGING=off` where you start `gemini`. Delete the folder at
any time; nothing needs it.

*Old setting:* `SEQ_LOG_DIR=off` (typing `off` in "Session log directory") still
turns logging off for this release; use `SEQ_LOGGING` instead, as the old value
will stop working.

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
and each makes a real game call (`scripts/check-clients.sh`, `test/clients`: no login,
no chat; a local server, the server started the way that client starts it, then
`new_character`, `look` and `logout`, where `look` must answer "You are ..."; Gemini
starts the server itself, so its variable redaction is part of the test) and then plays a first contract in Thornwick: a local server on the
Thornwick packs, the plugin's own `seq-mcp` command over stdio, a new character
who takes "Word from Wickett Mill", walks there, comes back and is paid, and the
session's log files (one session id, no credential). A second test walks a new
character through First Steps, and another dismisses it. A quest test plays the
guide's own calls for the first two Barleycross quests (word to Goodman Hatch,
then the 45 second granary watch) as a scripted session, not a model, and a
check keeps the guide free of the world's secret lore. A social test runs two
plugin processes on two accounts: they party up, talk in party and tell chat, and
both show in `who`. The launcher's log locations are
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
test/clients/                     the real game call through each installed client
skills/play-seq/SKILL.md          the guide as a skill (generated)
test/firstcontract/               the end-to-end test
```

The MCP config lives in `mcp/`, not a root `.mcp.json`, so Claude Code does not
also read it as a project config when run inside a checkout.

## Content

Names and text in the game follow the project's provenance rules: everything
is original to SEQ. This repository contains no game content and no engine
source.

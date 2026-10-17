# seq-plugin

Play SEQ (Spiel Engine Quest) from Claude Code. SEQ is a multiplayer fantasy RPG
with a server that keeps the world; you play by telling Claude what to do
("make a warrior called Aria, take the wolf hunt, and finish it"), and Claude
plays your character through the game's tools. Release 1 is the **Thornwick
alpha**.

**This is an invite-only alpha.** You need a server address and a credential
from the person who invited you. The engine and the world content are private
for now; this repository holds only the plugin: the manifest, the MCP
configuration, a how-to-play skill and a test.

## What you get

- The `seq` MCP server (`seq-mcp`), with tools to `look`, `go_to`, `talk_to`,
  `take_contract`, `attack`, `cast`, `loot`, `turn_in`, `use_door` and
  `release`, and to chat and party up with other players.
- The `play-seq` skill, which teaches Claude how to play and when to ask you
  first (spending crowns, trading, anything you can't take back).

## Install

You need Go (to build `seq-mcp`), Claude Code, and **read access to the private
`seq` repository on GitHub** from this machine: in M10 `seq-mcp` is built from
source at a pinned commit (`scripts/pin.env`). Prebuilt binaries come later. Check
with `git ls-remote https://github.com/GeneStevens/seq` (or `gh auth status`),
and tell Go the repository is private: `export GOPRIVATE='github.com/GeneStevens/*'`
(the plugin sets this itself when it builds).

A plugin installs into the config directory of the `claude` that runs the
install. If you keep several Claude accounts, each with its own
`CLAUDE_CONFIG_DIR`, run every `claude plugin` command under the one you will
play in, or it installs somewhere you never look:

```sh
git clone https://github.com/GeneStevens/seq-plugin
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
install`) sets:

| Option | What |
| --- | --- |
| `server_url` | the server's gateway, like `wss://host:port/seq/v1` (required) |
| `credential_file` | a file only you can read, holding your `seq_ag_...` credential on one line (required) |
| `account` | only if that file has several `account=credential` lines |
| `ca_file` | a PEM file for the server's certificate, if it uses a self-signed one |

Make the credential file with `umask 077`, and keep it out of any repository.
Nothing in this plugin holds a credential. `claude mcp list` should then show
`plugin:seq:seq ... Connected`, and in a session `/mcp` lists the server's tools. Start with: *"Play SEQ: make a warrior
called Aria and look around."*

The first start builds `seq-mcp` from source (kept in the plugin's data
directory, one binary per pinned commit), which takes a while; Claude Code may
show the server as failed until it finishes, and `/mcp` then reconnects it.
`scripts/install-seq-mcp.sh` run by hand does not help: it builds into
`~/.cache/seq-plugin`, not the plugin's data directory, so it only warms Go's cache.

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
under `umask 077`). Then set `server_url` to `wss://127.0.0.1:8443/seq/v1`,
`credential_file` to that file and `ca_file` to `cert.pem`. The server keeps its
accounts in memory: **after a restart the old credential is dead**, so put the new
one in the file and restart the seq server in Claude Code (`/mcp`).

## Check it

`make verify` needs Go and the same read access (it also fetches the pinned
`seq-world` content, read-only). It checks formatting, the repository hygiene
(no credential-shaped string and no absolute home path in any file), that
the pins agree, the plugin manifest (`claude plugin validate`, if Claude Code is
installed) and then plays a first contract in Thornwick: a local server on the
Thornwick packs, the plugin's own `seq-mcp` command over stdio, a new character
who takes "Word from Wickett Mill", walks there, comes back and is paid. It takes a
few seconds and fetches nothing but the pinned modules.

## Layout

```
.claude-plugin/plugin.json        manifest and the options above
.claude-plugin/marketplace.json   lets this repository be added as a marketplace
mcp/seq.json                      the seq-mcp server (server, credential file and certificate from the options)
scripts/                          build and start seq-mcp, a local server, pins, hygiene check
skills/play-seq/SKILL.md          how Claude plays
test/firstcontract/               the end-to-end test
```

The MCP config lives in `mcp/`, not a root `.mcp.json`, so Claude Code does not
also read it as a project config when run inside a checkout.

## Content

Names and text in the game follow the project's provenance rules: everything
is original to SEQ. This repository contains no game content and no engine
source.

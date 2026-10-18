# seq-plugin: make verify runs the repo hygiene check, the plugin manifest check
# and the first-contract test (a local SEQ server on Thornwick, the built
# seq-mcp over stdio), plus that the guide copies match guide/PLAY.md and that
# Codex CLI and Gemini CLI accept the packaging and make a real game call (skipped if not installed). Needs Go and read access to the private seq and
# seq-world repositories (see README); nothing else is fetched.
export GOPRIVATE ?= github.com/GeneStevens/*

.PHONY: verify
verify:
	@gofmt -l . | (! grep .) || { echo "gofmt: unformatted files"; exit 1; }
	@./scripts/hygiene.sh
	@./scripts/check-pins.sh
	@./scripts/gen-guide.sh --check
	@if command -v claude >/dev/null 2>&1; then claude plugin validate . && claude plugin validate .claude-plugin/plugin.json ; else echo "claude not installed: skipping plugin validate"; fi
	@./scripts/check-clients.sh
	@go vet ./...
	@go test -count=1 ./...

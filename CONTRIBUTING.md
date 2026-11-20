# Contributing to Inkweave

Thank you for helping improve Inkweave! All contributions to this repository are licensed under **CC BY-NC-SA 4.0** (see [LICENSE](LICENSE)).

## Development Loop

1. **Go Core Tests**:
   ```bash
   go test -v -race ./...
   ```
2. **Svelte 5 UI Check & Build**:
   ```bash
   cd web
   npm install
   npm run check
   npm run build
   ```
3. **Run Dev Mode**:
   Start the Go backend with `go run ./cmd/inkweave --port 7842 --no-open`, then run `cd web && npm run dev` if iterating on hot-reloaded UI (or access the embedded build directly at the URL printed by `cmd/inkweave`).

## Adding New Built-in Skills
See [docs/SKILLS_GUIDE.md](docs/SKILLS_GUIDE.md) for the Markdown + YAML frontmatter specification.

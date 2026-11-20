# Inkweave — Local-First AI Agent Novel Studio

**Inkweave** is an open-source, local-first **AI Agent Novel Writing Studio** for PC (Windows / macOS / Linux) and **Android APK**, engineered with a **100% Pure-Go Core Engine** (`modernc.org/sqlite` + `go-git/v5`) and a **Svelte 5 + CodeMirror 6 Literary Studio UI**.

Read in Thai: [README.th.md](README.th.md)

---

## Key Capabilities

- **4-Tier Story Memory & Thai-Aware Search**:
  - Dual SQLite FTS5 indexing (`unicode61` + `trigram`) + entity alias scanning for fast recall on unspaced Thai and English prose.
  - **Tier 0** Cache-Stable Story DNA (POV, style rules, anti-cliché list, world laws, active skill) — structured for Prompt Caching savings.
  - **Tier 1** Active Scene Working Set (chapter beats, prose tail, POV depth, exact Character States & `UnawareOf` knowledge boundaries).
  - **Tier 2** Selective Recall (FTS5 lore hits, asymmetric relationships, open plot threads, timeline).
  - **Tier 3** Hierarchical Rolling Summaries for long-form 100+ chapter manuscripts.
  - Built-in **Context Inspector** shows exact Tier 0–3 tokens before every turn.

- **3 Precision Agent Modes (`Eco` / `Balanced` / `Ultra`)**:
  | Mode | Budget Cap | Execution Strategy | Best For |
  | :--- | :--- | :--- | :--- |
  | **Eco (ประหยัด)** | ≤ 6,000 tokens | Single-pass + utility model | Fast drafts & maximum token savings |
  | **Balanced (สมดุล)** | ≤ 18,000 tokens | Cache-aligned + guided tool loop (≤ 4 tools) | Daily co-writing & continuity tracking |
  | **Ultra (ขั้นสูงสุด)** | ≤ 48,000+ tokens | 4-Stage Pipeline: Architect → Writer → Continuity/Anti-Slop Critic → Polish | Pivotal scenes, complex twists & deep audits |

- **Deep Non-Linear Character Atelier**:
  - Models **Core Paradox**, **Social Mask vs True Self**, **Blind Spot**, **Formative Wound → Misbelief**, **External Want vs Internal Need**, **Defense Mechanisms**, and **Voice Guide**.
  - Interactive **Non-Linear Arc Curve** (`spiral`, `oscillating`, `fractured`, `tragic`, `redemption`) with explicit **Regression / Relapse** beats.
  - **Asymmetric Directed Relationships** (`A → B` vs `B → A`) tracking Trust, Tension, Public Face, and Hidden Agenda.

- **9 Modular Story Skills & 20 Specialized Tools**:
  - Skills live as human-editable Markdown + YAML files (`skills/*.md`), including `scene-writer`, `character-architect`, `continuity-guard`, `plot-architect`, `character-voice`, `prose-editor`, `worldbuilder`, `memory-curator`, and `brainstorm-partner`.
  - Write tools create automatic **Snapshots** and queue line-level **Diff Proposals** for 1-click Accept / Reject.

- **Multi-API Hub & Model Router**:
  - Native drivers for **OpenAI**, **Anthropic Claude** (with `cache_control: ephemeral`), **Google Gemini**, and **OpenAI-Compatible** endpoints (OpenRouter, DeepSeek, Groq, Ollama local models) + Built-in offline Demo mode.
  - Route `Primary`, `Utility`, and `Architect` roles independently.

- **Local-First Multi-Cloud & Git Sync + EPUB 3 Export**:
  - Novels are stored as clean Markdown + YAML files (`manuscript/`, `bible/`, `continuity/`), while `.inkweave/` caches remain excluded.
  - Sync across PC and Android via **Git / GitHub** (pure-Go `go-git/v5`), **Google Drive** (REST v3 + OAuth 2.0 PKCE), **WebDAV** (Nextcloud, ownCloud, Synology NAS, Koofr), or **Synced Folders** (OneDrive, Dropbox, Syncthing) — all with 3-way conflict detection, automatic Snapshot safety backups, and a Side-by-Side Conflict Resolver UI.
  - Export full manuscripts to **EPUB 3** (`.epub`) or **Markdown** (`.md`).

---

## Quickstart (PC)

Prerequisites: **Go 1.23+** and **Node.js 20+**

```bash
# 1. Build the Literary Studio UI into internal/webui/dist
cd web && npm install && npm run build && cd ..

# 2. Build & run the single-binary PC studio
go build -o bin/inkweave ./cmd/inkweave
./bin/inkweave
```

## Android APK Build

See [`scripts/build-android.ps1`](scripts/build-android.ps1) / [`scripts/build-android.sh`](scripts/build-android.sh) or trigger the automated `.github/workflows/release.yml` workflow on GitHub Actions to compile `inkweavecore.aar` via `gomobile` and assemble `inkweave-android.apk`.

---

## License & Author Rights

Inkweave is licensed under **Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International (CC BY-NC-SA 4.0)** — see [LICENSE](LICENSE):
- You may freely share, fork, modify, and redistribute the software.
- **Attribution**: Keep credit and license notices intact.
- **NonCommercial**: Do not sell Inkweave or commercialize modified versions.
- **ShareAlike**: Derivatives of Inkweave must also be distributed under **CC BY-NC-SA 4.0**.

> **Author Rights Guarantee**: Creative works (novels, outlines, characters, and exported EPUB/Markdown files) authored *with* Inkweave belong **100% to their authors**, including full commercial publishing rights.

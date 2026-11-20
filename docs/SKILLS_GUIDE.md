# Inkweave Custom Skills Authoring Guide

Skills in Inkweave are portable Markdown files with YAML frontmatter. Built-in skills live in `skills/*.md`, and per-novel custom skills live in `<novel-repo>/skills/*.md` (which can override a built-in skill of the same `id` or add new genre-specific workflows).

## Skill File Format

```markdown
---
id: mystery-clue-weaver
name: Mystery Clue Weaver
name_th: นักวางเบาะแสสืบสวน
description: Designs fair-play clues and red herrings without breaking timeline logic.
description_th: ออกแบบเบาะแสและตัวหลอกอย่างยุติธรรมโดยไม่ขัดกับไทม์ไลน์
category: planning
recommended_modes:
  - balanced
  - ultra
tools:
  - get_timeline
  - list_open_threads
  - read_chapter
  - manage_plot_thread
---
You are a Fair-Play Mystery Designer...
```

## Available Story Tools

**Read-Only (Safe in any mode)**:
`search_manuscript`, `search_lore`, `read_chapter`, `get_outline`, `get_character_profile`, `get_character_state`, `get_timeline`, `list_open_threads`, `get_relationships`, `audit_continuity`

**Write / Propose (Queued as Diff Proposals or Snapshot-protected)**:
`write_chapter_draft`, `edit_chapter_section`, `save_chapter_summary`, `upsert_character_profile`, `record_character_arc_beat`, `upsert_lore_entry`, `update_character_state`, `upsert_relationship`, `record_timeline_event`, `manage_plot_thread`

<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { EditorState } from '@codemirror/state';
  import { EditorView, placeholder } from '@codemirror/view';
  import { markdown } from '@codemirror/lang-markdown';
  import {
    Maximize2,
    Minimize2,
    History,
    Sparkles,
    Check,
    X,
    ListChecks,
    RotateCcw,
    Camera,
    Trash2,
    Plus,
  } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';
  import { api } from '../api/client';
  import type { Outline, Snapshot } from '../types';

  let editorHost: HTMLDivElement | undefined = $state();
  let view: EditorView | null = null;
  let lastLoadedKey = '';
  let snapshotsOpen = $state(false);
  let beatsOpen = $state(false);
  let snapshots = $state<Snapshot[]>([]);
  let loadingSnaps = $state(false);
  let newBeatSummary = $state('');
  let newBeatTurning = $state('');
  let chapterGoalDraft = $state('');

  let chapter = $derived(studio.activeChapter);
  let chapterProposals = $derived(
    studio.proposals.filter((p) => p.status === 'pending' && p.target_id === chapter?.id),
  );
  let chapterOutline = $derived.by(() => {
    if (!studio.outline || !chapter) return null;
    for (const act of studio.outline.acts || []) {
      for (const arc of act.arcs || []) {
        const found = (arc.chapters || []).find((c) => c.chapter_id === chapter.id);
        if (found) return found;
      }
    }
    return null;
  });

  function mountEditor(initialDoc: string) {
    if (!editorHost) return;
    if (view) {
      view.destroy();
      view = null;
    }
    const state = EditorState.create({
      doc: initialDoc,
      extensions: [
        markdown(),
        EditorView.lineWrapping,
        placeholder(
          studio.lang === 'th'
            ? 'เริ่มต้นเขียนฉากของคุณที่นี่... (รองรับ Markdown, แตะเลือกข้อความเพื่อเรียก AI ช่วยเกลาได้)'
            : 'Begin your scene here... (Markdown supported; select text for Quick AI edits)',
        ),
        EditorView.updateListener.of((update) => {
          if (update.selectionSet) {
            const sel = update.state.selection.main;
            studio.selectedText = sel.empty
              ? ''
              : update.state.sliceDoc(sel.from, sel.to);
          }
          if (update.docChanged && chapter) {
            const nextText = update.state.doc.toString();
            chapter.content = nextText;
            studio.scheduleChapterSave(chapter);
          }
        }),
      ],
    });
    view = new EditorView({ state, parent: editorHost });
  }

  onMount(() => {
    if (chapter) {
      lastLoadedKey = `${chapter.id}:${chapter.updated_at}`;
      mountEditor(chapter.content || '');
    }
  });

  $effect(() => {
    const ch = chapter;
    if (!ch || !editorHost) return;
    const key = `${ch.id}:${ch.updated_at}`;
    if (!view || ch.id !== lastLoadedKey.split(':')[0]) {
      lastLoadedKey = key;
      mountEditor(ch.content || '');
    } else if (view && view.state.doc.toString() !== ch.content && studio.saveIndicator !== 'saving') {
      // External update (e.g., snapshot restore or AI proposal applied)
      const current = view.state.doc.toString();
      if (current !== ch.content) {
        view.dispatch({
          changes: { from: 0, to: current.length, insert: ch.content || '' },
        });
      }
      lastLoadedKey = key;
    }
  });

  onDestroy(() => {
    view?.destroy();
  });

  async function openSnapshots() {
    if (!studio.activeProject || !chapter) return;
    snapshotsOpen = !snapshotsOpen;
    if (snapshotsOpen) {
      loadingSnaps = true;
      try {
        snapshots = await api<Snapshot[]>(
          `/api/projects/${studio.activeProject.id}/chapters/${chapter.id}/snapshots`,
        );
      } finally {
        loadingSnaps = false;
      }
    }
  }

  async function saveCheckpoint() {
    if (!studio.activeProject || !chapter) return;
    try {
      await api(`/api/projects/${studio.activeProject.id}/chapters/${chapter.id}/snapshots`, {
        method: 'POST',
        body: JSON.stringify({ reason: 'manual-checkpoint' }),
      });
      snapshots = await api<Snapshot[]>(
        `/api/projects/${studio.activeProject.id}/chapters/${chapter.id}/snapshots`,
      );
      studio.notify(studio.lang === 'th' ? 'สร้างจุดสำรอง Snapshot แล้ว' : 'Snapshot saved');
    } catch (e) {
      studio.notify(String(e));
    }
  }

  async function restoreSnap(sid: string) {
    if (!studio.activeProject || !chapter) return;
    try {
      const restored = await api<typeof chapter>(
        `/api/projects/${studio.activeProject.id}/chapters/${chapter.id}/snapshots/${sid}/restore`,
        { method: 'POST' },
      );
      studio.chapters = studio.chapters.map((c) => (c.id === restored.id ? restored : c));
      snapshotsOpen = false;
      studio.notify(studio.lang === 'th' ? 'ย้อนกลับเนื้อหาจาก Snapshot แล้ว' : 'Restored chapter from snapshot');
    } catch (e) {
      studio.notify(String(e));
    }
  }

  async function deleteCurrentChapter() {
    if (!chapter) return;
    const msg =
      studio.lang === 'th'
        ? `ต้องการลบตอน "${chapter.title}" ถาวรหรือไม่?`
        : `Permanently delete chapter "${chapter.title}"?`;
    if (!window.confirm(msg)) return;
    try {
      await studio.deleteChapter(chapter.id);
    } catch (e) {
      studio.notify(String(e));
    }
  }

  function cloneOutlineWithChapter(): { outline: Outline; chOut: NonNullable<typeof chapterOutline> } | null {
    if (!chapter) return null;
    const base: Outline = studio.outline
      ? structuredClone($state.snapshot(studio.outline))
      : { premise: studio.activeProject?.synopsis || '', acts: [] };
    if (!base.acts || base.acts.length === 0) {
      base.acts = [
        {
          id: 'act-1',
          title: 'Act 1',
          summary: '',
          arcs: [{ id: 'arc-1', title: 'Main Arc', summary: '', chapters: [] }],
        },
      ];
    }
    if (!base.acts[0].arcs || base.acts[0].arcs.length === 0) {
      base.acts[0].arcs = [{ id: 'arc-1', title: 'Main Arc', summary: '', chapters: [] }];
    }
    for (const act of base.acts) {
      for (const arc of act.arcs || []) {
        const existing = (arc.chapters || []).find((c) => c.chapter_id === chapter.id);
        if (existing) {
          if (!existing.beats) existing.beats = [];
          return { outline: base, chOut: existing };
        }
      }
    }
    const created = {
      chapter_id: chapter.id,
      title: chapter.title,
      goal: '',
      target_words: 1500,
      beats: [],
    };
    base.acts[0].arcs[0].chapters = [...(base.acts[0].arcs[0].chapters || []), created];
    return { outline: base, chOut: created };
  }

  async function addBeat() {
    if (!newBeatSummary.trim()) return;
    const res = cloneOutlineWithChapter();
    if (!res) return;
    res.chOut.beats.push({
      id: `beat-${Date.now()}`,
      summary: newBeatSummary.trim(),
      turning: newBeatTurning.trim() || undefined,
      completed: false,
    });
    newBeatSummary = '';
    newBeatTurning = '';
    await studio.saveOutline(res.outline);
  }

  async function toggleBeat(beatId: string) {
    const res = cloneOutlineWithChapter();
    if (!res) return;
    res.chOut.beats = res.chOut.beats.map((b) =>
      b.id === beatId ? { ...b, completed: !b.completed } : b,
    );
    await studio.saveOutline(res.outline);
  }

  async function removeBeat(beatId: string) {
    const res = cloneOutlineWithChapter();
    if (!res) return;
    res.chOut.beats = res.chOut.beats.filter((b) => b.id !== beatId);
    await studio.saveOutline(res.outline);
  }

  async function saveChapterGoal() {
    const res = cloneOutlineWithChapter();
    if (!res) return;
    res.chOut.goal = chapterGoalDraft.trim();
    await studio.saveOutline(res.outline);
  }

  $effect(() => {
    chapterGoalDraft = chapterOutline?.goal || '';
  });
</script>

<div class="relative flex h-full flex-col overflow-hidden" style="background: var(--bg);">
  {#if chapter}
    <!-- Minimal Editorial Header -->
    <div
      class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b px-4 py-2.5"
      style="border-color: var(--border); background: var(--bg);"
    >
      <div class="flex min-w-0 flex-1 items-center gap-2.5">
        <input
          type="text"
          value={chapter.title}
          oninput={(e) => {
            chapter.title = e.currentTarget.value;
            studio.scheduleChapterSave(chapter);
          }}
          class="min-w-0 flex-1 bg-transparent text-base font-semibold outline-none sm:text-lg"
          style="color: var(--ink);"
          aria-label="Chapter Title"
        />
        <span
          class="hidden shrink-0 rounded-full px-2 py-0.5 text-xs tabular-nums sm:inline-block"
          style="background: var(--surface); color: var(--muted);"
        >
          {chapter.word_count.toLocaleString()} {studio.t.library.words}
        </span>
      </div>

      <div class="flex items-center gap-1.5">
        <span class="text-xs tabular-nums" style="color: var(--muted);">
          {#if studio.saveIndicator === 'saving'}
            {studio.t.editor.saving}
          {:else if studio.saveIndicator === 'saved'}
            {studio.t.editor.saved}
          {/if}
        </span>

        <button
          onclick={() => (beatsOpen = !beatsOpen)}
          class="flex items-center gap-1 rounded-lg px-2 py-1.5 text-xs font-medium transition-colors"
          style="background: {beatsOpen ? 'var(--accent-soft)' : 'transparent'}; color: {beatsOpen ? 'var(--accent)' : 'var(--muted)'};"
          title={studio.t.editor.beats}
        >
          <ListChecks size={15} />
          <span class="hidden md:inline">{studio.t.editor.beats}</span>
        </button>

        <button
          onclick={openSnapshots}
          class="flex items-center gap-1 rounded-lg px-2 py-1.5 text-xs font-medium transition-colors"
          style="background: {snapshotsOpen ? 'var(--accent-soft)' : 'transparent'}; color: {snapshotsOpen ? 'var(--accent)' : 'var(--muted)'};"
          title={studio.t.editor.snapshots}
        >
          <History size={15} />
          <span class="hidden md:inline">{studio.t.editor.snapshots}</span>
        </button>

        <button
          onclick={deleteCurrentChapter}
          class="flex items-center gap-1 rounded-lg px-2 py-1.5 text-xs font-medium transition-colors hover:opacity-80"
          style="color: var(--danger);"
          title={studio.lang === 'th' ? 'ลบตอนนี้' : 'Delete chapter'}
          aria-label="Delete current chapter"
        >
          <Trash2 size={15} />
        </button>

        <button
          onclick={() => (studio.focusMode = !studio.focusMode)}
          class="hidden items-center gap-1 rounded-lg px-2 py-1.5 text-xs font-medium transition-colors md:flex"
          style="background: {studio.focusMode ? 'var(--accent-soft)' : 'transparent'}; color: {studio.focusMode ? 'var(--accent)' : 'var(--muted)'};"
          title={studio.focusMode ? studio.t.editor.exitFocus : studio.t.editor.focusMode}
        >
          {#if studio.focusMode}
            <Minimize2 size={15} />
          {:else}
            <Maximize2 size={15} />
          {/if}
        </button>
      </div>
    </div>

    <!-- Chapter Metadata Strip (POV / Timeline Day / Status) -->
    {#if !studio.focusMode}
      <div
        class="flex shrink-0 flex-wrap items-center gap-3 border-b px-4 py-1.5 text-xs"
        style="border-color: var(--border); background: var(--surface); color: var(--muted);"
      >
        <label class="flex items-center gap-1.5">
          <span>POV:</span>
          <input
            type="text"
            value={chapter.pov}
            oninput={(e) => {
              chapter.pov = e.currentTarget.value;
              studio.scheduleChapterSave(chapter);
            }}
            class="w-32 rounded border bg-transparent px-1.5 py-0.5 text-xs outline-none"
            style="border-color: var(--border); color: var(--ink);"
          />
        </label>
        <label class="flex items-center gap-1.5">
          <span>Day:</span>
          <input
            type="text"
            value={chapter.timeline_day}
            oninput={(e) => {
              chapter.timeline_day = e.currentTarget.value;
              studio.scheduleChapterSave(chapter);
            }}
            class="w-28 rounded border bg-transparent px-1.5 py-0.5 text-xs outline-none"
            style="border-color: var(--border); color: var(--ink);"
          />
        </label>
        <select
          value={chapter.status}
          onchange={(e) => {
            chapter.status = e.currentTarget.value;
            studio.scheduleChapterSave(chapter);
          }}
          class="rounded border bg-transparent px-1.5 py-0.5 text-xs outline-none"
          style="border-color: var(--border); color: var(--ink);"
        >
          <option value="outline">Outline</option>
          <option value="draft">Draft</option>
          <option value="revising">Revising</option>
          <option value="final">Final</option>
        </select>
      </div>
    {/if}

    <!-- Beats Collapsible Strip (Full Add / Toggle / Delete) -->
    {#if beatsOpen}
      <div
        class="anim-fade shrink-0 space-y-2.5 border-b px-4 py-3 text-xs"
        style="border-color: var(--border); background: var(--surface-elevated);"
      >
        <div class="flex flex-wrap items-center gap-2">
          <input
            type="text"
            bind:value={chapterGoalDraft}
            onkeydown={(e) => {
              if (e.key === 'Enter') saveChapterGoal();
            }}
            placeholder={studio.lang === 'th' ? 'เป้าหมายของตอนนี้ (Chapter Goal)...' : 'Chapter goal...'}
            class="min-w-0 flex-1 rounded-lg border px-2.5 py-1.5 text-xs font-medium outline-none"
            style="border-color: var(--border); background: var(--bg); color: var(--ink);"
          />
          <button
            onclick={saveChapterGoal}
            class="rounded-lg border px-2.5 py-1.5 text-xs font-semibold"
            style="border-color: var(--border); background: var(--surface); color: var(--ink);"
          >
            {studio.lang === 'th' ? 'บันทึกเป้าหมาย' : 'Save Goal'}
          </button>
        </div>

        {#if chapterOutline && chapterOutline.beats.length > 0}
          <div class="max-h-40 space-y-1.5 overflow-y-auto">
            {#each chapterOutline.beats as beat, i}
              <div
                class="flex items-start justify-between gap-2 rounded-lg border px-2.5 py-1.5"
                style="border-color: var(--border); background: var(--bg);"
              >
                <button
                  onclick={() => toggleBeat(beat.id)}
                  class="flex min-w-0 flex-1 items-start gap-2 text-left"
                >
                  <span
                    class="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded border text-[10px]"
                    style="
                      border-color: {beat.completed ? 'var(--success)' : 'var(--border)'};
                      background: {beat.completed ? 'var(--success)' : 'transparent'};
                      color: #fff;
                    "
                  >
                    {#if beat.completed}✓{/if}
                  </span>
                  <div class="min-w-0 flex-1 {beat.completed ? 'line-through opacity-55' : ''}">
                    <span class="mr-1 font-semibold tabular-nums" style="color: var(--accent);">{i + 1}.</span>
                    <span>{beat.summary}</span>
                    {#if beat.turning}
                      <span class="ml-1 rounded px-1.5 py-0.5 text-[10px]" style="background: var(--accent-soft); color: var(--accent);">
                        {beat.turning}
                      </span>
                    {/if}
                  </div>
                </button>
                <button
                  onclick={() => removeBeat(beat.id)}
                  class="shrink-0 rounded p-1 opacity-60 hover:opacity-100"
                  style="color: var(--danger);"
                  title={studio.lang === 'th' ? 'ลบ Beat' : 'Delete beat'}
                >
                  <Trash2 size={12} />
                </button>
              </div>
            {/each}
          </div>
        {:else}
          <p style="color: var(--muted);">
            {studio.lang === 'th' ? 'ยังไม่มีโครงจังหวะฉาก (Beats) สำหรับตอนนี้ — เพิ่มด้านล่างได้เลย' : 'No scene beats yet — add one below.'}
          </p>
        {/if}

        <div class="flex flex-wrap gap-1.5">
          <input
            type="text"
            bind:value={newBeatSummary}
            onkeydown={(e) => {
              if (e.key === 'Enter') addBeat();
            }}
            placeholder={studio.lang === 'th' ? 'เพิ่มจังหวะฉากใหม่...' : 'Add a scene beat...'}
            class="min-w-[160px] flex-1 rounded-lg border px-2.5 py-1.5 text-xs outline-none"
            style="border-color: var(--border); background: var(--bg); color: var(--ink);"
          />
          <input
            type="text"
            bind:value={newBeatTurning}
            onkeydown={(e) => {
              if (e.key === 'Enter') addBeat();
            }}
            placeholder={studio.lang === 'th' ? 'จุดพลิก (ไม่บังคับ)' : 'Turning point (optional)'}
            class="w-36 rounded-lg border px-2.5 py-1.5 text-xs outline-none"
            style="border-color: var(--border); background: var(--bg); color: var(--ink);"
          />
          <button
            onclick={addBeat}
            disabled={!newBeatSummary.trim()}
            class="flex items-center gap-1 rounded-lg px-3 py-1.5 text-xs font-semibold disabled:opacity-40"
            style="background: var(--accent); color: var(--accent-ink);"
          >
            <Plus size={12} />
            <span>{studio.lang === 'th' ? 'เพิ่ม Beat' : 'Add'}</span>
          </button>
        </div>
      </div>
    {/if}

    <!-- Snapshot History Drawer -->
    {#if snapshotsOpen}
      <div
        class="anim-fade shrink-0 border-b px-4 py-3 text-xs"
        style="border-color: var(--border); background: var(--surface-elevated);"
      >
        <div class="mb-2 flex items-center justify-between">
          <span class="font-semibold">{studio.t.editor.snapshots}</span>
          <button
            onclick={saveCheckpoint}
            class="flex items-center gap-1 rounded-md px-2.5 py-1 text-xs font-medium"
            style="background: var(--accent); color: var(--accent-ink);"
          >
            <Camera size={13} />
            {studio.t.editor.saveSnapshot}
          </button>
        </div>
        {#if loadingSnaps}
          <p style="color: var(--muted);">Loading...</p>
        {:else if snapshots.length === 0}
          <p style="color: var(--muted);">ยังไม่มีจุดสำรอง Snapshot</p>
        {:else}
          <div class="max-h-44 space-y-1.5 overflow-y-auto">
            {#each snapshots as snap}
              <div
                class="flex items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5"
                style="border-color: var(--border); background: var(--bg);"
              >
                <div class="min-w-0">
                  <div class="truncate font-medium">{snap.reason} • {snap.word_count} {studio.t.library.words}</div>
                  <div class="truncate text-[11px]" style="color: var(--muted);">{snap.preview}</div>
                </div>
                <button
                  onclick={() => restoreSnap(snap.id)}
                  class="flex shrink-0 items-center gap-1 rounded px-2 py-1 text-xs font-medium"
                  style="background: var(--surface); color: var(--ink);"
                >
                  <RotateCcw size={12} />
                  {studio.t.editor.restore}
                </button>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    <!-- Inline Diff Review Overlay for Pending Proposals -->
    {#if chapterProposals.length > 0}
      <div
        class="anim-fade max-h-72 shrink-0 overflow-y-auto border-b px-4 py-3"
        style="border-color: var(--accent); background: var(--surface-elevated);"
      >
        {#each chapterProposals as prop}
          <div class="mb-2 last:mb-0">
            <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
              <div>
                <div class="text-xs font-semibold" style="color: var(--accent);">
                  {studio.t.agent.proposalTitle}: {prop.title}
                </div>
                {#if prop.summary}
                  <div class="text-xs" style="color: var(--muted);">{prop.summary}</div>
                {/if}
              </div>
              <div class="flex items-center gap-2">
                <button
                  onclick={() => studio.rejectProposal(prop.id)}
                  class="flex items-center gap-1 rounded-lg border px-2.5 py-1 text-xs font-medium"
                  style="border-color: var(--border); color: var(--danger);"
                >
                  <X size={13} />
                  {studio.t.agent.reject}
                </button>
                <button
                  onclick={() => studio.applyProposal(prop.id)}
                  class="flex items-center gap-1 rounded-lg px-2.5 py-1 text-xs font-medium"
                  style="background: var(--success); color: #fff;"
                >
                  <Check size={13} />
                  {studio.t.agent.accept}
                </button>
              </div>
            </div>
            <div
              class="max-h-40 overflow-y-auto rounded-lg border p-2.5 font-mono text-xs leading-relaxed"
              style="border-color: var(--border); background: var(--bg);"
            >
              {#each prop.hunks as h}
                {#if h.type === 'delete'}
                  <div class="rounded px-1.5 py-0.5 line-through" style="background: var(--danger-soft); color: var(--danger);">{h.text}</div>
                {:else if h.type === 'insert'}
                  <div class="rounded px-1.5 py-0.5" style="background: var(--success-soft); color: var(--success);">+ {h.text}</div>
                {:else}
                  <div class="truncate px-1.5 opacity-60">{h.text}</div>
                {/if}
              {/each}
            </div>
          </div>
        {/each}
      </div>
    {/if}

    <!-- CodeMirror 6 Canvas -->
    <div bind:this={editorHost} class="min-h-0 flex-1 overflow-hidden"></div>

    <!-- Floating Quick-Agent Trigger (pinned inside Editor container) -->
    <button
      onclick={() => (studio.quickAgentOpen = true)}
      class="absolute right-4 bottom-4 z-20 flex items-center gap-1.5 rounded-full px-3.5 py-2.5 text-xs font-semibold shadow-lg transition-transform active:scale-95 md:right-6 md:bottom-5"
      style="background: var(--accent); color: var(--accent-ink); box-shadow: var(--shadow-subtle);"
      title={studio.t.editor.quickAI}
    >
      <Sparkles size={15} />
      <span>{studio.selectedText ? (studio.lang === 'th' ? 'AI ข้อความที่เลือก' : 'AI Selection') : studio.t.editor.quickAI}</span>
    </button>
  {:else}
    <div class="flex h-full flex-col items-center justify-center gap-3 p-6 text-center text-sm" style="color: var(--muted);">
      <p>{studio.lang === 'th' ? 'ยังไม่มีตอนในเรื่องนี้ — สร้างตอนใหม่เพื่อเริ่มเขียนได้เลย' : 'No chapters yet — create your first chapter to begin writing.'}</p>
      <button
        onclick={() => studio.addChapter(studio.lang === 'th' ? `บทที่ ${studio.chapters.length + 1}` : `Chapter ${studio.chapters.length + 1}`)}
        class="flex items-center gap-1.5 rounded-xl px-4 py-2 text-xs font-semibold"
        style="background: var(--accent); color: var(--accent-ink);"
      >
        <Plus size={14} />
        <span>{studio.t.editor.newChapter}</span>
      </button>
    </div>
  {/if}
</div>

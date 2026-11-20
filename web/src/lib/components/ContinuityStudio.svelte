<script lang="ts">
  import { ShieldAlert, Clock, GitBranch, UserCheck, Plus, X, Trash2 } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';
  import { api } from '../api/client';

  let tab = $state<'timeline' | 'threads' | 'states'>('timeline');
  let auditResult = $state<{ chapters_scanned: number; open_threads: number; findings: string[] } | null>(null);

  // Timeline form
  let newEventDay = $state('');
  let newEventLocation = $state('');
  let newEventSummary = $state('');
  let newEventImpact = $state('');

  // Plot thread form
  let newThreadTitle = $state('');
  let newThreadNotes = $state('');

  // Character state form
  let newStateCharId = $state('');
  let newStateLocation = $state('');
  let newStatePhysical = $state('');
  let newStateEmotion = $state('');
  let newStateKnows = $state('');
  let newStateUnaware = $state('');

  function charName(id: string) {
    return studio.characters.find((c) => c.id === id)?.name || id;
  }

  function chapterTitle(id: string) {
    return studio.chapters.find((c) => c.id === id)?.title || id;
  }

  async function runAudit() {
    if (!studio.activeProject) return;
    auditResult = await api(`/api/projects/${studio.activeProject.id}/continuity/audit`, { method: 'POST' });
  }

  async function addTimelineEvent() {
    if (!studio.activeProject || !newEventSummary.trim()) return;
    const next = [
      ...studio.timeline,
      {
        id: `tl-${Date.now()}`,
        order: studio.timeline.length + 1,
        day_label: newEventDay.trim() || (studio.lang === 'th' ? 'วันใหม่' : 'New Day'),
        chapter_id: studio.activeChapterId || studio.chapters[0]?.id || 'ch-001',
        location: newEventLocation.trim() || '-',
        participants: [],
        summary: newEventSummary.trim(),
        impact: newEventImpact.trim(),
        locked: false,
      },
    ];
    studio.timeline = await api(`/api/projects/${studio.activeProject.id}/continuity/timeline`, {
      method: 'PUT',
      body: JSON.stringify(next),
    });
    newEventDay = '';
    newEventLocation = '';
    newEventSummary = '';
    newEventImpact = '';
  }

  async function deleteTimelineEvent(id: string) {
    if (!studio.activeProject) return;
    const next = studio.timeline.filter((e) => e.id !== id);
    studio.timeline = await api(`/api/projects/${studio.activeProject.id}/continuity/timeline`, {
      method: 'PUT',
      body: JSON.stringify(next),
    });
  }

  async function addThread() {
    if (!studio.activeProject || !newThreadTitle.trim()) return;
    const next = [
      ...studio.threads,
      {
        id: `th-${Date.now()}`,
        title: newThreadTitle.trim(),
        type: 'mystery',
        status: 'planted',
        planted_chapter: studio.activeChapterId || studio.chapters[0]?.id || 'ch-001',
        payoff_chapter: '',
        related_characters: [],
        notes: newThreadNotes.trim(),
      },
    ];
    studio.threads = await api(`/api/projects/${studio.activeProject.id}/continuity/threads`, {
      method: 'PUT',
      body: JSON.stringify(next),
    });
    newThreadTitle = '';
    newThreadNotes = '';
  }

  async function updateThreadStatus(id: string, status: string) {
    if (!studio.activeProject) return;
    const next = studio.threads.map((t) => (t.id === id ? { ...t, status } : t));
    studio.threads = await api(`/api/projects/${studio.activeProject.id}/continuity/threads`, {
      method: 'PUT',
      body: JSON.stringify(next),
    });
  }

  async function deleteThread(id: string) {
    if (!studio.activeProject) return;
    const next = studio.threads.filter((t) => t.id !== id);
    studio.threads = await api(`/api/projects/${studio.activeProject.id}/continuity/threads`, {
      method: 'PUT',
      body: JSON.stringify(next),
    });
  }

  async function addState() {
    if (!studio.activeProject) return;
    const cid = newStateCharId || studio.characters[0]?.id;
    if (!cid) return;
    const splitList = (raw: string) =>
      raw
        .split(/[,;\n]/)
        .map((s) => s.trim())
        .filter(Boolean);
    const next = [
      ...studio.states,
      {
        chapter_id: studio.activeChapterId || studio.chapters[0]?.id || 'ch-001',
        character_id: cid,
        location: newStateLocation.trim() || '-',
        emotional_state: newStateEmotion.trim() || '-',
        physical_condition: newStatePhysical.trim() || '-',
        inventory: [],
        knows: splitList(newStateKnows),
        unaware_of: splitList(newStateUnaware),
        active_goal: '',
        notes: '',
      },
    ];
    await studio.saveStates(next);
    newStateLocation = '';
    newStatePhysical = '';
    newStateEmotion = '';
    newStateKnows = '';
    newStateUnaware = '';
  }

  async function deleteState(index: number) {
    const next = studio.states.filter((_, i) => i !== index);
    await studio.saveStates(next);
  }
</script>

<div class="flex h-full flex-col overflow-hidden" style="background: var(--bg);">
  <div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b px-4 py-2.5" style="border-color: var(--border); background: var(--surface);">
    <div class="flex flex-wrap gap-1.5">
      <button
        onclick={() => (tab = 'timeline')}
        class="flex items-center gap-1 rounded-lg px-2.5 py-1.5 text-xs font-semibold"
        style="background: {tab === 'timeline' ? 'var(--accent)' : 'transparent'}; color: {tab === 'timeline' ? 'var(--accent-ink)' : 'var(--muted)'};"
      >
        <Clock size={13} />
        <span>Timeline ({studio.timeline.length})</span>
      </button>
      <button
        onclick={() => (tab = 'threads')}
        class="flex items-center gap-1 rounded-lg px-2.5 py-1.5 text-xs font-semibold"
        style="background: {tab === 'threads' ? 'var(--accent)' : 'transparent'}; color: {tab === 'threads' ? 'var(--accent-ink)' : 'var(--muted)'};"
      >
        <GitBranch size={13} />
        <span>Plot Threads ({studio.threads.length})</span>
      </button>
      <button
        onclick={() => (tab = 'states')}
        class="flex items-center gap-1 rounded-lg px-2.5 py-1.5 text-xs font-semibold"
        style="background: {tab === 'states' ? 'var(--accent)' : 'transparent'}; color: {tab === 'states' ? 'var(--accent-ink)' : 'var(--muted)'};"
      >
        <UserCheck size={13} />
        <span>States ({studio.states.length})</span>
      </button>
    </div>

    <button
      onclick={runAudit}
      class="flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs font-semibold"
      style="border-color: var(--accent); color: var(--accent);"
    >
      <ShieldAlert size={13} />
      <span>{studio.lang === 'th' ? 'ตรวจความต่อเนื่องทันที' : 'Run Continuity Audit'}</span>
    </button>
  </div>

  {#if auditResult}
    <div class="anim-fade shrink-0 border-b px-4 py-3 text-xs" style="border-color: var(--border); background: var(--surface-elevated);">
      <div class="mb-1 flex items-center justify-between gap-2 font-semibold">
        <span>Audit Complete • {auditResult.chapters_scanned} chapters • {auditResult.open_threads} open threads</span>
        <button onclick={() => (auditResult = null)} class="rounded p-1 hover:opacity-75" aria-label="Close">
          <X size={14} />
        </button>
      </div>
      {#if !auditResult.findings || auditResult.findings.length === 0}
        <p style="color: var(--success);">ไม่พบปัญหาคำซ้ำซาก AI Slop หรือความไม่สอดคล้องในตอนนี้</p>
      {:else}
        <ul class="list-disc space-y-1 pl-4" style="color: var(--warning);">
          {#each auditResult.findings as f}
            <li>{f}</li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}

  <div class="min-h-0 flex-1 space-y-3 overflow-y-auto p-4 text-xs">
    {#if tab === 'timeline'}
      <div class="space-y-2 rounded-xl border p-3" style="border-color: var(--border); background: var(--surface-elevated);">
        <div class="grid gap-2 sm:grid-cols-2">
          <input
            type="text"
            bind:value={newEventDay}
            placeholder={studio.lang === 'th' ? 'วัน/เวลา (เช่น วันที่ 14 ค่ำ)' : 'Day / Time label'}
            class="rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg); color: var(--ink);"
          />
          <input
            type="text"
            bind:value={newEventLocation}
            placeholder={studio.lang === 'th' ? 'สถานที่' : 'Location'}
            class="rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg); color: var(--ink);"
          />
        </div>
        <div class="flex flex-wrap gap-2">
          <input
            type="text"
            bind:value={newEventSummary}
            onkeydown={(e) => {
              if (e.key === 'Enter') addTimelineEvent();
            }}
            placeholder={studio.lang === 'th' ? 'สรุปเหตุการณ์ที่เกิดขึ้น *' : 'Chronological event summary *'}
            class="min-w-[180px] flex-1 rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg); color: var(--ink);"
          />
          <input
            type="text"
            bind:value={newEventImpact}
            onkeydown={(e) => {
              if (e.key === 'Enter') addTimelineEvent();
            }}
            placeholder={studio.lang === 'th' ? 'ผลกระทบต่อเรื่อง (ไม่บังคับ)' : 'Story impact (optional)'}
            class="min-w-[140px] flex-1 rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg); color: var(--ink);"
          />
          <button
            onclick={addTimelineEvent}
            disabled={!newEventSummary.trim()}
            class="flex items-center gap-1 rounded-lg px-3 py-1.5 font-semibold disabled:opacity-40"
            style="background: var(--accent); color: var(--accent-ink);"
          >
            <Plus size={13} />
            <span>{studio.lang === 'th' ? 'เพิ่ม' : 'Add'}</span>
          </button>
        </div>
      </div>

      {#each studio.timeline as ev}
        <div class="rounded-xl border p-3" style="border-color: var(--border); background: var(--surface-elevated);">
          <div class="mb-1 flex items-center justify-between gap-2 font-semibold">
            <span style="color: var(--accent);">{ev.day_label} • {ev.location}</span>
            <div class="flex items-center gap-1.5">
              <span class="rounded px-1.5 py-0.5 text-[10px]" style="background: var(--surface); color: var(--muted);">
                {chapterTitle(ev.chapter_id)}
              </span>
              <button
                onclick={() => deleteTimelineEvent(ev.id)}
                class="rounded p-1 opacity-60 hover:opacity-100"
                style="color: var(--danger);"
                title={studio.lang === 'th' ? 'ลบเหตุการณ์' : 'Delete event'}
              >
                <Trash2 size={12} />
              </button>
            </div>
          </div>
          <p class="text-sm">{ev.summary}</p>
          {#if ev.impact}
            <p class="mt-1" style="color: var(--muted);">Impact: {ev.impact}</p>
          {/if}
        </div>
      {/each}
    {:else if tab === 'threads'}
      <div class="space-y-2 rounded-xl border p-3" style="border-color: var(--border); background: var(--surface-elevated);">
        <div class="flex flex-wrap gap-2">
          <input
            type="text"
            bind:value={newThreadTitle}
            onkeydown={(e) => {
              if (e.key === 'Enter') addThread();
            }}
            placeholder={studio.lang === 'th' ? 'ชื่อปมเรื่อง / Foreshadowing ใหม่ *' : 'New plot thread or foreshadowing *'}
            class="min-w-[180px] flex-1 rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg); color: var(--ink);"
          />
          <input
            type="text"
            bind:value={newThreadNotes}
            onkeydown={(e) => {
              if (e.key === 'Enter') addThread();
            }}
            placeholder={studio.lang === 'th' ? 'รายละเอียด / แผนเฉลย (ไม่บังคับ)' : 'Notes / payoff plan (optional)'}
            class="min-w-[160px] flex-1 rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg); color: var(--ink);"
          />
          <button
            onclick={addThread}
            disabled={!newThreadTitle.trim()}
            class="flex items-center gap-1 rounded-lg px-3 py-1.5 font-semibold disabled:opacity-40"
            style="background: var(--accent); color: var(--accent-ink);"
          >
            <Plus size={13} />
            <span>{studio.lang === 'th' ? 'เพิ่ม' : 'Add'}</span>
          </button>
        </div>
      </div>

      {#each studio.threads as th}
        <div class="rounded-xl border p-3" style="border-color: var(--border); background: var(--surface-elevated);">
          <div class="mb-1 flex items-center justify-between gap-2">
            <span class="text-sm font-semibold">{th.title}</span>
            <div class="flex items-center gap-1.5">
              <select
                value={th.status}
                onchange={(e) => updateThreadStatus(th.id, e.currentTarget.value)}
                class="rounded-lg border px-2 py-1 text-xs outline-none"
                style="border-color: var(--border); background: var(--bg);"
              >
                <option value="planted">planted</option>
                <option value="active">active</option>
                <option value="escalated">escalated</option>
                <option value="paid_off">paid_off</option>
              </select>
              <button
                onclick={() => deleteThread(th.id)}
                class="rounded p-1 opacity-60 hover:opacity-100"
                style="color: var(--danger);"
                title={studio.lang === 'th' ? 'ลบปมเรื่อง' : 'Delete thread'}
              >
                <Trash2 size={12} />
              </button>
            </div>
          </div>
          {#if th.notes}
            <p style="color: var(--muted);">{th.notes}</p>
          {/if}
        </div>
      {/each}
    {:else}
      {#if studio.characters.length > 0}
        <div class="space-y-2 rounded-xl border p-3" style="border-color: var(--border); background: var(--surface-elevated);">
          <div class="font-semibold" style="color: var(--ink);">
            {studio.lang === 'th' ? 'บันทึกสถานะตัวละคร (ณ ตอนที่เปิดอยู่)' : 'Log Character State (Current Chapter)'}
          </div>
          <div class="grid gap-2 sm:grid-cols-3">
            <select
              value={newStateCharId || studio.characters[0]?.id || ''}
              onchange={(e) => (newStateCharId = e.currentTarget.value)}
              class="rounded-lg border px-2.5 py-1.5 outline-none"
              style="border-color: var(--border); background: var(--bg); color: var(--ink);"
            >
              {#each studio.characters as c}
                <option value={c.id}>{c.name}</option>
              {/each}
            </select>
            <input
              type="text"
              bind:value={newStateLocation}
              placeholder={studio.lang === 'th' ? 'ตำแหน่ง / สถานที่' : 'Location'}
              class="rounded-lg border px-2.5 py-1.5 outline-none"
              style="border-color: var(--border); background: var(--bg); color: var(--ink);"
            />
            <input
              type="text"
              bind:value={newStateEmotion}
              placeholder={studio.lang === 'th' ? 'สภาพอารมณ์' : 'Emotional state'}
              class="rounded-lg border px-2.5 py-1.5 outline-none"
              style="border-color: var(--border); background: var(--bg); color: var(--ink);"
            />
          </div>
          <div class="grid gap-2 sm:grid-cols-3">
            <input
              type="text"
              bind:value={newStatePhysical}
              placeholder={studio.lang === 'th' ? 'สภาพร่างกาย / บาดแผล' : 'Physical condition'}
              class="rounded-lg border px-2.5 py-1.5 outline-none"
              style="border-color: var(--border); background: var(--bg); color: var(--ink);"
            />
            <input
              type="text"
              bind:value={newStateKnows}
              placeholder={studio.lang === 'th' ? 'สิ่งที่รู้ (คั่นด้วย ,)' : 'Knows (comma-separated)'}
              class="rounded-lg border px-2.5 py-1.5 outline-none"
              style="border-color: var(--border); background: var(--bg); color: var(--ink);"
            />
            <input
              type="text"
              bind:value={newStateUnaware}
              placeholder={studio.lang === 'th' ? 'สิ่งที่ยังไม่รู้ (คั่นด้วย ,)' : 'Unaware of (comma-separated)'}
              class="rounded-lg border px-2.5 py-1.5 outline-none"
              style="border-color: var(--border); background: var(--bg); color: var(--ink);"
            />
          </div>
          <div class="flex justify-end">
            <button
              onclick={addState}
              class="flex items-center gap-1 rounded-lg px-3 py-1.5 font-semibold"
              style="background: var(--accent); color: var(--accent-ink);"
            >
              <Plus size={13} />
              <span>{studio.lang === 'th' ? 'เพิ่มสถานะ' : 'Add State'}</span>
            </button>
          </div>
        </div>
      {/if}

      {#each studio.states as st, idx}
        <div class="rounded-xl border p-3" style="border-color: var(--border); background: var(--surface-elevated);">
          <div class="mb-1 flex items-center justify-between gap-2 font-semibold">
            <span style="color: var(--accent);">{charName(st.character_id)} @ {chapterTitle(st.chapter_id)} — {st.location}</span>
            <button
              onclick={() => deleteState(idx)}
              class="rounded p-1 opacity-60 hover:opacity-100"
              style="color: var(--danger);"
              title={studio.lang === 'th' ? 'ลบสถานะนี้' : 'Delete state'}
            >
              <Trash2 size={12} />
            </button>
          </div>
          <div><strong>Physical:</strong> {st.physical_condition} | <strong>Emotion:</strong> {st.emotional_state}</div>
          <div class="mt-1" style="color: var(--success);"><strong>Knows:</strong> {(st.knows || []).join(' • ') || '-'}</div>
          <div style="color: var(--danger);"><strong>Unaware Of:</strong> {(st.unaware_of || []).join(' • ') || '-'}</div>
        </div>
      {/each}
    {/if}
  </div>
</div>

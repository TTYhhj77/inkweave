<script lang="ts">
  import { untrack } from 'svelte';
  import { Plus, Sparkles, Save, Trash2, TrendingDown } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';
  import type { Character, CharacterArcPhase } from '../types';

  let {
    character,
    onUpdate,
    onDelete,
  }: {
    character: Character;
    onUpdate: (c: Character) => void;
    onDelete?: (c: Character) => void;
  } = $props();
  let tab = $state<'overview' | 'psyche' | 'arc' | 'state' | 'ties'>('psyche');
  let draft = $state<Character>({} as Character);

  let newTieTarget = $state('');
  let newTieLabel = $state('');
  let newTieTrust = $state(20);
  let newTieTension = $state(40);
  let newTiePublic = $state('');
  let newTieHidden = $state('');

  $effect(() => {
    const snap = $state.snapshot(character);
    untrack(() => {
      draft = structuredClone(snap);
    });
  });

  function save() {
    onUpdate(draft);
  }

  function addArcPhase() {
    const nextOrder = (draft.arc_phases?.length || 0) + 1;
    const phase: CharacterArcPhase = {
      id: `phase-${Date.now()}`,
      phase_order: nextOrder,
      label: `${nextOrder}. จุดเปลี่ยนใหม่`,
      chapter_ref: studio.activeChapterId || 'ch-001',
      trajectory_value: 0,
      internal_shift: '',
      trigger_event: '',
      is_regression: false,
    };
    draft.arc_phases = [...(draft.arc_phases || []), phase];
  }

  function removeArcPhase(idx: number) {
    draft.arc_phases = draft.arc_phases.filter((_, i) => i !== idx);
  }

  function stressTestWithAI() {
    studio.rightDeskOpen = true;
    studio.mobileTab = 'agent';
    studio.sendAgentPrompt(
      studio.lang === 'th'
        ? `วิเคราะห์ความซับซ้อนและทดสอบความย้อนแย้ง (Stress-Test) ของตัวละคร "${draft.name}" พร้อมเสนอจุดถดถอย (Relapse) แบบไม่เป็นเส้นตรง`
        : `Stress-test "${draft.name}" psychology, core paradox, blind spot, and propose a non-linear relapse beat.`,
      'character-architect',
    );
  }

  let charStates = $derived(studio.states.filter((s) => s.character_id === draft.id));
  let outgoingTies = $derived(studio.relationships.filter((r) => r.from_char_id === draft.id));
  let incomingTies = $derived(studio.relationships.filter((r) => r.to_char_id === draft.id));

  // SVG Non-Linear Trajectory Curve coordinates
  let curvePoints = $derived.by(() => {
    const phases = [...(draft.arc_phases || [])].sort((a, b) => a.phase_order - b.phase_order);
    if (phases.length === 0) return { points: [], pathD: '' };
    const w = 480;
    const h = 150;
    const padX = 36;
    const padY = 20;
    const spanX = w - padX * 2;
    const spanY = h - padY * 2;
    const pts = phases.map((p, idx) => {
      const x = phases.length === 1 ? w / 2 : padX + (idx / (phases.length - 1)) * spanX;
      const clamped = Math.max(-100, Math.min(100, p.trajectory_value));
      const y = padY + ((100 - clamped) / 200) * spanY;
      return { x, y, phase: p };
    });
    let d = `M ${pts[0].x} ${pts[0].y}`;
    for (let i = 1; i < pts.length; i++) {
      const prev = pts[i - 1];
      const curr = pts[i];
      const cx = (prev.x + curr.x) / 2;
      d += ` C ${cx} ${prev.y}, ${cx} ${curr.y}, ${curr.x} ${curr.y}`;
    }
    return { points: pts, pathD: d };
  });

  function nameOfChar(id: string) {
    return studio.characters.find((c) => c.id === id)?.name || id;
  }

  let otherChars = $derived(studio.characters.filter((c) => c.id !== draft.id));

  async function addRelationship() {
    const targetId = newTieTarget || otherChars[0]?.id;
    if (!targetId || !newTieLabel.trim()) return;
    const next = [
      ...studio.relationships,
      {
        id: `rel-${Date.now()}`,
        from_char_id: draft.id,
        to_char_id: targetId,
        label: newTieLabel.trim(),
        trust: Number(newTieTrust),
        tension: Number(newTieTension),
        power_dynamic: '',
        public_face: newTiePublic.trim(),
        hidden_agenda: newTieHidden.trim(),
        updated_chapter: studio.activeChapterId || '',
      },
    ];
    await studio.saveRelationships(next);
    newTieLabel = '';
    newTiePublic = '';
    newTieHidden = '';
  }

  async function removeRelationship(id: string) {
    const next = studio.relationships.filter((r) => r.id !== id);
    await studio.saveRelationships(next);
  }
</script>

<div class="flex h-full flex-col overflow-hidden" style="background: var(--bg);">
  <!-- Header -->
  <div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b px-4 py-3" style="border-color: var(--border);">
    <div>
      <h3 class="text-base font-semibold">{draft.name}</h3>
      <p class="text-xs" style="color: var(--muted);">
        {draft.role} • Arc: {draft.arc_type}
      </p>
    </div>
    <div class="flex flex-wrap items-center gap-2">
      <button
        onclick={stressTestWithAI}
        class="flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs font-medium"
        style="border-color: var(--accent); color: var(--accent);"
      >
        <Sparkles size={13} />
        <span>{studio.t.character.deepenAI}</span>
      </button>
      {#if onDelete}
        <button
          onclick={() => onDelete(character)}
          class="flex items-center gap-1 rounded-lg border px-2.5 py-1.5 text-xs font-medium"
          style="border-color: var(--border); color: var(--danger);"
          title={studio.lang === 'th' ? 'ลบตัวละครนี้' : 'Delete character'}
        >
          <Trash2 size={13} />
          <span>{studio.lang === 'th' ? 'ลบ' : 'Delete'}</span>
        </button>
      {/if}
      <button
        onclick={save}
        class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-semibold"
        style="background: var(--accent); color: var(--accent-ink);"
      >
        <Save size={13} />
        <span>{studio.lang === 'th' ? 'บันทึก' : 'Save'}</span>
      </button>
    </div>
  </div>

  <!-- 5 Depth Tabs -->
  <div class="flex shrink-0 flex-wrap gap-1 border-b px-4 pt-2" style="border-color: var(--border); background: var(--surface);">
    {#each [
      ['psyche', studio.t.character.psyche],
      ['arc', studio.t.character.arc],
      ['overview', studio.t.character.overview],
      ['state', studio.t.character.state],
      ['ties', studio.t.character.ties],
    ] as [id, label]}
      <button
        onclick={() => (tab = id as typeof tab)}
        class="shrink-0 border-b-2 px-3 py-2 text-xs font-semibold transition-colors"
        style="
          border-color: {tab === id ? 'var(--accent)' : 'transparent'};
          color: {tab === id ? 'var(--accent)' : 'var(--muted)'};
        "
      >
        {label}
      </button>
    {/each}
  </div>

  <!-- Tab Body -->
  <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4 text-sm">
    {#if tab === 'psyche'}
      <div class="grid gap-3 md:grid-cols-2">
        <label class="block md:col-span-2">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--accent);">{studio.t.character.coreParadox}</span>
          <textarea
            bind:value={draft.core_paradox}
            rows={2}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
          ></textarea>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">{studio.t.character.mask}</span>
          <textarea
            bind:value={draft.mask}
            rows={3}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
          ></textarea>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">{studio.t.character.trueSelf}</span>
          <textarea
            bind:value={draft.true_self}
            rows={3}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
          ></textarea>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">{studio.t.character.wound}</span>
          <textarea
            bind:value={draft.formative_wound}
            rows={2}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
          ></textarea>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">{studio.t.character.misbelief}</span>
          <textarea
            bind:value={draft.misbelief}
            rows={2}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
          ></textarea>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">{studio.t.character.want}</span>
          <input
            type="text"
            bind:value={draft.external_want}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">{studio.t.character.need}</span>
          <input
            type="text"
            bind:value={draft.internal_need}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
          />
        </label>
        <label class="block md:col-span-2">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">{studio.t.character.blindSpot}</span>
          <input
            type="text"
            bind:value={draft.blind_spot}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
          />
        </label>
      </div>
    {:else if tab === 'arc'}
      <div class="space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <label class="flex items-center gap-2 text-xs font-medium">
            <span>Arc Pattern:</span>
            <select
              bind:value={draft.arc_type}
              class="rounded-lg border px-2.5 py-1.5 text-xs outline-none"
              style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
            >
              <option value="spiral">Spiral (ก้าวหน้าแล้วถอยวนซ้ำก่อนพ้น)</option>
              <option value="oscillating">Oscillating (แกว่งขึ้น-ลงตามแรงกดดัน)</option>
              <option value="fractured">Fractured (แตกหักหลายมิติ)</option>
              <option value="tragic">Tragic / Corruption (เสื่อมถอย)</option>
              <option value="redemption">Redemption (ไถ่ถอน)</option>
              <option value="flat">Flat / Steadfast (แก่นเดิมแต่เปลี่ยนโลก)</option>
              <option value="positive">Positive Growth</option>
              <option value="negative">Negative Descent</option>
            </select>
          </label>
          <button
            onclick={addArcPhase}
            class="flex items-center gap-1 rounded-lg px-2.5 py-1.5 text-xs font-semibold"
            style="background: var(--accent-soft); color: var(--accent);"
          >
            <Plus size={13} />
            {studio.t.character.addPhase}
          </button>
        </div>

        <!-- Non-Linear SVG Curve -->
        <div class="rounded-xl border p-3" style="border-color: var(--border); background: var(--surface-elevated);">
          <div class="mb-1 flex items-center justify-between text-[11px]" style="color: var(--muted);">
            <span>Non-Linear Psychological Trajectory (-100..+100)</span>
            <span class="flex items-center gap-1" style="color: var(--danger);">● Relapse / Regression</span>
          </div>
          <svg viewBox="0 0 480 150" class="h-36 w-full">
            <line x1="20" y1="75" x2="460" y2="75" stroke="var(--border)" stroke-dasharray="4 4" />
            {#if curvePoints.pathD}
              <path d={curvePoints.pathD} fill="none" stroke="var(--accent)" stroke-width="2.5" />
            {/if}
            {#each curvePoints.points as pt}
              <circle
                cx={pt.x}
                cy={pt.y}
                r="6"
                fill={pt.phase.is_regression ? 'var(--danger)' : 'var(--accent)'}
                stroke="var(--bg)"
                stroke-width="2"
              />
              <text x={pt.x} y={pt.y - 10} text-anchor="middle" font-size="9" fill="var(--muted)">
                {pt.phase.trajectory_value}
              </text>
            {/each}
          </svg>
        </div>

        <!-- Arc Phases List -->
        <div class="space-y-2.5">
          {#each draft.arc_phases || [] as phase, i}
            <div
              class="rounded-xl border p-3 text-xs"
              style="border-color: {phase.is_regression ? 'var(--danger)' : 'var(--border)'}; background: var(--surface-elevated);"
            >
              <div class="mb-2 flex items-center justify-between gap-2">
                <input
                  type="text"
                  bind:value={phase.label}
                  class="flex-1 bg-transparent text-xs font-semibold outline-none"
                  style="color: var(--ink);"
                />
                <label class="flex items-center gap-1 text-[11px]" style="color: var(--danger);">
                  <input type="checkbox" bind:checked={phase.is_regression} />
                  <TrendingDown size={12} />
                  <span>Relapse</span>
                </label>
                <button onclick={() => removeArcPhase(i)} class="p-1 opacity-60 hover:opacity-100">
                  <Trash2 size={13} />
                </button>
              </div>
              <div class="grid gap-2 sm:grid-cols-3">
                <label class="block">
                  <span class="text-[10px]" style="color: var(--muted);">Chapter Ref</span>
                  <input
                    type="text"
                    bind:value={phase.chapter_ref}
                    class="mt-0.5 w-full rounded border bg-transparent px-2 py-1 outline-none"
                    style="border-color: var(--border);"
                  />
                </label>
                <label class="block sm:col-span-2">
                  <span class="text-[10px]" style="color: var(--muted);">Trajectory (-100 to +100): {phase.trajectory_value}</span>
                  <input
                    type="range"
                    min="-100"
                    max="100"
                    bind:value={phase.trajectory_value}
                    class="mt-1.5 w-full accent-[var(--accent)]"
                  />
                </label>
              </div>
              <input
                type="text"
                bind:value={phase.internal_shift}
                placeholder="Internal shift..."
                class="mt-2 w-full rounded border bg-transparent px-2 py-1 outline-none"
                style="border-color: var(--border);"
              />
            </div>
          {/each}
        </div>
      </div>
    {:else if tab === 'overview'}
      <div class="grid gap-3">
        <div class="grid gap-3 sm:grid-cols-2">
          <label class="block">
            <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">Name</span>
            <input
              type="text"
              bind:value={draft.name}
              class="w-full rounded-xl border p-2.5 text-sm outline-none"
              style="border-color: var(--border); background: var(--surface-elevated);"
            />
          </label>
          <label class="block">
            <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">Role</span>
            <input
              type="text"
              bind:value={draft.role}
              class="w-full rounded-xl border p-2.5 text-sm outline-none"
              style="border-color: var(--border); background: var(--surface-elevated);"
            />
          </label>
        </div>
        <label class="block">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">Voice Guide</span>
          <textarea
            bind:value={draft.voice_guide}
            rows={2}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated);"
          ></textarea>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">Avoids Saying</span>
          <input
            type="text"
            bind:value={draft.avoids_saying}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated);"
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">Summary & Backstory</span>
          <textarea
            bind:value={draft.summary}
            rows={3}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated);"
          ></textarea>
        </label>
      </div>
    {:else if tab === 'state'}
      {#if charStates.length === 0}
        <p class="text-xs" style="color: var(--muted);">ยังไม่มีบันทึก Character State สำหรับตัวละครนี้</p>
      {:else}
        <div class="space-y-3">
          {#each charStates as st}
            <div class="rounded-xl border p-3.5 text-xs" style="border-color: var(--border); background: var(--surface-elevated);">
              <div class="mb-2 font-semibold" style="color: var(--accent);">Chapter: {st.chapter_id} • {st.location}</div>
              <div class="grid gap-1.5">
                <div><strong>Emotion:</strong> {st.emotional_state}</div>
                <div><strong>Physical:</strong> {st.physical_condition}</div>
                <div><strong>Inventory:</strong> {(st.inventory || []).join(', ') || '-'}</div>
                <div style="color: var(--success);"><strong>Knows:</strong> {(st.knows || []).join(' • ') || '-'}</div>
                <div style="color: var(--danger);"><strong>Unaware Of:</strong> {(st.unaware_of || []).join(' • ') || '-'}</div>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    {:else if tab === 'ties'}
      <div class="space-y-4 text-xs">
        {#if otherChars.length > 0}
          <div class="space-y-2 rounded-xl border p-3" style="border-color: var(--border); background: var(--surface-elevated);">
            <div class="font-semibold" style="color: var(--ink);">
              {studio.lang === 'th' ? `เพิ่มความสัมพันธ์จาก ${draft.name}` : `Add Relationship from ${draft.name}`}
            </div>
            <div class="grid gap-2 sm:grid-cols-2">
              <select
                value={newTieTarget || otherChars[0]?.id || ''}
                onchange={(e) => (newTieTarget = e.currentTarget.value)}
                class="rounded-lg border px-2.5 py-1.5 outline-none"
                style="border-color: var(--border); background: var(--bg); color: var(--ink);"
              >
                {#each otherChars as oc}
                  <option value={oc.id}>→ {oc.name}</option>
                {/each}
              </select>
              <input
                type="text"
                bind:value={newTieLabel}
                placeholder={studio.lang === 'th' ? 'สถานะความสัมพันธ์ (เช่น คู่ปรับ, อดีตเพื่อน)' : 'Relationship label (e.g. Rival, Mentor)'}
                class="rounded-lg border px-2.5 py-1.5 outline-none"
                style="border-color: var(--border); background: var(--bg); color: var(--ink);"
              />
            </div>
            <div class="grid gap-2 sm:grid-cols-2">
              <label class="flex items-center gap-2">
                <span class="w-16 shrink-0" style="color: var(--muted);">Trust: {newTieTrust}</span>
                <input type="range" min="-100" max="100" bind:value={newTieTrust} class="flex-1" />
              </label>
              <label class="flex items-center gap-2">
                <span class="w-20 shrink-0" style="color: var(--muted);">Tension: {newTieTension}%</span>
                <input type="range" min="0" max="100" bind:value={newTieTension} class="flex-1" />
              </label>
            </div>
            <div class="grid gap-2 sm:grid-cols-2">
              <input
                type="text"
                bind:value={newTiePublic}
                placeholder={studio.lang === 'th' ? 'ท่าทีที่แสดงออกต่อหน้า (Public Face)' : 'Public face'}
                class="rounded-lg border px-2.5 py-1.5 outline-none"
                style="border-color: var(--border); background: var(--bg); color: var(--ink);"
              />
              <input
                type="text"
                bind:value={newTieHidden}
                placeholder={studio.lang === 'th' ? 'เจตนาซ่อนเร้น (Hidden Agenda)' : 'Hidden agenda'}
                class="rounded-lg border px-2.5 py-1.5 outline-none"
                style="border-color: var(--border); background: var(--bg); color: var(--ink);"
              />
            </div>
            <div class="flex justify-end">
              <button
                onclick={addRelationship}
                disabled={!newTieLabel.trim()}
                class="flex items-center gap-1 rounded-lg px-3 py-1.5 font-semibold disabled:opacity-40"
                style="background: var(--accent); color: var(--accent-ink);"
              >
                <Plus size={12} />
                <span>{studio.lang === 'th' ? 'เพิ่มความสัมพันธ์' : 'Add Tie'}</span>
              </button>
            </div>
          </div>
        {/if}

        <div>
          <h4 class="mb-2 font-semibold" style="color: var(--accent);">มุมมองของ {draft.name} ต่อผู้อื่น</h4>
          {#if outgoingTies.length === 0}
            <p style="color: var(--muted);">{studio.lang === 'th' ? 'ยังไม่มีข้อมูลความสัมพันธ์ขาออก' : 'No outgoing relationships yet.'}</p>
          {:else}
            {#each outgoingTies as r}
              <div class="mb-2 rounded-xl border p-3" style="border-color: var(--border); background: var(--surface-elevated);">
                <div class="flex items-center justify-between gap-2">
                  <div class="font-semibold">→ {nameOfChar(r.to_char_id)}: {r.label}</div>
                  <button
                    onclick={() => removeRelationship(r.id)}
                    class="rounded p-1 opacity-65 hover:opacity-100"
                    style="color: var(--danger);"
                    title={studio.lang === 'th' ? 'ลบความสัมพันธ์' : 'Delete relationship'}
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
                <div class="mt-1 flex gap-4 tabular-nums" style="color: var(--muted);">
                  <span>Trust: {r.trust}</span>
                  <span>Tension: {r.tension}%</span>
                </div>
                {#if r.public_face}
                  <div class="mt-1"><strong>Public:</strong> {r.public_face}</div>
                {/if}
                {#if r.hidden_agenda}
                  <div style="color: var(--accent);"><strong>Hidden Agenda:</strong> {r.hidden_agenda}</div>
                {/if}
              </div>
            {/each}
          {/if}
        </div>
        <div>
          <h4 class="mb-2 font-semibold" style="color: var(--muted);">มุมมองของผู้อื่นต่อ {draft.name}</h4>
          {#if incomingTies.length === 0}
            <p style="color: var(--muted);">{studio.lang === 'th' ? 'ยังไม่มีข้อมูลความสัมพันธ์ขาเข้า' : 'No incoming relationships yet.'}</p>
          {:else}
            {#each incomingTies as r}
              <div class="mb-2 rounded-xl border p-3" style="border-color: var(--border); background: var(--surface-elevated);">
                <div class="flex items-center justify-between gap-2">
                  <div class="font-semibold">{nameOfChar(r.from_char_id)} → : {r.label}</div>
                  <button
                    onclick={() => removeRelationship(r.id)}
                    class="rounded p-1 opacity-65 hover:opacity-100"
                    style="color: var(--danger);"
                    title={studio.lang === 'th' ? 'ลบความสัมพันธ์' : 'Delete relationship'}
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
                <div class="mt-1 flex gap-4 tabular-nums" style="color: var(--muted);">
                  <span>Trust: {r.trust}</span>
                  <span>Tension: {r.tension}%</span>
                </div>
                {#if r.hidden_agenda}
                  <div style="color: var(--accent);"><strong>Hidden Agenda:</strong> {r.hidden_agenda}</div>
                {/if}
              </div>
            {/each}
          {/if}
        </div>
      </div>
    {/if}
  </div>
</div>

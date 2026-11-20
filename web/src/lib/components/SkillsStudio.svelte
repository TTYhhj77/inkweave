<script lang="ts">
  import { untrack } from 'svelte';
  import { Wand2, Plus, Save, Trash2 } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';
  import { api } from '../api/client';
  import type { Skill } from '../types';

  let selectedId = $state('');
  let draft = $state<Skill | null>(null);

  let selected = $derived(studio.skills.find((s) => s.id === selectedId) || studio.skills[0] || null);

  $effect(() => {
    const snap = selected ? $state.snapshot(selected) : null;
    untrack(() => {
      if (snap) {
        draft = structuredClone(snap);
      }
    });
  });

  function newCustomSkill() {
    draft = {
      id: '',
      name: 'Custom Genre Skill',
      name_th: 'สกิลเฉพาะทางใหม่',
      description: 'Custom instructions for this novel',
      description_th: 'คำสั่งสกิลเฉพาะสำหรับนิยายเรื่องนี้',
      category: 'craft',
      recommended_modes: ['balanced', 'ultra'],
      tools: ['read_chapter', 'get_character_profile', 'edit_chapter_section'],
      built_in: false,
      custom: true,
      instructions: '## Custom Guidelines\n1. ',
    };
  }

  async function saveSkill() {
    if (!studio.activeProject || !draft) return;
    try {
      const list = await api<Skill[]>(`/api/projects/${studio.activeProject.id}/skills`, {
        method: 'POST',
        body: JSON.stringify(draft),
      });
      studio.skills = list;
      studio.notify(studio.lang === 'th' ? 'บันทึก Skill แล้ว' : 'Saved skill');
    } catch (e) {
      studio.notify(String(e));
    }
  }

  async function deleteCustomSkill(id: string) {
    if (!studio.activeProject || !id) return;
    try {
      const list = await api<Skill[]>(`/api/projects/${studio.activeProject.id}/skills/${id}`, {
        method: 'DELETE',
      });
      studio.skills = list;
      selectedId = list[0]?.id || '';
      studio.notify(studio.lang === 'th' ? 'ลบ Custom Skill แล้ว' : 'Deleted custom skill');
    } catch (e) {
      studio.notify(String(e));
    }
  }
</script>

<div class="flex h-full flex-col overflow-hidden @2xl:flex-row" style="background: var(--bg);">
  <div class="max-h-40 w-full shrink-0 overflow-y-auto border-b p-3 @2xl:max-h-none @2xl:w-56 @2xl:border-r @2xl:border-b-0" style="border-color: var(--border); background: var(--surface);">
    <div class="mb-2 flex items-center justify-between">
      <span class="text-xs font-semibold" style="color: var(--muted);">Agent Skills ({studio.skills.length})</span>
      <button
        onclick={newCustomSkill}
        class="flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-semibold"
        style="background: var(--accent); color: var(--accent-ink);"
      >
        <Plus size={12} /> New
      </button>
    </div>
    <div class="flex gap-1.5 overflow-x-auto @2xl:flex-col @2xl:overflow-x-visible">
      {#each studio.skills as sk}
        <button
          onclick={() => (selectedId = sk.id)}
          class="shrink-0 rounded-xl border p-2.5 text-left @2xl:w-full"
          style="
            border-color: {selected?.id === sk.id ? 'var(--accent)' : 'var(--border)'};
            background: {selected?.id === sk.id ? 'var(--accent-soft)' : 'var(--bg)'};
          "
        >
          <div class="flex items-center gap-1.5 text-xs font-semibold">
            <Wand2 size={12} style="color: var(--accent);" />
            <span>{studio.lang === 'th' ? sk.name_th || sk.name : sk.name}</span>
          </div>
          <div class="mt-0.5 text-[11px]" style="color: var(--muted);">
            {sk.category} {sk.custom ? '• custom' : ''}
          </div>
        </button>
      {/each}
    </div>
  </div>

  <div class="min-h-0 flex-1 overflow-y-auto p-4 text-sm">
    {#if draft}
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 class="text-base font-semibold">{studio.lang === 'th' ? draft.name_th || draft.name : draft.name}</h3>
          <p class="text-xs" style="color: var(--muted);">{studio.lang === 'th' ? draft.description_th || draft.description : draft.description}</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button
            onclick={() => {
              if (draft?.id) studio.selectedSkillId = draft.id;
              studio.rightDeskOpen = true;
              studio.mobileTab = 'agent';
            }}
            class="rounded-lg border px-3 py-1.5 text-xs font-semibold"
            style="border-color: var(--accent); color: var(--accent);"
          >
            {studio.lang === 'th' ? 'ใช้งาน Skill นี้' : 'Use in Agent'}
          </button>
          {#if draft.custom && draft.id}
            <button
              onclick={() => deleteCustomSkill(draft!.id)}
              class="flex items-center gap-1 rounded-lg border px-2.5 py-1.5 text-xs font-semibold"
              style="border-color: var(--border); color: var(--danger);"
            >
              <Trash2 size={13} /> Delete
            </button>
          {/if}
          <button
            onclick={saveSkill}
            class="flex items-center gap-1 rounded-lg px-3 py-1.5 text-xs font-semibold"
            style="background: var(--accent); color: var(--accent-ink);"
          >
            <Save size={13} /> Save Override
          </button>
        </div>
      </div>

      {#if studio.lang === 'th'}
        <label class="mb-3 block">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">ชื่อสกิล (ภาษาไทย)</span>
          <input
            type="text"
            bind:value={draft.name_th}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
          />
        </label>
      {:else}
        <label class="mb-3 block">
          <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">Skill Name</span>
          <input
            type="text"
            bind:value={draft.name}
            class="w-full rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
          />
        </label>
      {/if}

      <label class="block">
        <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">Skill Instructions (Markdown)</span>
        <textarea
          bind:value={draft.instructions}
          rows={14}
          class="w-full rounded-xl border p-3 font-mono text-xs leading-relaxed outline-none"
          style="border-color: var(--border); background: var(--surface-elevated);"
        ></textarea>
      </label>
    {/if}
  </div>
</div>

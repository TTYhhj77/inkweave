<script lang="ts">
  import { Users, MapPin, Shield, Gem, Scale, Plus, Trash2 } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';
  import { api } from '../api/client';
  import CharacterAtelier from './CharacterAtelier.svelte';
  import type { Character, LoreEntry } from '../types';

  let subTab = $state<'characters' | 'locations' | 'factions' | 'items' | 'rules'>('characters');
  let selectedCharId = $state('');
  let selectedLoreId = $state('');
  let newName = $state('');

  let selectedChar = $derived(
    studio.characters.find((c) => c.id === selectedCharId) || studio.characters[0] || null,
  );
  let filteredLore = $derived(studio.lore.filter((l) => l.category === subTab));
  let selectedLore = $derived(
    filteredLore.find((l) => l.id === selectedLoreId) || filteredLore[0] || null,
  );

  async function addCharacter() {
    if (!newName.trim() || !studio.activeProject) return;
    const created: Character = {
      id: '',
      slug: '',
      name: newName.trim(),
      aliases: [],
      role: 'ensemble',
      age: '',
      appearance: '',
      tags: [],
      summary: '',
      core_paradox: '',
      mask: '',
      true_self: '',
      blind_spot: '',
      formative_wound: '',
      misbelief: '',
      buried_truth: '',
      external_want: '',
      internal_need: '',
      moral_lines: [],
      breaking_points: [],
      voice_guide: '',
      speech_examples: [],
      avoids_saying: '',
      defense_mechanisms: [],
      secrets: [],
      arc_type: 'spiral',
      arc_phases: [],
      bio_markdown: '',
    };
    await studio.saveCharacter(created);
    newName = '';
    selectedCharId = studio.characters[studio.characters.length - 1]?.id || '';
  }

  async function addLore() {
    if (!newName.trim() || !studio.activeProject || subTab === 'characters') return;
    const saved = await api<LoreEntry>(`/api/projects/${studio.activeProject.id}/lore`, {
      method: 'POST',
      body: JSON.stringify({
        category: subTab,
        name: newName.trim(),
        summary: '',
        content: '',
        aliases: [],
        tags: [],
        attributes: {},
      }),
    });
    studio.lore = [...studio.lore, saved];
    selectedLoreId = saved.id;
    newName = '';
  }

  async function saveLore(item: LoreEntry) {
    if (!studio.activeProject) return;
    const saved = await api<LoreEntry>(`/api/projects/${studio.activeProject.id}/lore/${item.id}`, {
      method: 'PUT',
      body: JSON.stringify(item),
    });
    studio.lore = studio.lore.map((l) => (l.id === saved.id ? saved : l));
    studio.notify(studio.lang === 'th' ? 'บันทึกข้อมูลคลังเรื่องแล้ว' : 'Saved lore entry');
  }

  async function removeCharacter(c: Character) {
    const msg =
      studio.lang === 'th'
        ? `ต้องการลบตัวละคร "${c.name}" ถาวรหรือไม่?`
        : `Permanently delete character "${c.name}"?`;
    if (!window.confirm(msg)) return;
    try {
      await studio.deleteCharacter(c.id);
      if (selectedCharId === c.id) selectedCharId = '';
    } catch (e) {
      studio.notify(String(e));
    }
  }

  async function removeLore(item: LoreEntry) {
    const msg =
      studio.lang === 'th'
        ? `ต้องการลบรายการ "${item.name}" ถาวรหรือไม่?`
        : `Permanently delete "${item.name}"?`;
    if (!window.confirm(msg)) return;
    try {
      await studio.deleteLore(item.category, item.id);
      if (selectedLoreId === item.id) selectedLoreId = '';
    } catch (e) {
      studio.notify(String(e));
    }
  }
</script>

<div class="flex h-full flex-col overflow-hidden" style="background: var(--bg);">
  <!-- Category Strip -->
  <div class="flex shrink-0 flex-wrap items-center gap-1 border-b px-3 py-2" style="border-color: var(--border); background: var(--surface);">
    <button
      onclick={() => (subTab = 'characters')}
      class="flex shrink-0 items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-semibold"
      style="background: {subTab === 'characters' ? 'var(--accent)' : 'transparent'}; color: {subTab === 'characters' ? 'var(--accent-ink)' : 'var(--muted)'};"
    >
      <Users size={13} />
      <span>{studio.lang === 'th' ? 'ตัวละคร' : 'Characters'} ({studio.characters.length})</span>
    </button>
    {#each [
      ['locations', studio.lang === 'th' ? 'สถานที่' : 'Locations', MapPin],
      ['rules', studio.lang === 'th' ? 'กฎโลก' : 'Rules', Scale],
      ['factions', studio.lang === 'th' ? 'องค์กร' : 'Factions', Shield],
      ['items', studio.lang === 'th' ? 'ไอเทม' : 'Items', Gem],
    ] as [id, label, Icon]}
      {@const Comp = Icon as typeof MapPin}
      <button
        onclick={() => (subTab = id as typeof subTab)}
        class="flex shrink-0 items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-semibold"
        style="background: {subTab === id ? 'var(--accent)' : 'transparent'}; color: {subTab === id ? 'var(--accent-ink)' : 'var(--muted)'};"
      >
        <Comp size={13} />
        <span>{label}</span>
      </button>
    {/each}
  </div>

  <!-- Split View: Selector List + Detail Atelier -->
  <div class="flex min-h-0 flex-1 flex-col overflow-hidden @2xl:flex-row">
    <div class="max-h-40 w-full shrink-0 overflow-y-auto border-b p-2.5 @2xl:max-h-none @2xl:w-56 @2xl:border-r @2xl:border-b-0" style="border-color: var(--border); background: var(--surface);">
      <div class="mb-2 flex gap-1.5">
        <input
          type="text"
          bind:value={newName}
          onkeydown={(e) => {
            if (e.key === 'Enter') subTab === 'characters' ? addCharacter() : addLore();
          }}
          placeholder={subTab === 'characters' ? (studio.lang === 'th' ? 'ชื่อตัวละครใหม่...' : 'New character...') : (studio.lang === 'th' ? 'ชื่อรายการใหม่...' : 'New entry...')}
          class="min-w-0 flex-1 rounded-lg border px-2.5 py-1.5 text-xs outline-none"
          style="border-color: var(--border); background: var(--bg); color: var(--ink);"
        />
        <button
          onclick={() => (subTab === 'characters' ? addCharacter() : addLore())}
          class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
          style="background: var(--accent); color: var(--accent-ink);"
          title="Add"
        >
          <Plus size={14} />
        </button>
      </div>

      <div class="flex gap-1.5 overflow-x-auto @2xl:flex-col @2xl:overflow-x-visible">
        {#if subTab === 'characters'}
          {#each studio.characters as c}
            <button
              onclick={() => (selectedCharId = c.id)}
              class="shrink-0 rounded-xl border p-2.5 text-left transition-colors @2xl:w-full"
              style="
                border-color: {selectedChar?.id === c.id ? 'var(--accent)' : 'var(--border)'};
                background: {selectedChar?.id === c.id ? 'var(--accent-soft)' : 'var(--bg)'};
              "
            >
              <div class="text-xs font-semibold">{c.name}</div>
              <div class="mt-0.5 truncate text-[11px]" style="color: var(--muted);">{c.role} • {c.arc_type}</div>
            </button>
          {/each}
        {:else}
          {#each filteredLore as item}
            <button
              onclick={() => (selectedLoreId = item.id)}
              class="shrink-0 rounded-xl border p-2.5 text-left @2xl:w-full"
              style="
                border-color: {selectedLore?.id === item.id ? 'var(--accent)' : 'var(--border)'};
                background: {selectedLore?.id === item.id ? 'var(--accent-soft)' : 'var(--bg)'};
              "
            >
              <div class="text-xs font-semibold">{item.name}</div>
              <div class="mt-0.5 truncate text-[11px]" style="color: var(--muted);">{item.summary || '-'}</div>
            </button>
          {/each}
        {/if}
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-hidden">
      {#if subTab === 'characters'}
        {#if selectedChar}
          <CharacterAtelier
            character={selectedChar}
            onUpdate={(c) => studio.saveCharacter(c)}
            onDelete={(c) => removeCharacter(c)}
          />
        {:else}
          <div class="flex h-full items-center justify-center p-6 text-xs" style="color: var(--muted);">
            สร้างตัวละครตัวแรกเพื่อเริ่มออกแบบมิติและความย้อนแย้ง
          </div>
        {/if}
      {:else if selectedLore}
        <div class="h-full space-y-3 overflow-y-auto p-4 text-sm">
          <div class="flex items-center justify-between gap-2">
            <input
              type="text"
              bind:value={selectedLore.name}
              class="min-w-0 flex-1 bg-transparent text-base font-semibold outline-none"
            />
            <button
              onclick={() => removeLore(selectedLore)}
              class="flex items-center gap-1 rounded-lg border px-2.5 py-1.5 text-xs font-medium"
              style="border-color: var(--border); color: var(--danger);"
              title={studio.lang === 'th' ? 'ลบรายการนี้' : 'Delete entry'}
            >
              <Trash2 size={13} />
              <span>{studio.lang === 'th' ? 'ลบ' : 'Delete'}</span>
            </button>
            <button
              onclick={() => saveLore(selectedLore)}
              class="rounded-lg px-3 py-1.5 text-xs font-semibold"
              style="background: var(--accent); color: var(--accent-ink);"
            >
              {studio.lang === 'th' ? 'บันทึก' : 'Save'}
            </button>
          </div>
          <label class="block">
            <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">Summary</span>
            <textarea
              bind:value={selectedLore.summary}
              rows={2}
              class="w-full rounded-xl border p-2.5 text-sm outline-none"
              style="border-color: var(--border); background: var(--surface-elevated);"
            ></textarea>
          </label>
          <label class="block">
            <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">Details & Rules (Markdown)</span>
            <textarea
              bind:value={selectedLore.content}
              rows={10}
              class="w-full rounded-xl border p-2.5 text-sm leading-relaxed outline-none"
              style="border-color: var(--border); background: var(--surface-elevated);"
            ></textarea>
          </label>
        </div>
      {:else}
        <div class="flex h-full items-center justify-center p-6 text-xs" style="color: var(--muted);">
          ยังไม่มีข้อมูลในหมวดนี้
        </div>
      {/if}
    </div>
  </div>
</div>

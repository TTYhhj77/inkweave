<script lang="ts">
  import { Search, X, BookOpen, Users, MapPin, Clock, GitBranch } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';
  import { api } from '../api/client';

  interface SearchHit {
    doc_id: string;
    kind: string;
    ref_id: string;
    title: string;
    snippet: string;
    score: number;
  }

  let query = $state('');
  let hits = $state<SearchHit[]>([]);
  let loading = $state(false);
  let timer: ReturnType<typeof setTimeout> | null = null;

  function onInput() {
    if (timer) clearTimeout(timer);
    const q = query.trim();
    if (!q || !studio.activeProject) {
      hits = [];
      return;
    }
    timer = setTimeout(async () => {
      if (!studio.activeProject) return;
      loading = true;
      try {
        hits = await api<SearchHit[]>(`/api/projects/${studio.activeProject.id}/search`, {
          method: 'POST',
          body: JSON.stringify({ query: q, limit: 20 }),
        });
      } catch {
        hits = [];
      } finally {
        loading = false;
      }
    }, 180);
  }

  function openHit(hit: SearchHit) {
    studio.searchModalOpen = false;
    if (hit.kind === 'chapter') {
      studio.activeChapterId = hit.ref_id;
      studio.leftSection = 'chapters';
      studio.mobileTab = 'write';
    } else if (hit.kind === 'character' || hit.kind === 'location' || hit.kind === 'faction' || hit.kind === 'item' || hit.kind === 'rule') {
      studio.leftSection = 'bible';
      studio.mobileTab = 'bible';
    } else {
      studio.leftSection = 'continuity';
      studio.mobileTab = 'studio';
    }
  }

  function kindLabel(kind: string): string {
    if (studio.lang !== 'th') return kind;
    const map: Record<string, string> = {
      chapter: 'ตอน',
      character: 'ตัวละคร',
      location: 'สถานที่',
      faction: 'องค์กร',
      item: 'ไอเทม',
      rule: 'กฎโลก',
      timeline: 'ไทม์ไลน์',
      thread: 'ปมเรื่อง',
    };
    return map[kind] || kind;
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      if (studio.activeProject) {
        studio.searchModalOpen = !studio.searchModalOpen;
      }
    } else if (e.key === 'Escape' && studio.searchModalOpen) {
      studio.searchModalOpen = false;
    }
  }}
/>

{#if studio.searchModalOpen && studio.activeProject}
  <div
    class="fixed inset-0 z-50 flex items-start justify-center bg-black/40 p-3 pt-16 sm:p-4 sm:pt-20"
    onclick={(e) => {
      if (e.target === e.currentTarget) studio.searchModalOpen = false;
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape') studio.searchModalOpen = false;
    }}
    tabindex="-1"
    role="dialog"
    aria-modal="true"
  >
    <div
      class="anim-sheet flex max-h-[75dvh] w-full max-w-xl flex-col overflow-hidden rounded-2xl border shadow-2xl"
      style="background: var(--bg); border-color: var(--border); color: var(--ink);"
    >
      <div class="flex shrink-0 items-center gap-2 border-b px-4 py-3" style="border-color: var(--border);">
        <Search size={16} style="color: var(--accent);" />
        <input
          type="text"
          bind:value={query}
          oninput={onInput}
          placeholder={studio.lang === 'th'
            ? `ค้นหาใน "${studio.activeProject.title}" (ตอน, ตัวละคร, คลังเรื่อง, ไทม์ไลน์)...`
            : `Search "${studio.activeProject.title}" (chapters, characters, lore, timeline)...`}
          class="min-w-0 flex-1 bg-transparent text-sm outline-none"
          style="color: var(--ink);"
        />
        <button
          onclick={() => (studio.searchModalOpen = false)}
          class="rounded-lg p-1 hover:opacity-75"
          aria-label="Close"
        >
          <X size={16} />
        </button>
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto p-2.5 text-xs">
        {#if !query.trim()}
          <p class="p-4 text-center" style="color: var(--muted);">
            {studio.lang === 'th' ? 'พิมพ์คำค้นหาได้เลย (รองรับภาษาไทยเต็มรูปแบบ • กด Ctrl+K เปิดได้ทุกหน้า)' : 'Type to search across your entire novel (Ctrl+K).'}
          </p>
        {:else if loading}
          <p class="p-4 text-center" style="color: var(--muted);">
            {studio.lang === 'th' ? 'กำลังค้นหา...' : 'Searching...'}
          </p>
        {:else if hits.length === 0}
          <p class="p-4 text-center" style="color: var(--muted);">
            {studio.lang === 'th' ? 'ไม่พบผลลัพธ์ที่ตรงกับคำค้นหา' : 'No matching results found.'}
          </p>
        {:else}
          <div class="space-y-1">
            {#each hits as hit}
              <button
                onclick={() => openHit(hit)}
                class="flex w-full items-start gap-2.5 rounded-xl border p-2.5 text-left transition-colors hover:opacity-90"
                style="border-color: var(--border); background: var(--surface-elevated);"
              >
                <span class="mt-0.5 shrink-0" style="color: var(--accent);">
                  {#if hit.kind === 'chapter'}
                    <BookOpen size={14} />
                  {:else if hit.kind === 'character'}
                    <Users size={14} />
                  {:else if hit.kind === 'timeline'}
                    <Clock size={14} />
                  {:else if hit.kind === 'thread'}
                    <GitBranch size={14} />
                  {:else}
                    <MapPin size={14} />
                  {/if}
                </span>
                <div class="min-w-0 flex-1">
                  <div class="flex items-center justify-between gap-2">
                    <span class="truncate font-semibold">{hit.title}</span>
                    <span class="shrink-0 rounded px-1.5 py-0.5 text-[10px]" style="background: var(--surface); color: var(--muted);">
                      {kindLabel(hit.kind)}
                    </span>
                  </div>
                  {#if hit.snippet}
                    <p class="mt-0.5 line-clamp-2 text-[11px]" style="color: var(--muted);">{hit.snippet}</p>
                  {/if}
                </div>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}

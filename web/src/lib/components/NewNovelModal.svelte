<script lang="ts">
  import { X, BookPlus } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';

  let { onCreated }: { onCreated?: () => void } = $props();

  let title = $state('');
  let genre = $state('');
  let lang = $state('th');
  let synopsis = $state('');
  let submitting = $state(false);

  const genrePresetsTh = ['แฟนตาซี', 'ดราม่า', 'สืบสวน', 'โรแมนติก', 'ไซไฟ', 'วรรณกรรม'];
  const genrePresetsEn = ['Fantasy', 'Drama', 'Mystery', 'Romance', 'Sci-Fi', 'Literary'];

  async function submit() {
    const cleanTitle = title.trim();
    if (!cleanTitle || submitting) return;
    submitting = true;
    try {
      await studio.createProject(
        cleanTitle,
        genre.trim() || (lang === 'th' ? 'นิยายทั่วไป' : 'Fiction'),
        lang,
        synopsis.trim(),
      );
      title = '';
      genre = '';
      synopsis = '';
      studio.newNovelModalOpen = false;
      onCreated?.();
    } catch (e) {
      studio.notify(String(e));
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape' && studio.newNovelModalOpen) studio.newNovelModalOpen = false;
  }}
/>

{#if studio.newNovelModalOpen}
  <div
    class="fixed inset-0 z-50 flex items-end justify-center bg-black/45 p-0 sm:items-center sm:p-4"
    onclick={(e) => {
      if (e.target === e.currentTarget) studio.newNovelModalOpen = false;
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape') studio.newNovelModalOpen = false;
    }}
    tabindex="-1"
    role="dialog"
    aria-modal="true"
  >
    <div
      class="anim-sheet safe-pb flex max-h-[90dvh] w-full max-w-lg flex-col overflow-hidden rounded-t-2xl border shadow-2xl sm:rounded-2xl"
      style="background: var(--bg); border-color: var(--border); color: var(--ink);"
    >
      <div class="flex shrink-0 items-center justify-between border-b px-5 py-3.5" style="border-color: var(--border);">
        <div class="flex items-center gap-2 text-sm font-semibold">
          <BookPlus size={17} style="color: var(--accent);" />
          <span>{studio.t.library.newNovel}</span>
        </div>
        <button
          onclick={() => (studio.newNovelModalOpen = false)}
          class="rounded-lg p-1.5 hover:opacity-75"
          aria-label="Close"
        >
          <X size={17} />
        </button>
      </div>

      <div class="min-h-0 flex-1 space-y-3.5 overflow-y-auto p-5 text-sm">
        <div>
          <label for="new-novel-title" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
            {studio.lang === 'th' ? 'ชื่อเรื่อง *' : 'Novel Title *'}
          </label>
          <input
            id="new-novel-title"
            type="text"
            bind:value={title}
            onkeydown={(e) => {
              if (e.key === 'Enter') submit();
            }}
            placeholder={studio.lang === 'th' ? 'เช่น เงาแห่งหอนาฬิกา...' : 'e.g. The Clocktower Shadow...'}
            class="w-full rounded-xl border px-3 py-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface); color: var(--ink);"
          />
        </div>

        <div class="grid gap-3 sm:grid-cols-3">
          <div class="sm:col-span-2">
            <label for="new-novel-genre" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
              {studio.lang === 'th' ? 'แนวเรื่อง (Genre)' : 'Genre'}
            </label>
            <input
              id="new-novel-genre"
              type="text"
              bind:value={genre}
              placeholder={studio.lang === 'th' ? 'แฟนตาซี, สืบสวน...' : 'Fantasy, Mystery...'}
              class="w-full rounded-xl border px-3 py-2 text-sm outline-none"
              style="border-color: var(--border); background: var(--surface); color: var(--ink);"
            />
          </div>
          <div>
            <label for="new-novel-lang" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
              {studio.lang === 'th' ? 'ภาษาหลัก' : 'Language'}
            </label>
            <select
              id="new-novel-lang"
              bind:value={lang}
              class="w-full rounded-xl border px-3 py-2 text-sm outline-none"
              style="border-color: var(--border); background: var(--surface); color: var(--ink);"
            >
              <option value="th">ไทย</option>
              <option value="en">English</option>
            </select>
          </div>
        </div>

        <div class="flex flex-wrap gap-1.5">
          {#each studio.lang === 'th' ? genrePresetsTh : genrePresetsEn as preset}
            <button
              type="button"
              onclick={() => (genre = preset)}
              class="rounded-full border px-2.5 py-0.5 text-[11px] transition-colors"
              style="
                border-color: {genre === preset ? 'var(--accent)' : 'var(--border)'};
                background: {genre === preset ? 'var(--accent-soft)' : 'var(--surface)'};
                color: {genre === preset ? 'var(--accent)' : 'var(--muted)'};
              "
            >
              {preset}
            </button>
          {/each}
        </div>

        <div>
          <label for="new-novel-synopsis" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
            {studio.lang === 'th' ? 'เรื่องย่อ / แนวคิดหลัก (ใส่ทีหลังได้)' : 'Synopsis / Premise (optional)'}
          </label>
          <textarea
            id="new-novel-synopsis"
            bind:value={synopsis}
            rows={3}
            placeholder={studio.lang === 'th' ? 'สรุปไอเดียหลักของเรื่องนี้สั้นๆ...' : 'Brief premise or story idea...'}
            class="w-full resize-none rounded-xl border px-3 py-2 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface); color: var(--ink);"
          ></textarea>
        </div>
      </div>

      <div class="flex shrink-0 items-center justify-end gap-2 border-t px-5 py-3" style="border-color: var(--border); background: var(--surface);">
        <button
          onclick={() => (studio.newNovelModalOpen = false)}
          class="rounded-xl border px-3.5 py-2 text-xs font-medium"
          style="border-color: var(--border); color: var(--muted);"
        >
          {studio.lang === 'th' ? 'ยกเลิก' : 'Cancel'}
        </button>
        <button
          onclick={submit}
          disabled={!title.trim() || submitting}
          class="rounded-xl px-4 py-2 text-xs font-semibold transition-opacity disabled:opacity-40"
          style="background: var(--accent); color: var(--accent-ink);"
        >
          {submitting
            ? studio.lang === 'th' ? 'กำลังสร้าง...' : 'Creating...'
            : studio.lang === 'th' ? 'สร้างเรื่องและเริ่มเขียน' : 'Create & Start Writing'}
        </button>
      </div>
    </div>
  </div>
{/if}

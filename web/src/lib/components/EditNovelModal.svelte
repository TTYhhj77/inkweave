<script lang="ts">
  import { untrack } from 'svelte';
  import { X, BookMarked, Save } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';

  let title = $state('');
  let author = $state('');
  let genre = $state('');
  let language = $state('th');
  let pov = $state('');
  let tense = $state('');
  let synopsis = $state('');
  let styleRulesText = $state('');
  let antiClichesText = $state('');
  let saving = $state(false);

  $effect(() => {
    if (studio.editNovelModalOpen && studio.activeProject) {
      const p = $state.snapshot(studio.activeProject);
      untrack(() => {
        title = p.title || '';
        author = p.author || '';
        genre = p.genre || '';
        language = p.language || 'th';
        pov = p.pov || '';
        tense = p.tense || '';
        synopsis = p.synopsis || '';
        styleRulesText = (p.style_rules || []).join('\n');
        antiClichesText = (p.anti_cliches || []).join('\n');
      });
    }
  });

  async function save() {
    if (!title.trim() || saving) return;
    saving = true;
    try {
      await studio.updateProject({
        title: title.trim(),
        author: author.trim(),
        genre: genre.trim(),
        language,
        pov: pov.trim(),
        tense: tense.trim(),
        synopsis: synopsis.trim(),
        style_rules: styleRulesText
          .split('\n')
          .map((s) => s.trim())
          .filter(Boolean),
        anti_cliches: antiClichesText
          .split('\n')
          .map((s) => s.trim())
          .filter(Boolean),
      });
      studio.editNovelModalOpen = false;
    } catch (e) {
      studio.notify(String(e));
    } finally {
      saving = false;
    }
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape' && studio.editNovelModalOpen) studio.editNovelModalOpen = false;
  }}
/>

{#if studio.editNovelModalOpen && studio.activeProject}
  <div
    class="fixed inset-0 z-50 flex items-end justify-center bg-black/45 p-0 sm:items-center sm:p-4"
    onclick={(e) => {
      if (e.target === e.currentTarget) studio.editNovelModalOpen = false;
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape') studio.editNovelModalOpen = false;
    }}
    tabindex="-1"
    role="dialog"
    aria-modal="true"
  >
    <div
      class="anim-sheet safe-pb flex max-h-[90dvh] w-full max-w-2xl flex-col overflow-hidden rounded-t-2xl border shadow-2xl sm:rounded-2xl"
      style="background: var(--bg); border-color: var(--border); color: var(--ink);"
    >
      <div class="flex shrink-0 items-center justify-between border-b px-5 py-3.5" style="border-color: var(--border);">
        <div class="flex items-center gap-2 text-sm font-semibold">
          <BookMarked size={17} style="color: var(--accent);" />
          <span>{studio.lang === 'th' ? 'แก้ไขข้อมูลและ DNA ของเรื่อง' : 'Edit Novel Info & Story DNA'}</span>
        </div>
        <button
          onclick={() => (studio.editNovelModalOpen = false)}
          class="rounded-lg p-1.5 hover:opacity-75"
          aria-label="Close"
        >
          <X size={17} />
        </button>
      </div>

      <div class="min-h-0 flex-1 space-y-3.5 overflow-y-auto p-5 text-sm">
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label for="edit-novel-title" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
              {studio.lang === 'th' ? 'ชื่อเรื่อง *' : 'Novel Title *'}
            </label>
            <input
              id="edit-novel-title"
              type="text"
              bind:value={title}
              class="w-full rounded-xl border px-3 py-2 text-sm outline-none"
              style="border-color: var(--border); background: var(--surface); color: var(--ink);"
            />
          </div>
          <div>
            <label for="edit-novel-author" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
              {studio.lang === 'th' ? 'นามปากกา / ผู้แต่ง' : 'Author'}
            </label>
            <input
              id="edit-novel-author"
              type="text"
              bind:value={author}
              class="w-full rounded-xl border px-3 py-2 text-sm outline-none"
              style="border-color: var(--border); background: var(--surface); color: var(--ink);"
            />
          </div>
        </div>

        <div class="grid gap-3 sm:grid-cols-3">
          <div>
            <label for="edit-novel-genre" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
              {studio.lang === 'th' ? 'แนวเรื่อง' : 'Genre'}
            </label>
            <input
              id="edit-novel-genre"
              type="text"
              bind:value={genre}
              class="w-full rounded-xl border px-3 py-2 text-sm outline-none"
              style="border-color: var(--border); background: var(--surface); color: var(--ink);"
            />
          </div>
          <div>
            <label for="edit-novel-pov" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
              POV
            </label>
            <input
              id="edit-novel-pov"
              type="text"
              bind:value={pov}
              placeholder="Third-person limited"
              class="w-full rounded-xl border px-3 py-2 text-sm outline-none"
              style="border-color: var(--border); background: var(--surface); color: var(--ink);"
            />
          </div>
          <div>
            <label for="edit-novel-lang" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
              {studio.lang === 'th' ? 'ภาษาหลัก' : 'Language'}
            </label>
            <select
              id="edit-novel-lang"
              bind:value={language}
              class="w-full rounded-xl border px-3 py-2 text-sm outline-none"
              style="border-color: var(--border); background: var(--surface); color: var(--ink);"
            >
              <option value="th">ไทย</option>
              <option value="en">English</option>
            </select>
          </div>
        </div>

        <div>
          <label for="edit-novel-synopsis" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
            {studio.lang === 'th' ? 'เรื่องย่อ / แก่นเรื่อง' : 'Synopsis / Premise'}
          </label>
          <textarea
            id="edit-novel-synopsis"
            bind:value={synopsis}
            rows={3}
            class="w-full rounded-xl border px-3 py-2 text-sm outline-none"
            style="border-color: var(--border); background: var(--surface); color: var(--ink);"
          ></textarea>
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label for="edit-novel-style" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
              {studio.lang === 'th' ? 'กฎสไตล์การเล่า (บรรทัดละ 1 ข้อ)' : 'Style Rules (one per line)'}
            </label>
            <textarea
              id="edit-novel-style"
              bind:value={styleRulesText}
              rows={4}
              placeholder={studio.lang === 'th' ? 'เช่น ใช้ภาษาบรรยายที่กระชับ\nเลี่ยงการเฉลยความคิดตัวละครตรงๆ' : 'Show, don\'t tell...'}
              class="w-full rounded-xl border px-3 py-2 text-xs leading-relaxed outline-none"
              style="border-color: var(--border); background: var(--surface); color: var(--ink);"
            ></textarea>
          </div>
          <div>
            <label for="edit-novel-cliches" class="mb-1 block text-xs font-semibold" style="color: var(--muted);">
              {studio.lang === 'th' ? 'คำ/สำนวนต้องห้าม Anti-AI-Slop (บรรทัดละ 1 คำ)' : 'Forbidden Clichés (one per line)'}
            </label>
            <textarea
              id="edit-novel-cliches"
              bind:value={antiClichesText}
              rows={4}
              class="w-full rounded-xl border px-3 py-2 text-xs leading-relaxed outline-none"
              style="border-color: var(--border); background: var(--surface); color: var(--ink);"
            ></textarea>
          </div>
        </div>
      </div>

      <div class="flex shrink-0 items-center justify-end gap-2 border-t px-5 py-3" style="border-color: var(--border); background: var(--surface);">
        <button
          onclick={() => (studio.editNovelModalOpen = false)}
          class="rounded-xl border px-3.5 py-2 text-xs font-medium"
          style="border-color: var(--border); color: var(--muted);"
        >
          {studio.lang === 'th' ? 'ยกเลิก' : 'Cancel'}
        </button>
        <button
          onclick={save}
          disabled={!title.trim() || saving}
          class="flex items-center gap-1.5 rounded-xl px-4 py-2 text-xs font-semibold disabled:opacity-40"
          style="background: var(--accent); color: var(--accent-ink);"
        >
          <Save size={13} />
          <span>{saving ? (studio.lang === 'th' ? 'กำลังบันทึก...' : 'Saving...') : (studio.lang === 'th' ? 'บันทึกข้อมูลเรื่อง' : 'Save Novel Info')}</span>
        </button>
      </div>
    </div>
  </div>
{/if}

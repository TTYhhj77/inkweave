<script lang="ts">
  import { onMount } from 'svelte';
  import {
    BookOpen,
    PenLine,
    Sparkles,
    Users,
    GitBranch,
    Clock,
    Wand2,
    Settings,
    PanelLeft,
    PanelRight,
    Plus,
    Library,
    Menu,
    ChevronDown,
    Check,
    BookPlus,
    Trash2,
    ArrowUp,
    ArrowDown,
    Pencil,
    Search,
  } from '@lucide/svelte';
  import { studio } from './lib/stores/studio.svelte';
  import ManuscriptEditor from './lib/components/ManuscriptEditor.svelte';
  import AgentDesk from './lib/components/AgentDesk.svelte';
  import StoryBible from './lib/components/StoryBible.svelte';
  import ContinuityStudio from './lib/components/ContinuityStudio.svelte';
  import SkillsStudio from './lib/components/SkillsStudio.svelte';
  import GitSyncPanel from './lib/components/GitSyncPanel.svelte';
  import ContextInspector from './lib/components/ContextInspector.svelte';
  import SettingsModal from './lib/components/SettingsModal.svelte';
  import QuickAgentSheet from './lib/components/QuickAgentSheet.svelte';
  import LibraryView from './lib/components/LibraryView.svelte';
  import NewNovelModal from './lib/components/NewNovelModal.svelte';
  import EditNovelModal from './lib/components/EditNovelModal.svelte';
  import SearchModal from './lib/components/SearchModal.svelte';

  let showLibrary = $state(false);
  let novelMenuOpen = $state(false);
  let newChapterTitle = $state('');
  let ready = $state(false);

  onMount(async () => {
    try {
      await studio.bootstrap();
      if (studio.projects.length === 0) {
        showLibrary = true;
      }
    } finally {
      ready = true;
    }
  });

  async function toggleLibrary() {
    novelMenuOpen = false;
    showLibrary = !showLibrary;
    if (showLibrary) {
      await studio.loadProjects();
    }
  }

  async function openNovelMenu() {
    novelMenuOpen = !novelMenuOpen;
    if (novelMenuOpen) {
      await studio.loadProjects();
    }
  }

  async function switchNovel(id: string) {
    novelMenuOpen = false;
    showLibrary = false;
    if (studio.activeProject?.id !== id) {
      await studio.openProject(id);
    }
  }

  function openNewNovelModal() {
    novelMenuOpen = false;
    studio.newNovelModalOpen = true;
  }

  async function addChapter() {
    const title = newChapterTitle.trim() || `บทที่ ${studio.chapters.length + 1}`;
    await studio.addChapter(title);
    newChapterTitle = '';
    studio.leftSection = 'chapters';
    studio.mobileTab = 'write';
  }

  async function removeChapter(e: MouseEvent, id: string, title: string) {
    e.stopPropagation();
    const msg =
      studio.lang === 'th'
        ? `ต้องการลบตอน "${title}" ถาวรหรือไม่?`
        : `Permanently delete chapter "${title}"?`;
    if (!window.confirm(msg)) return;
    try {
      await studio.deleteChapter(id);
    } catch (err) {
      studio.notify(String(err));
    }
  }

  async function moveChapter(e: MouseEvent, index: number, delta: -1 | 1) {
    e.stopPropagation();
    const target = index + delta;
    if (target < 0 || target >= studio.chapters.length) return;
    const ids = studio.chapters.map((c) => c.id);
    [ids[index], ids[target]] = [ids[target], ids[index]];
    try {
      await studio.reorderChapters(ids);
    } catch (err) {
      studio.notify(String(err));
    }
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape' && novelMenuOpen) novelMenuOpen = false;
  }}
/>

<div class="flex h-dvh w-screen flex-col overflow-hidden" style="background: var(--bg); color: var(--ink);">
  <!-- Top Studio Chrome -->
  {#if !studio.focusMode}
    <header
      class="flex h-12 shrink-0 items-center justify-between gap-2 border-b px-3 sm:px-4"
      style="border-color: var(--border); background: var(--surface);"
    >
      <div class="flex min-w-0 flex-1 items-center gap-1.5 sm:gap-2">
        <button
          onclick={() => (studio.leftRailOpen = !studio.leftRailOpen)}
          class="hidden rounded-lg p-1.5 transition-colors hover:opacity-75 md:inline-flex"
          style="color: var(--muted);"
          title="Toggle Sidebar"
        >
          <PanelLeft size={17} />
        </button>

        <!-- Clear Library / All Novels Button (visible on both Desktop & Mobile) -->
        <button
          onclick={toggleLibrary}
          class="flex shrink-0 items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs font-semibold transition-colors"
          style="
            border-color: {showLibrary ? 'var(--accent)' : 'var(--border)'};
            background: {showLibrary ? 'var(--accent-soft)' : 'var(--bg)'};
            color: {showLibrary ? 'var(--accent)' : 'var(--ink)'};
          "
          title={studio.t.library.allNovels}
        >
          <Library size={14} style="color: var(--accent);" />
          <span>{studio.t.library.allNovels}</span>
          {#if studio.projects.length > 0}
            <span
              class="rounded-full px-1.5 py-0.5 text-[10px] leading-none tabular-nums"
              style="background: var(--accent-soft); color: var(--accent);"
            >
              {studio.projects.length}
            </span>
          {/if}
        </button>

        <!-- Active Novel Quick Switcher Dropdown -->
        {#if studio.activeProject && !showLibrary}
          <div class="relative min-w-0">
            <button
              onclick={openNovelMenu}
              class="flex max-w-full items-center gap-1 rounded-lg px-2 py-1 text-xs font-semibold transition-colors sm:max-w-[280px] sm:text-sm"
              style="background: {novelMenuOpen ? 'var(--accent-soft)' : 'transparent'}; color: var(--ink);"
              title={studio.t.library.switchNovel}
            >
              <span class="truncate">{studio.activeProject.title}</span>
              <ChevronDown size={14} class="shrink-0 opacity-65" />
            </button>

            {#if novelMenuOpen}
              <!-- Backdrop to close popover -->
              <div
                class="fixed inset-0 z-30"
                onclick={() => (novelMenuOpen = false)}
                role="presentation"
              ></div>
              <div
                class="anim-fade fixed top-12 left-3 z-40 mt-1 w-72 max-w-[calc(100vw-1.5rem)] overflow-hidden rounded-2xl border shadow-xl sm:absolute sm:top-full sm:left-0"
                style="border-color: var(--border); background: var(--bg); color: var(--ink);"
              >
                <div class="flex items-center justify-between border-b px-3 py-2 text-[11px] font-semibold" style="border-color: var(--border); color: var(--muted);">
                  <span>{studio.t.library.allNovels} ({studio.projects.length})</span>
                  <button
                    onclick={openNewNovelModal}
                    class="flex items-center gap-1 rounded-md px-2 py-0.5 font-semibold"
                    style="background: var(--accent); color: var(--accent-ink);"
                  >
                    <Plus size={12} />
                    <span>{studio.t.library.newNovel}</span>
                  </button>
                </div>

                <div class="max-h-64 space-y-0.5 overflow-y-auto p-1.5">
                  {#each studio.projects as proj}
                    {@const isCurrent = studio.activeProject?.id === proj.id}
                    <button
                      onclick={() => switchNovel(proj.id)}
                      class="flex w-full items-center justify-between gap-2 rounded-xl px-2.5 py-2 text-left transition-colors"
                      style="background: {isCurrent ? 'var(--accent-soft)' : 'transparent'};"
                    >
                      <div class="min-w-0 flex-1">
                        <div class="truncate text-xs font-semibold" style="color: {isCurrent ? 'var(--accent)' : 'var(--ink)'};">
                          {proj.title}
                        </div>
                        <div class="text-[10px] tabular-nums" style="color: var(--muted);">
                          {proj.chapter_count} {studio.t.library.chapters} • {proj.total_words.toLocaleString()} {studio.t.library.words}
                        </div>
                      </div>
                      {#if isCurrent}
                        <Check size={14} class="shrink-0" style="color: var(--accent);" />
                      {/if}
                    </button>
                  {/each}
                </div>

                <div class="flex gap-1 border-t p-1.5" style="border-color: var(--border); background: var(--surface);">
                  <button
                    onclick={() => {
                      novelMenuOpen = false;
                      studio.editNovelModalOpen = true;
                    }}
                    class="flex flex-1 items-center justify-center gap-1 rounded-lg py-1.5 text-xs font-medium"
                    style="color: var(--ink);"
                  >
                    <Pencil size={12} />
                    <span>{studio.lang === 'th' ? 'แก้ไขข้อมูลเรื่อง' : 'Edit Info'}</span>
                  </button>
                  <button
                    onclick={toggleLibrary}
                    class="flex flex-1 items-center justify-center gap-1 rounded-lg py-1.5 text-xs font-medium"
                    style="color: var(--accent);"
                  >
                    <Library size={13} />
                    <span>{studio.lang === 'th' ? 'คลังทั้งหมด' : 'All Novels'}</span>
                  </button>
                </div>
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <div class="flex shrink-0 items-center gap-1 sm:gap-1.5">
        {#if studio.activeProject && !showLibrary}
          <button
            onclick={() => (studio.searchModalOpen = true)}
            class="flex items-center gap-1 rounded-lg px-2 py-1.5 text-xs font-medium transition-colors"
            style="color: var(--muted);"
            title={studio.lang === 'th' ? 'ค้นหาในเรื่อง (Ctrl+K)' : 'Search Novel (Ctrl+K)'}
            aria-label="Search"
          >
            <Search size={15} />
          </button>
        {/if}
        <button
          onclick={openNewNovelModal}
          class="flex items-center gap-1 rounded-lg px-2 py-1.5 text-xs font-semibold transition-colors sm:px-2.5"
          style="background: var(--accent); color: var(--accent-ink);"
          title={studio.t.library.newNovel}
        >
          <BookPlus size={14} />
          <span class="hidden sm:inline">{studio.t.library.newNovel}</span>
        </button>
        {#if !showLibrary}
          <button
            onclick={() => (studio.rightDeskOpen = !studio.rightDeskOpen)}
            class="hidden items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium md:inline-flex"
            style="background: {studio.rightDeskOpen ? 'var(--accent-soft)' : 'transparent'}; color: {studio.rightDeskOpen ? 'var(--accent)' : 'var(--muted)'};"
          >
            <PanelRight size={15} />
            <span>{studio.t.nav.agent}</span>
          </button>
        {/if}
        <button
          onclick={() => (studio.settingsOpen = true)}
          class="flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-xs font-medium transition-colors sm:px-2.5"
          style="color: var(--muted);"
          title={studio.t.nav.settings}
        >
          <Settings size={16} />
          <span class="hidden sm:inline">{studio.t.nav.settings}</span>
        </button>
      </div>
    </header>
  {/if}

  <!-- Main Workspace Body -->
  {#if !ready}
    <div class="flex flex-1 items-center justify-center text-sm" style="color: var(--muted);">
      Loading Inkweave Studio...
    </div>
  {:else if showLibrary || !studio.activeProject}
    <div class="min-h-0 flex-1 overflow-hidden">
      <LibraryView onEnterStudio={() => (showLibrary = false)} />
    </div>
  {:else}
    <!-- DESKTOP 3-PANE STUDIO (md and up) -->
    <div class="hidden min-h-0 flex-1 overflow-hidden md:flex">
      {#if studio.leftRailOpen && !studio.focusMode}
        <aside
          class="flex w-52 shrink-0 flex-col border-r lg:w-60"
          style="border-color: var(--border); background: var(--surface);"
        >
          <!-- Workspace Navigation Pills -->
          <div class="grid shrink-0 grid-cols-5 gap-1 border-b p-2" style="border-color: var(--border);">
            {#each [
              ['chapters', BookOpen, studio.t.nav.write],
              ['bible', Users, studio.t.nav.bible],
              ['continuity', Clock, studio.t.nav.continuity],
              ['skills', Wand2, studio.t.nav.skills],
              ['git', GitBranch, studio.t.nav.git],
            ] as [secId, Icon, tip]}
              {@const Comp = Icon as typeof BookOpen}
              <button
                onclick={() => (studio.leftSection = secId as typeof studio.leftSection)}
                class="flex flex-col items-center justify-center rounded-lg py-1.5 text-[10px] font-medium transition-colors"
                style="
                  background: {studio.leftSection === secId ? 'var(--accent-soft)' : 'transparent'};
                  color: {studio.leftSection === secId ? 'var(--accent)' : 'var(--muted)'};
                "
                title={tip as string}
              >
                <Comp size={15} />
              </button>
            {/each}
          </div>

          <!-- Novel Switcher inside Left Rail (Novels -> Chapters hierarchy) -->
          <div class="shrink-0 border-b p-2.5" style="border-color: var(--border);">
            <div class="mb-1.5 flex items-center justify-between gap-1 text-[10px] font-semibold uppercase tracking-wide" style="color: var(--muted);">
              <span class="truncate">{studio.t.library.currentNovel} ({studio.projects.length})</span>
              <div class="flex shrink-0 items-center gap-1">
                <button
                  onclick={() => (studio.editNovelModalOpen = true)}
                  class="flex items-center justify-center rounded p-1 transition-colors hover:opacity-80"
                  style="color: var(--muted);"
                  title={studio.lang === 'th' ? 'แก้ไขข้อมูลเรื่อง' : 'Edit Novel Info'}
                  aria-label="Edit Novel Info"
                >
                  <Pencil size={11} />
                </button>
                <button
                  onclick={openNewNovelModal}
                  class="flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-semibold normal-case"
                  style="background: var(--accent-soft); color: var(--accent);"
                  title={studio.t.library.newNovel}
                >
                  <Plus size={11} />
                  <span>{studio.lang === 'th' ? 'เรื่องใหม่' : 'New'}</span>
                </button>
              </div>
            </div>
            <select
              value={studio.activeProject?.id || ''}
              onchange={(e) => {
                const id = e.currentTarget.value;
                if (id) switchNovel(id);
              }}
              class="w-full truncate rounded-lg border px-2 py-1.5 text-xs font-semibold outline-none"
              style="border-color: var(--border); background: var(--bg); color: var(--ink);"
            >
              {#each studio.projects as proj}
                <option value={proj.id}>{proj.title}</option>
              {/each}
            </select>
          </div>

          <!-- Chapter List inside Left Rail -->
          <div class="min-h-0 flex-1 overflow-y-auto p-2.5">
            <div class="mb-1.5 text-[10px] font-semibold uppercase tracking-wide" style="color: var(--muted);">
              {studio.t.library.chaptersHeading} ({studio.chapters.length})
            </div>
            <div class="mb-2 flex gap-1.5">
              <input
                type="text"
                bind:value={newChapterTitle}
                onkeydown={(e) => {
                  if (e.key === 'Enter') addChapter();
                }}
                placeholder={studio.t.editor.newChapter}
                class="min-w-0 flex-1 rounded-lg border px-2 py-1 text-xs outline-none"
                style="border-color: var(--border); background: var(--bg); color: var(--ink);"
              />
              <button
                onclick={addChapter}
                class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg"
                style="background: var(--accent); color: var(--accent-ink);"
                title={studio.t.editor.newChapter}
              >
                <Plus size={14} />
              </button>
            </div>
            <div class="space-y-1">
              {#each studio.chapters as ch, idx}
                {@const isCurrentCh = studio.activeChapter?.id === ch.id && studio.leftSection === 'chapters'}
                <div
                  role="button"
                  tabindex="0"
                  onclick={() => {
                    studio.activeChapterId = ch.id;
                    studio.leftSection = 'chapters';
                    studio.refreshContextPreview('');
                  }}
                  onkeydown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      studio.activeChapterId = ch.id;
                      studio.leftSection = 'chapters';
                      studio.refreshContextPreview('');
                    }
                  }}
                  class="group flex w-full cursor-pointer items-center justify-between gap-1 rounded-xl px-2.5 py-2 text-left transition-colors"
                  style="
                    background: {isCurrentCh ? 'var(--surface-elevated)' : 'transparent'};
                    border: 1px solid {isCurrentCh ? 'var(--border)' : 'transparent'};
                  "
                >
                  <div class="min-w-0 flex-1">
                    <div class="truncate text-xs font-semibold">{ch.title}</div>
                    <div class="mt-0.5 flex items-center gap-2 text-[10px] tabular-nums" style="color: var(--muted);">
                      <span>{ch.status}</span>
                      <span>•</span>
                      <span>{ch.word_count.toLocaleString()} {studio.t.library.words}</span>
                    </div>
                  </div>
                  <div class="flex shrink-0 items-center gap-0.5 opacity-60 transition-opacity group-hover:opacity-100 {isCurrentCh ? 'opacity-100' : ''}">
                    <button
                      type="button"
                      onclick={(e) => moveChapter(e, idx, -1)}
                      disabled={idx === 0}
                      class="rounded p-1 transition-opacity disabled:opacity-20"
                      style="color: var(--muted);"
                      title={studio.lang === 'th' ? 'เลื่อนขึ้น' : 'Move up'}
                      aria-label="Move chapter up"
                    >
                      <ArrowUp size={11} />
                    </button>
                    <button
                      type="button"
                      onclick={(e) => moveChapter(e, idx, 1)}
                      disabled={idx === studio.chapters.length - 1}
                      class="rounded p-1 transition-opacity disabled:opacity-20"
                      style="color: var(--muted);"
                      title={studio.lang === 'th' ? 'เลื่อนลง' : 'Move down'}
                      aria-label="Move chapter down"
                    >
                      <ArrowDown size={11} />
                    </button>
                    <button
                      type="button"
                      onclick={(e) => removeChapter(e, ch.id, ch.title)}
                      class="rounded p-1 transition-opacity hover:opacity-100"
                      style="color: var(--danger);"
                      title={studio.lang === 'th' ? 'ลบตอนนี้' : 'Delete chapter'}
                      aria-label="Delete chapter"
                    >
                      <Trash2 size={12} />
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          </div>
        </aside>
      {/if}

      <!-- Center Canvas (switches cleanly between Manuscript Editor, Story Bible, Continuity, Skills, Git) -->
      <main class="@container min-w-0 flex-1 overflow-hidden">
        {#if studio.leftSection === 'chapters'}
          <ManuscriptEditor />
        {:else if studio.leftSection === 'bible'}
          <StoryBible />
        {:else if studio.leftSection === 'continuity'}
          <ContinuityStudio />
        {:else if studio.leftSection === 'skills'}
          <SkillsStudio />
        {:else if studio.leftSection === 'git'}
          <GitSyncPanel />
        {/if}
      </main>

      <!-- Right Agent Desk -->
      {#if studio.rightDeskOpen && !studio.focusMode}
        <aside
          class="w-72 shrink-0 border-l lg:w-80 xl:w-96"
          style="border-color: var(--border);"
        >
          <AgentDesk />
        </aside>
      {/if}
    </div>

    <!-- DEDICATED MOBILE STUDIO LAYOUT (< 768px) -->
    <div class="flex min-h-0 flex-1 flex-col overflow-hidden md:hidden">
      <div class="min-h-0 flex-1 overflow-hidden">
        {#if studio.mobileTab === 'write'}
          <div class="flex h-full flex-col overflow-hidden">
            <!-- Quick Chapter Strip on Mobile -->
            <div class="flex shrink-0 items-center gap-1.5 overflow-x-auto border-b px-3 py-1.5" style="border-color: var(--border); background: var(--surface);">
              {#each studio.chapters as ch}
                <button
                  onclick={() => (studio.activeChapterId = ch.id)}
                  class="shrink-0 rounded-full px-2.5 py-1 text-xs font-medium"
                  style="
                    background: {studio.activeChapter?.id === ch.id ? 'var(--accent)' : 'var(--bg)'};
                    color: {studio.activeChapter?.id === ch.id ? 'var(--accent-ink)' : 'var(--muted)'};
                  "
                >
                  {ch.title}
                </button>
              {/each}
              <button
                onclick={addChapter}
                class="flex shrink-0 items-center gap-1 rounded-full border px-2 py-1 text-xs"
                style="border-color: var(--border);"
              >
                <Plus size={12} />
              </button>
            </div>
            <div class="min-h-0 flex-1 overflow-hidden">
              <ManuscriptEditor />
            </div>
          </div>
        {:else if studio.mobileTab === 'agent'}
          <AgentDesk />
        {:else if studio.mobileTab === 'bible'}
          <StoryBible />
        {:else}
          <!-- Mobile Studio Hub (Continuity / Skills / Git Sync) -->
          <div class="flex h-full flex-col overflow-hidden">
            <div class="flex shrink-0 gap-1 border-b p-2" style="border-color: var(--border); background: var(--surface);">
              {#each [
                ['continuity', studio.t.nav.continuity],
                ['skills', studio.t.nav.skills],
                ['git', studio.t.nav.git],
              ] as [sec, label]}
                <button
                  onclick={() => (studio.leftSection = sec as typeof studio.leftSection)}
                  class="flex-1 rounded-lg py-1.5 text-xs font-semibold"
                  style="
                    background: {studio.leftSection === sec ? 'var(--accent)' : 'transparent'};
                    color: {studio.leftSection === sec ? 'var(--accent-ink)' : 'var(--muted)'};
                  "
                >
                  {label}
                </button>
              {/each}
            </div>
            <div class="min-h-0 flex-1 overflow-hidden">
              {#if studio.leftSection === 'skills'}
                <SkillsStudio />
              {:else if studio.leftSection === 'git'}
                <GitSyncPanel />
              {:else}
                <ContinuityStudio />
              {/if}
            </div>
          </div>
        {/if}
      </div>

      <!-- Mobile 4-Tab Bottom Navigation with Safe Area -->
      <nav
        class="safe-pb grid shrink-0 grid-cols-4 border-t pt-1.5"
        style="border-color: var(--border); background: var(--surface);"
      >
        {#each [
          ['write', PenLine, studio.t.nav.write],
          ['agent', Sparkles, studio.t.nav.agent],
          ['bible', Users, studio.t.nav.bible],
          ['studio', Menu, studio.t.nav.studio],
        ] as [tabId, Icon, label]}
          {@const Comp = Icon as typeof PenLine}
          <button
            onclick={() => {
              const target = tabId as typeof studio.mobileTab;
              studio.mobileTab = target;
              if (target === 'studio' && (studio.leftSection === 'chapters' || studio.leftSection === 'bible')) {
                studio.leftSection = 'continuity';
              }
            }}
            class="flex flex-col items-center gap-0.5 py-1 text-[11px] font-medium"
            style="color: {studio.mobileTab === tabId ? 'var(--accent)' : 'var(--muted)'};"
          >
            <Comp size={18} />
            <span>{label}</span>
          </button>
        {/each}
      </nav>
    </div>
  {/if}

  <!-- Modals, Drawers & Toast -->
  <ContextInspector />
  <SettingsModal />
  <QuickAgentSheet />
  <NewNovelModal onCreated={() => (showLibrary = false)} />
  <EditNovelModal />
  <SearchModal />

  {#if studio.toastMessage}
    <div
      class="anim-fade fixed bottom-16 left-1/2 z-50 -translate-x-1/2 rounded-full px-4 py-2 text-xs font-medium shadow-lg md:bottom-6"
      style="background: var(--ink); color: var(--bg);"
    >
      {studio.toastMessage}
    </div>
  {/if}
</div>

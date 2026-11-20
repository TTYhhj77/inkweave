<script lang="ts">
  import { onMount } from 'svelte';
  import { Plus, Sparkles, GitBranch, ArrowRight, ArrowLeft, CloudDownload, Trash2, FolderOpen, RefreshCw } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';
  import { api } from '../api/client';
  import type { RemoteNovel } from '../types';

  let { onEnterStudio }: { onEnterStudio: () => void } = $props();

  let newTitle = $state('');
  let newGenre = $state('');
  let newLang = $state('th');
  let newSynopsis = $state('');
  let showCreate = $state(false);
  let creating = $state(false);
  let showClone = $state(false);
  let cloneUrl = $state('');

  let showCloudPull = $state(false);
  let cloudProvider = $state<'gdrive' | 'webdav' | 'folder'>('gdrive');
  let remoteNovels = $state<RemoteNovel[]>([]);
  let cloudLoading = $state(false);

  onMount(() => {
    studio.loadProjects();
  });

  async function createNovel() {
    if (!newTitle.trim() || creating) return;
    creating = true;
    try {
      await studio.createProject(
        newTitle.trim(),
        newGenre.trim() || (newLang === 'th' ? 'นิยายทั่วไป' : 'Fiction'),
        newLang,
        newSynopsis.trim(),
      );
      newTitle = '';
      newGenre = '';
      newSynopsis = '';
      showCreate = false;
      onEnterStudio();
    } catch (e) {
      studio.notify(String(e));
    } finally {
      creating = false;
    }
  }

  async function removeNovel(e: MouseEvent, id: string, title: string) {
    e.stopPropagation();
    const ok = window.confirm(`${studio.t.library.confirmDelete}\n\n"${title}"`);
    if (!ok) return;
    try {
      await studio.deleteProject(id);
    } catch (err) {
      studio.notify(String(err));
    }
  }

  async function cloneNovel() {
    if (!cloneUrl.trim()) return;
    try {
      const proj = await api<{ id: string }>('/api/projects/clone', {
        method: 'POST',
        body: JSON.stringify({ url: cloneUrl.trim() }),
      });
      await studio.loadProjects();
      await studio.openProject(proj.id);
      showClone = false;
      cloneUrl = '';
      onEnterStudio();
    } catch (e) {
      studio.notify(String(e));
    }
  }

  function isCloudReady(kind: 'gdrive' | 'webdav' | 'folder'): boolean {
    const cc = studio.cloudConfig;
    if (!cc) return false;
    if (kind === 'gdrive') return cc.google_drive.connected;
    if (kind === 'webdav') return cc.webdav.enabled && cc.webdav.base_url !== '';
    return cc.local_folder.enabled && cc.local_folder.path !== '';
  }

  function openCloudPull() {
    showCloudPull = !showCloudPull;
    if (!showCloudPull) return;
    if (isCloudReady('gdrive')) cloudProvider = 'gdrive';
    else if (isCloudReady('folder')) cloudProvider = 'folder';
    else if (isCloudReady('webdav')) cloudProvider = 'webdav';
    if (isCloudReady(cloudProvider)) {
      loadCloudNovels();
    } else {
      remoteNovels = [];
    }
  }

  async function loadCloudNovels() {
    cloudLoading = true;
    try {
      remoteNovels = await api<RemoteNovel[]>(`/api/cloud/${cloudProvider}/novels`);
    } catch (e) {
      remoteNovels = [];
      studio.notify(String(e));
    } finally {
      cloudLoading = false;
    }
  }

  async function pullFromCloud(slug: string) {
    try {
      const proj = await api<{ id: string }>(`/api/cloud/${cloudProvider}/pull-novel`, {
        method: 'POST',
        body: JSON.stringify({ slug }),
      });
      await studio.loadProjects();
      await studio.openProject(proj.id);
      showCloudPull = false;
      onEnterStudio();
    } catch (e) {
      studio.notify(String(e));
    }
  }
</script>

<div class="h-full overflow-y-auto px-4 py-8 sm:px-8" style="background: var(--bg);">
  <div class="mx-auto max-w-4xl">
    <div class="mb-6 flex flex-wrap items-end justify-between gap-4">
      <div>
        {#if studio.activeProject}
          <button
            onclick={onEnterStudio}
            class="mb-2 inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs font-medium transition-colors"
            style="border-color: var(--border); background: var(--surface); color: var(--ink);"
          >
            <ArrowLeft size={13} />
            <span>{studio.t.library.backToStudio} ({studio.activeProject.title})</span>
          </button>
        {/if}
        <div class="mb-1 text-xs font-semibold tracking-wider uppercase" style="color: var(--accent);">
          Inkweave Literary Studio • {studio.projects.length} {studio.t.library.novelCount}
        </div>
        <h1 class="text-2xl font-semibold sm:text-3xl">{studio.t.library.title}</h1>
        <p class="mt-1 text-sm" style="color: var(--muted);">{studio.t.library.subtitle}</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button
          onclick={async () => {
            await studio.createStarterProject();
            onEnterStudio();
          }}
          class="flex items-center gap-1.5 rounded-xl border px-3.5 py-2 text-xs font-semibold"
          style="border-color: var(--border); background: var(--surface);"
        >
          <Sparkles size={14} style="color: var(--accent);" />
          <span>{studio.t.library.loadDemo}</span>
        </button>
        <button
          onclick={openCloudPull}
          class="flex items-center gap-1.5 rounded-xl border px-3.5 py-2 text-xs font-semibold"
          style="border-color: var(--border); background: var(--surface);"
        >
          <CloudDownload size={14} />
          <span>{studio.t.git.pullCloud}</span>
        </button>
        <button
          onclick={() => (showClone = !showClone)}
          class="flex items-center gap-1.5 rounded-xl border px-3.5 py-2 text-xs font-semibold"
          style="border-color: var(--border); background: var(--surface);"
        >
          <GitBranch size={14} />
          <span>{studio.t.library.cloneGit}</span>
        </button>
        <button
          onclick={() => (showCreate = !showCreate)}
          class="flex items-center gap-1.5 rounded-xl px-4 py-2 text-xs font-semibold"
          style="background: var(--accent); color: var(--accent-ink);"
        >
          <Plus size={14} />
          <span>{studio.t.library.newNovel}</span>
        </button>
      </div>
    </div>

    {#if studio.androidFolder.available}
      <div
        class="mb-6 flex flex-wrap items-center justify-between gap-3 rounded-2xl border p-4 text-xs"
        style="
          border-color: {studio.androidFolder.granted ? 'var(--success)' : 'var(--accent)'};
          background: var(--surface-elevated);
        "
      >
        <div class="flex min-w-0 flex-1 items-start gap-2.5">
          <FolderOpen size={18} class="mt-0.5 shrink-0" style="color: {studio.androidFolder.granted ? 'var(--success)' : 'var(--accent)'};" />
          <div class="min-w-0">
            <div class="font-semibold" style="color: var(--ink);">
              {studio.lang === 'th' ? 'โฟลเดอร์เก็บนิยายเฉพาะบน Android' : 'Dedicated Android Novel Folder'}
            </div>
            {#if studio.androidFolder.granted}
              <div class="mt-0.5 truncate" style="color: var(--muted);">
                {studio.androidFolder.label || studio.androidFolder.uri}
                {#if studio.androidFolder.lastSync}
                  • {studio.androidFolder.lastSync}
                {/if}
              </div>
            {:else}
              <div class="mt-0.5" style="color: var(--muted);">
                {studio.lang === 'th'
                  ? 'เลือกหรือสร้างโฟลเดอร์เฉพาะบนเครื่อง แอพจะมีสิทธิ์อ่าน/เขียน/แก้ไข/ลบไฟล์นิยายเฉพาะในโฟลเดอร์นั้น'
                  : 'Create or choose a dedicated Android folder. Inkweave gets read/write/edit/delete access solely inside that folder.'}
              </div>
            {/if}
          </div>
        </div>
        <div class="flex shrink-0 flex-wrap gap-1.5">
          {#if studio.androidFolder.granted}
            <button
              onclick={() => studio.syncAndroidNovelFolderNow()}
              class="flex items-center gap-1 rounded-xl border px-3 py-1.5 font-semibold"
              style="border-color: var(--border); background: var(--surface); color: var(--ink);"
            >
              <RefreshCw size={12} />
              <span>{studio.lang === 'th' ? 'ซิงก์โฟลเดอร์' : 'Sync Folder'}</span>
            </button>
          {/if}
          <button
            onclick={() => studio.pickAndroidNovelFolder()}
            class="flex items-center gap-1 rounded-xl px-3.5 py-1.5 font-semibold"
            style="background: var(--accent); color: var(--accent-ink);"
          >
            <FolderOpen size={13} />
            <span>
              {studio.androidFolder.granted
                ? (studio.lang === 'th' ? 'เปลี่ยนโฟลเดอร์' : 'Change Folder')
                : (studio.lang === 'th' ? 'สร้าง / เลือกโฟลเดอร์' : 'Choose / Create Folder')}
            </span>
          </button>
        </div>
      </div>
    {/if}

    {#if showCloudPull}
      <div class="anim-fade mb-6 space-y-3 rounded-2xl border p-4" style="border-color: var(--border); background: var(--surface-elevated);">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-2 text-xs font-semibold">
            <CloudDownload size={15} style="color: var(--accent);" />
            <span>{studio.t.git.pullCloud}</span>
          </div>
          <div class="flex gap-1.5 text-xs">
            <select
              bind:value={cloudProvider}
              onchange={loadCloudNovels}
              class="rounded-lg border px-2.5 py-1 outline-none"
              style="border-color: var(--border); background: var(--bg);"
            >
              <option value="gdrive">Google Drive</option>
              <option value="webdav">WebDAV</option>
              <option value="folder">{studio.t.git.folderTab}</option>
            </select>
            <button onclick={loadCloudNovels} class="rounded-lg border px-2.5 py-1" style="border-color: var(--border);">
              Refresh
            </button>
          </div>
        </div>
        {#if cloudLoading}
          <p class="text-xs" style="color: var(--muted);">Loading remote novels...</p>
        {:else if remoteNovels.length === 0}
          <p class="text-xs" style="color: var(--muted);">ไม่พบนิยายที่ซิงก์ไว้บน {cloudProvider} (กรุณาตั้งค่าและซิงก์ในหน้า ซิงก์ Git & Cloud ก่อน)</p>
        {:else}
          <div class="grid gap-2 sm:grid-cols-2">
            {#each remoteNovels as rn}
              <div class="flex items-center justify-between rounded-xl border p-3 text-xs" style="border-color: var(--border); background: var(--bg);">
                <div>
                  <div class="font-semibold">{rn.title || rn.slug}</div>
                  <div style="color: var(--muted);">{rn.file_count} files</div>
                </div>
                <button
                  onclick={() => pullFromCloud(rn.slug)}
                  class="rounded-lg px-3 py-1.5 font-semibold"
                  style="background: var(--accent); color: var(--accent-ink);"
                >
                  Pull
                </button>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    {#if showCreate}
      <div class="anim-fade mb-6 rounded-2xl border p-5" style="border-color: var(--border); background: var(--surface-elevated);">
        <h3 class="mb-3 text-sm font-semibold">{studio.t.library.newNovel}</h3>
        <div class="grid gap-3 sm:grid-cols-2">
          <input
            type="text"
            bind:value={newTitle}
            onkeydown={(e) => {
              if (e.key === 'Enter') createNovel();
            }}
            placeholder="ชื่อเรื่อง / Novel Title"
            class="rounded-xl border p-2.5 text-sm outline-none"
            style="border-color: var(--border); background: var(--bg); color: var(--ink);"
          />
          <div class="flex gap-2">
            <input
              type="text"
              bind:value={newGenre}
              placeholder="Genre"
              class="flex-1 rounded-xl border p-2.5 text-sm outline-none"
              style="border-color: var(--border); background: var(--bg);"
            />
            <select
              bind:value={newLang}
              class="rounded-xl border px-3 text-sm outline-none"
              style="border-color: var(--border); background: var(--bg);"
            >
              <option value="th">ไทย</option>
              <option value="en">EN</option>
            </select>
          </div>
          <textarea
            bind:value={newSynopsis}
            rows={2}
            placeholder="เรื่องย่อ / Premise..."
            class="rounded-xl border p-2.5 text-sm outline-none sm:col-span-2"
            style="border-color: var(--border); background: var(--bg);"
          ></textarea>
        </div>
        <div class="mt-3 flex justify-end gap-2">
          <button onclick={() => (showCreate = false)} class="rounded-xl border px-3 py-1.5 text-xs" style="border-color: var(--border);">
            {studio.lang === 'th' ? 'ยกเลิก' : 'Cancel'}
          </button>
          <button
            onclick={createNovel}
            disabled={!newTitle.trim() || creating}
            class="rounded-xl px-4 py-1.5 text-xs font-semibold disabled:opacity-40"
            style="background: var(--accent); color: var(--accent-ink);"
          >
            {creating ? (studio.lang === 'th' ? 'กำลังสร้าง...' : 'Creating...') : (studio.lang === 'th' ? 'สร้างเรื่อง' : 'Create')}
          </button>
        </div>
      </div>
    {/if}

    {#if showClone}
      <div class="anim-fade mb-6 flex gap-2 rounded-2xl border p-4" style="border-color: var(--border); background: var(--surface-elevated);">
        <input
          type="text"
          bind:value={cloneUrl}
          onkeydown={(e) => {
            if (e.key === 'Enter') cloneNovel();
          }}
          placeholder="https://github.com/user/my-novel.git"
          class="flex-1 rounded-xl border px-3 py-2 text-sm outline-none"
          style="border-color: var(--border); background: var(--bg); color: var(--ink);"
        />
        <button onclick={cloneNovel} class="rounded-xl px-4 py-2 text-xs font-semibold" style="background: var(--accent); color: var(--accent-ink);">
          Clone
        </button>
      </div>
    {/if}

    <div class="grid gap-4 sm:grid-cols-2">
      {#each studio.projects as proj}
        {@const isActive = studio.activeProject?.id === proj.id}
        <div
          role="button"
          tabindex="0"
          onclick={async () => {
            await studio.openProject(proj.id);
            onEnterStudio();
          }}
          onkeydown={async (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              await studio.openProject(proj.id);
              onEnterStudio();
            }
          }}
          class="group flex cursor-pointer flex-col justify-between rounded-2xl border p-5 text-left transition-all hover:shadow-md"
          style="
            border-color: {isActive ? 'var(--accent)' : 'var(--border)'};
            background: var(--surface-elevated);
          "
        >
          <div>
            <div class="mb-2 flex items-center justify-between gap-2">
              <div class="flex flex-wrap items-center gap-1.5">
                <span class="rounded-md px-2 py-0.5 text-[11px] font-medium" style="background: var(--surface); color: var(--muted);">
                  {proj.genre || proj.language.toUpperCase()}
                </span>
                {#if isActive}
                  <span class="rounded-md px-2 py-0.5 text-[11px] font-semibold" style="background: var(--accent-soft); color: var(--accent);">
                    {studio.t.library.activeBadge}
                  </span>
                {/if}
              </div>
              <div class="flex items-center gap-1.5">
                {#if proj.has_git}
                  <span class="flex items-center gap-1 text-[11px]" style="color: var(--success);">
                    <GitBranch size={12} /> Git
                  </span>
                {/if}
                <button
                  type="button"
                  onclick={(e) => removeNovel(e, proj.id, proj.title)}
                  class="rounded-lg p-1 opacity-60 transition-opacity hover:opacity-100"
                  style="color: var(--danger);"
                  title={studio.t.library.deleteNovel}
                  aria-label={studio.t.library.deleteNovel}
                >
                  <Trash2 size={14} />
                </button>
              </div>
            </div>
            <h2 class="text-lg font-semibold group-hover:underline" style="text-decoration-color: var(--accent);">
              {proj.title}
            </h2>
            <p class="mt-1.5 line-clamp-2 text-xs leading-relaxed" style="color: var(--muted);">
              {proj.synopsis || '-'}
            </p>
          </div>
          <div class="mt-4 flex items-center justify-between border-t pt-3 text-xs tabular-nums" style="border-color: var(--border); color: var(--muted);">
            <span>{proj.chapter_count} {studio.t.library.chapters} • {proj.character_count} {studio.t.library.characters} • {proj.total_words.toLocaleString()} {studio.t.library.words}</span>
            <span class="flex items-center gap-1 font-semibold" style="color: var(--accent);">
              {studio.t.library.openStudio} <ArrowRight size={13} />
            </span>
          </div>
        </div>
      {/each}
    </div>
  </div>
</div>

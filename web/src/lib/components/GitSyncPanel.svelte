<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import {
    GitBranch,
    RefreshCw,
    AlertTriangle,
    Check,
    Cloud,
    HardDrive,
    FolderSync,
    Link2,
    Unlink,
    Copy,
    CheckCircle2,
  } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';
  import { api } from '../api/client';
  import type {
    CloudProviderKind,
    CloudSyncConfig,
    CloudSyncResult,
    CloudSyncStatus,
    ConflictFile,
  } from '../types';

  let providerTab = $state<CloudProviderKind>('github');

  // Git state
  let remoteInput = $state(studio.gitStatus?.status.remote_url || '');
  let commitMsg = $state('');
  let busy = $state(false);
  let logEntries = $state<Array<{ hash: string; short: string; message: string; author: string; when: string }>>([]);

  // Cloud config form state
  let gdriveClientID = $state('');
  let gdriveClientSecret = $state('');
  let gdriveRootFolder = $state('Inkweave');
  let gdriveDirectToken = $state('');
  let webdavURL = $state('');
  let webdavUser = $state('');
  let webdavPass = $state('');
  let webdavRoot = $state('/Inkweave');
  let folderPath = $state('');

  let cloudStatus = $state<CloudSyncStatus | null>(null);
  let testMsg = $state('');

  $effect(() => {
    if (studio.gitStatus?.status.remote_url && !remoteInput) {
      remoteInput = studio.gitStatus.status.remote_url;
    }
  });

  $effect(() => {
    const cc = studio.cloudConfig;
    if (cc) {
      if (!gdriveClientID && cc.google_drive.client_id) gdriveClientID = cc.google_drive.client_id;
      if (cc.google_drive.root_folder) gdriveRootFolder = cc.google_drive.root_folder;
      if (!webdavURL && cc.webdav.base_url) webdavURL = cc.webdav.base_url;
      if (!webdavUser && cc.webdav.username) webdavUser = cc.webdav.username;
      if (cc.webdav.root_path) webdavRoot = cc.webdav.root_path;
      if (!folderPath && cc.local_folder.path) folderPath = cc.local_folder.path;
    }
  });

  function onOAuthMessage(ev: MessageEvent) {
    if (ev.data?.type === 'inkweave-gdrive-connected') {
      studio.loadCloudConfig();
      studio.notify(studio.lang === 'th' ? 'เชื่อมต่อ Google Drive สำเร็จ!' : 'Google Drive connected!');
    }
  }

  onMount(() => {
    window.addEventListener('message', onOAuthMessage);
    if (!studio.cloudConfig) studio.loadCloudConfig();
  });

  onDestroy(() => {
    window.removeEventListener('message', onOAuthMessage);
  });

  async function selectTab(kind: CloudProviderKind) {
    providerTab = kind;
    testMsg = '';
    if (kind !== 'github' && studio.activeProject) {
      try {
        cloudStatus = await api<CloudSyncStatus>(
          `/api/projects/${studio.activeProject.id}/cloud/${kind}/status`,
        );
      } catch {
        cloudStatus = null;
      }
    }
  }

  // --- Git Handlers ---
  async function initGit() {
    if (!studio.activeProject) return;
    try {
      await api(`/api/projects/${studio.activeProject.id}/git/init`, { method: 'POST' });
      await studio.refreshGitStatus();
    } catch (e) {
      studio.notify(String(e));
    }
  }

  async function saveRemote() {
    if (!studio.activeProject) return;
    try {
      await api(`/api/projects/${studio.activeProject.id}/git/remote`, {
        method: 'POST',
        body: JSON.stringify({ url: remoteInput }),
      });
      await studio.refreshGitStatus();
      studio.notify('Remote URL updated');
    } catch (e) {
      studio.notify(String(e));
    }
  }

  async function commitLocal() {
    if (!studio.activeProject) return;
    busy = true;
    try {
      await api(`/api/projects/${studio.activeProject.id}/git/commit`, {
        method: 'POST',
        body: JSON.stringify({ message: commitMsg }),
      });
      commitMsg = '';
      await studio.refreshGitStatus();
      await loadLog();
      studio.notify(studio.lang === 'th' ? 'Commit ลงเครื่องเรียบร้อย' : 'Committed locally');
    } catch (e) {
      studio.notify(String(e));
    } finally {
      busy = false;
    }
  }

  async function syncGit() {
    if (!studio.activeProject) return;
    busy = true;
    try {
      const res = await api<{ status: string; message: string; conflicts?: ConflictFile[] }>(
        `/api/projects/${studio.activeProject.id}/git/sync`,
        { method: 'POST', body: JSON.stringify({ message: commitMsg }) },
      );
      studio.gitConflicts = res.conflicts || [];
      commitMsg = '';
      await studio.refreshGitStatus();
      await loadLog();
      studio.notify(res.message);
    } catch (e) {
      studio.notify(String(e));
    } finally {
      busy = false;
    }
  }

  async function resolveGitConflict(path: string, content: string) {
    if (!studio.activeProject) return;
    try {
      await api(`/api/projects/${studio.activeProject.id}/git/resolve`, {
        method: 'POST',
        body: JSON.stringify({ path, content }),
      });
      studio.gitConflicts = studio.gitConflicts.filter((c) => c.path !== path);
      await studio.openProject(studio.activeProject.id);
      studio.notify(studio.lang === 'th' ? 'แก้ Conflict และ Merge เรียบร้อย' : 'Conflict resolved');
    } catch (e) {
      studio.notify(String(e));
    }
  }

  async function loadLog() {
    if (!studio.activeProject) return;
    try {
      logEntries = await api(`/api/projects/${studio.activeProject.id}/git/log`);
    } catch (e) {
      studio.notify(String(e));
    }
  }

  // --- Cloud Sync Handlers (Google Drive / WebDAV / Folder) ---
  async function saveCloudSettings() {
    const cc: CloudSyncConfig = {
      active_provider: providerTab,
      google_drive: {
        client_id: gdriveClientID.trim(),
        root_folder: gdriveRootFolder.trim() || 'Inkweave',
        account_email: studio.cloudConfig?.google_drive.account_email || '',
        connected: studio.cloudConfig?.google_drive.connected || false,
      },
      webdav: {
        base_url: webdavURL.trim(),
        username: webdavUser.trim(),
        root_path: webdavRoot.trim() || '/Inkweave',
        enabled: webdavURL.trim() !== '',
        has_password: studio.cloudConfig?.webdav.has_password || false,
      },
      local_folder: {
        path: folderPath.trim(),
        enabled: folderPath.trim() !== '',
      },
    };
    const res = await api<{
      cloud_sync: CloudSyncConfig;
      gdrive_redirect_uri: string;
      has_gdrive_secret: boolean;
    }>('/api/cloud/config', {
      method: 'PUT',
      body: JSON.stringify({
        cloud_sync: cc,
        ...(gdriveClientSecret ? { gdrive_client_secret: gdriveClientSecret } : {}),
        ...(gdriveDirectToken ? { gdrive_access_token: gdriveDirectToken } : {}),
        ...(webdavPass ? { webdav_password: webdavPass } : {}),
      }),
    });
    studio.cloudConfig = res.cloud_sync;
    studio.gdriveRedirectURI = res.gdrive_redirect_uri;
    studio.hasGDriveSecret = res.has_gdrive_secret;
    gdriveClientSecret = '';
    gdriveDirectToken = '';
    webdavPass = '';
    studio.notify(studio.lang === 'th' ? 'บันทึกการตั้งค่า Cloud Sync แล้ว' : 'Cloud settings saved');
  }

  async function testCloud(kind: 'gdrive' | 'webdav' | 'folder') {
    await saveCloudSettings();
    testMsg = 'Testing...';
    try {
      const res = await api<{ ok: boolean; detail: string }>(`/api/cloud/${kind}/test`, { method: 'POST' });
      testMsg = `Connected: ${res.detail}`;
      await studio.loadCloudConfig();
    } catch (e) {
      testMsg = String(e);
    }
  }

  async function connectGDriveOAuth() {
    try {
      await saveCloudSettings();
      const res = await api<{ auth_url: string }>('/api/oauth/gdrive/start', { method: 'POST' });
      window.open(res.auth_url, 'inkweave_gdrive_oauth', 'width=540,height=680');
    } catch (e) {
      studio.notify(String(e));
    }
  }

  async function disconnectGDrive() {
    await api('/api/oauth/gdrive/disconnect', { method: 'POST' });
    await studio.loadCloudConfig();
    studio.notify('Disconnected Google Drive');
  }

  async function syncCloud(kind: 'gdrive' | 'webdav' | 'folder') {
    if (!studio.activeProject) return;
    busy = true;
    try {
      await saveCloudSettings();
      const res = await api<CloudSyncResult>(
        `/api/projects/${studio.activeProject.id}/cloud/${kind}/sync`,
        { method: 'POST' },
      );
      if (res.conflicts && res.conflicts.length > 0) {
        studio.cloudConflicts = { provider: kind, items: res.conflicts };
      } else {
        studio.cloudConflicts = { provider: '', items: [] };
      }
      if (res.downloaded > 0) {
        await studio.openProject(studio.activeProject.id);
      }
      cloudStatus = await api<CloudSyncStatus>(
        `/api/projects/${studio.activeProject.id}/cloud/${kind}/status`,
      );
      studio.notify(res.message);
    } catch (e) {
      studio.notify(String(e));
    } finally {
      busy = false;
    }
  }

  async function resolveCloudConflict(path: string, content: string) {
    if (!studio.activeProject || !studio.cloudConflicts.provider) return;
    const prov = studio.cloudConflicts.provider;
    await api(`/api/projects/${studio.activeProject.id}/cloud/${prov}/resolve`, {
      method: 'POST',
      body: JSON.stringify({ path, content }),
    });
    const remaining = studio.cloudConflicts.items.filter((c) => c.path !== path);
    studio.cloudConflicts = { provider: remaining.length ? prov : '', items: remaining };
    await studio.openProject(studio.activeProject.id);
    studio.notify(studio.lang === 'th' ? 'แก้ Conflict และอัปโหลดเรียบร้อย' : 'Conflict resolved');
  }

  function copyRedirectURI() {
    if (studio.gdriveRedirectURI) {
      navigator.clipboard?.writeText(studio.gdriveRedirectURI);
      studio.notify('Copied Redirect URI');
    }
  }
</script>

<div class="h-full space-y-4 overflow-y-auto p-4 text-xs" style="background: var(--bg);">
  <div class="flex flex-wrap items-center justify-between gap-2">
    <div class="flex items-center gap-2">
      <Cloud size={16} style="color: var(--accent);" />
      <h3 class="text-sm font-semibold">{studio.t.git.syncHubTitle}</h3>
    </div>
    {#if providerTab === 'github'}
      <button onclick={loadLog} class="rounded-lg border px-2.5 py-1" style="border-color: var(--border);">
        History
      </button>
    {/if}
  </div>

  <!-- 4 Sync Provider Selector Tabs -->
  <div class="flex flex-wrap gap-1.5 rounded-xl border p-1.5" style="border-color: var(--border); background: var(--surface);">
    {#each [
      ['github', 'GitHub / Git', GitBranch],
      ['gdrive', studio.t.git.gdriveTab, Cloud],
      ['webdav', studio.t.git.webdavTab, HardDrive],
      ['folder', studio.t.git.folderTab, FolderSync],
    ] as [id, label, Icon]}
      {@const Comp = Icon as typeof GitBranch}
      <button
        onclick={() => selectTab(id as CloudProviderKind)}
        class="flex flex-1 items-center justify-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-semibold transition-colors"
        style="
          background: {providerTab === id ? 'var(--accent)' : 'transparent'};
          color: {providerTab === id ? 'var(--accent-ink)' : 'var(--muted)'};
        "
      >
        <Comp size={13} />
        <span>{label}</span>
      </button>
    {/each}
  </div>

  <!-- TAB 1: GitHub / Git -->
  {#if providerTab === 'github'}
    {#if !studio.gitStatus?.status.initialized}
      <div class="rounded-xl border p-4" style="border-color: var(--border); background: var(--surface-elevated);">
        <p class="mb-3" style="color: var(--muted);">
          โปรเจกต์นิยายนี้ยังไม่ได้เปิดใช้ Git (ระบบจะข้ามไฟล์แคช `.inkweave/` ให้อัตโนมัติ)
        </p>
        <button
          onclick={initGit}
          class="rounded-xl px-4 py-2 font-semibold"
          style="background: var(--accent); color: var(--accent-ink);"
        >
          {studio.t.git.initRepo}
        </button>
      </div>
    {:else}
      <div class="space-y-3 rounded-xl border p-4" style="border-color: var(--border); background: var(--surface-elevated);">
        <div class="flex items-center justify-between">
          <span>Branch: <strong>{studio.gitStatus.status.branch}</strong></span>
          <span style="color: var(--muted);">
            {studio.gitStatus.status.dirty_files?.length || 0} modified file(s)
          </span>
        </div>

        <div class="flex gap-2">
          <input
            type="text"
            bind:value={remoteInput}
            placeholder="https://github.com/username/my-novel.git"
            class="flex-1 rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg);"
          />
          <button onclick={saveRemote} class="rounded-lg border px-3 py-1.5 font-medium" style="border-color: var(--border);">
            Save Remote
          </button>
        </div>

        <input
          type="text"
          bind:value={commitMsg}
          placeholder={studio.gitStatus.smart_message || studio.t.git.commitMsg}
          class="w-full rounded-lg border px-2.5 py-1.5 outline-none"
          style="border-color: var(--border); background: var(--bg);"
        />

        <div class="flex flex-wrap gap-2">
          <button
            onclick={commitLocal}
            disabled={busy}
            class="flex items-center gap-1.5 rounded-xl border px-3 py-2 font-semibold"
            style="border-color: var(--border);"
          >
            <Check size={14} /> {studio.t.git.commitOnly}
          </button>
          <button
            onclick={syncGit}
            disabled={busy}
            class="flex items-center gap-1.5 rounded-xl px-4 py-2 font-semibold"
            style="background: var(--accent); color: var(--accent-ink);"
          >
            <RefreshCw size={14} class={busy ? 'animate-spin' : ''} />
            {studio.t.git.syncNow}
          </button>
        </div>
      </div>
    {/if}

    {#if logEntries.length > 0}
      <div class="space-y-1.5">
        <h4 class="font-semibold" style="color: var(--muted);">Recent Commits</h4>
        {#each logEntries as c}
          <div class="flex items-center justify-between rounded-lg border px-3 py-2" style="border-color: var(--border); background: var(--surface-elevated);">
            <div class="min-w-0">
              <div class="truncate font-medium">{c.message}</div>
              <div class="text-[10px]" style="color: var(--muted);">{c.short} • {c.author}</div>
            </div>
          </div>
        {/each}
      </div>
    {/if}

  <!-- TAB 2: Google Drive -->
  {:else if providerTab === 'gdrive'}
    <div class="space-y-3 rounded-xl border p-4" style="border-color: var(--border); background: var(--surface-elevated);">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-2 font-semibold">
          <span>Google Drive API v3 (OAuth 2.0 PKCE)</span>
        </div>
        {#if studio.cloudConfig?.google_drive.connected}
          <span class="flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px]" style="background: var(--success-soft); color: var(--success);">
            <CheckCircle2 size={12} />
            {studio.cloudConfig.google_drive.account_email || 'Connected'}
          </span>
        {/if}
      </div>

      <p style="color: var(--muted);">
        ซิงก์ไฟล์นิยาย (Markdown + YAML) เข้าโฟลเดอร์ส่วนตัวใน Google Drive โดยตรงด้วยสิทธิ์ <code>drive.file</code> (เข้าถึงเฉพาะไฟล์ที่ Inkweave สร้าง)
      </p>

      {#if studio.gdriveRedirectURI}
        <div class="flex items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5 text-[11px]" style="border-color: var(--border); background: var(--bg);">
          <span class="truncate">Redirect URI: <code>{studio.gdriveRedirectURI}</code></span>
          <button onclick={copyRedirectURI} class="flex shrink-0 items-center gap-1 rounded px-2 py-0.5 font-medium" style="background: var(--surface);">
            <Copy size={11} /> Copy
          </button>
        </div>
      {/if}

      <div class="grid gap-2 sm:grid-cols-2">
        <label class="block">
          <span class="mb-1 block text-[11px]" style="color: var(--muted);">Google OAuth Client ID</span>
          <input
            type="text"
            bind:value={gdriveClientID}
            placeholder="xxxx.apps.googleusercontent.com"
            class="w-full rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg);"
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-[11px]" style="color: var(--muted);">Client Secret (ถ้ามี)</span>
          <input
            type="password"
            bind:value={gdriveClientSecret}
            placeholder={studio.hasGDriveSecret ? '•••••••• (saved)' : 'Optional for PKCE Desktop'}
            class="w-full rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg);"
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-[11px]" style="color: var(--muted);">โฟลเดอร์หลักบน Drive</span>
          <input
            type="text"
            bind:value={gdriveRootFolder}
            placeholder="Inkweave"
            class="w-full rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg);"
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-[11px]" style="color: var(--muted);">Access Token (ทางลัดสำหรับทดสอบ)</span>
          <input
            type="password"
            bind:value={gdriveDirectToken}
            placeholder="ya29.... (ไม่บังคับ)"
            class="w-full rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg);"
          />
        </label>
      </div>

      <div class="flex flex-wrap gap-2 pt-1">
        <button onclick={saveCloudSettings} class="rounded-xl border px-3 py-2 font-medium" style="border-color: var(--border);">
          Save
        </button>
        <button
          onclick={connectGDriveOAuth}
          class="flex items-center gap-1.5 rounded-xl border px-3 py-2 font-semibold"
          style="border-color: var(--accent); color: var(--accent);"
        >
          <Link2 size={14} /> {studio.lang === 'th' ? 'เชื่อมต่อ Google Drive' : 'Connect Google Drive'}
        </button>
        {#if studio.cloudConfig?.google_drive.connected}
          <button onclick={() => testCloud('gdrive')} class="rounded-xl border px-3 py-2 font-medium" style="border-color: var(--border);">
            Test
          </button>
          <button onclick={disconnectGDrive} class="flex items-center gap-1 rounded-xl border px-3 py-2" style="border-color: var(--border); color: var(--danger);">
            <Unlink size={13} /> Disconnect
          </button>
        {/if}
        <button
          onclick={() => syncCloud('gdrive')}
          disabled={busy || !studio.cloudConfig?.google_drive.connected}
          class="flex items-center gap-1.5 rounded-xl px-4 py-2 font-semibold disabled:opacity-45"
          style="background: var(--accent); color: var(--accent-ink);"
        >
          <RefreshCw size={14} class={busy ? 'animate-spin' : ''} />
          {studio.lang === 'th' ? 'ซิงก์กับ Google Drive ทันที' : 'Sync Google Drive Now'}
        </button>
      </div>
      {#if testMsg}
        <div class="text-[11px]" style="color: var(--muted);">{testMsg}</div>
      {/if}
    </div>

  <!-- TAB 3: WebDAV -->
  {:else if providerTab === 'webdav'}
    <div class="space-y-3 rounded-xl border p-4" style="border-color: var(--border); background: var(--surface-elevated);">
      <div class="font-semibold">WebDAV (Nextcloud / ownCloud / Synology NAS / Koofr)</div>
      <div class="grid gap-2 sm:grid-cols-2">
        <label class="block sm:col-span-2">
          <span class="mb-1 block text-[11px]" style="color: var(--muted);">WebDAV Base URL</span>
          <input
            type="text"
            bind:value={webdavURL}
            placeholder="https://cloud.example.com/remote.php/dav/files/username"
            class="w-full rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg);"
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-[11px]" style="color: var(--muted);">Username</span>
          <input
            type="text"
            bind:value={webdavUser}
            class="w-full rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg);"
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-[11px]" style="color: var(--muted);">App Password</span>
          <input
            type="password"
            bind:value={webdavPass}
            placeholder={studio.cloudConfig?.webdav.has_password ? '•••••••• (saved)' : 'Password / App Token'}
            class="w-full rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg);"
          />
        </label>
        <label class="block sm:col-span-2">
          <span class="mb-1 block text-[11px]" style="color: var(--muted);">Remote Folder Path</span>
          <input
            type="text"
            bind:value={webdavRoot}
            placeholder="/Inkweave"
            class="w-full rounded-lg border px-2.5 py-1.5 outline-none"
            style="border-color: var(--border); background: var(--bg);"
          />
        </label>
      </div>
      <div class="flex flex-wrap gap-2">
        <button onclick={() => testCloud('webdav')} class="rounded-xl border px-3 py-2 font-medium" style="border-color: var(--border);">
          Save & Test
        </button>
        <button
          onclick={() => syncCloud('webdav')}
          disabled={busy || !webdavURL.trim()}
          class="flex items-center gap-1.5 rounded-xl px-4 py-2 font-semibold disabled:opacity-45"
          style="background: var(--accent); color: var(--accent-ink);"
        >
          <RefreshCw size={14} class={busy ? 'animate-spin' : ''} />
          {studio.lang === 'th' ? 'ซิงก์ WebDAV ทันที' : 'Sync WebDAV Now'}
        </button>
      </div>
      {#if testMsg}
        <div class="text-[11px]" style="color: var(--muted);">{testMsg}</div>
      {/if}
    </div>

  <!-- TAB 4: Synced Folder (OneDrive / Dropbox / Drive Desktop / Syncthing) -->
  {:else if providerTab === 'folder'}
    <div class="space-y-3 rounded-xl border p-4" style="border-color: var(--border); background: var(--surface-elevated);">
      <div class="font-semibold">Synced Folder (OneDrive / Dropbox / Google Drive Desktop / Syncthing)</div>
      <p style="color: var(--muted);">
        เลือกโฟลเดอร์บนเครื่องที่มีโปรแกรมคลาวด์หรือ Syncthing ซิงก์อยู่แล้ว ระบบจะทำ 3-Way Sync และตรวจจับการแก้ไขชนกันให้โดยอัตโนมัติ
      </p>
      <label class="block">
        <span class="mb-1 block text-[11px]" style="color: var(--muted);">Target Sync Folder Path</span>
        <input
          type="text"
          bind:value={folderPath}
          placeholder="C:\Users\Admin\OneDrive\Inkweave"
          class="w-full rounded-lg border px-2.5 py-1.5 outline-none"
          style="border-color: var(--border); background: var(--bg);"
        />
      </label>
      <div class="flex flex-wrap gap-2">
        <button onclick={() => testCloud('folder')} class="rounded-xl border px-3 py-2 font-medium" style="border-color: var(--border);">
          Save & Test Folder
        </button>
        <button
          onclick={() => syncCloud('folder')}
          disabled={busy || !folderPath.trim()}
          class="flex items-center gap-1.5 rounded-xl px-4 py-2 font-semibold disabled:opacity-45"
          style="background: var(--accent); color: var(--accent-ink);"
        >
          <RefreshCw size={14} class={busy ? 'animate-spin' : ''} />
          {studio.lang === 'th' ? 'ซิงก์กับโฟลเดอร์ทันที' : 'Sync Folder Now'}
        </button>
      </div>
      {#if testMsg}
        <div class="text-[11px]" style="color: var(--muted);">{testMsg}</div>
      {/if}
    </div>
  {/if}

  {#if providerTab !== 'github' && cloudStatus}
    <div class="rounded-xl border px-3.5 py-2.5 text-[11px]" style="border-color: var(--border); background: var(--surface);">
      Tracked novel files: <strong>{cloudStatus.tracked_count}</strong> • Unsynced changes: <strong>{cloudStatus.modified_since_sync?.length || 0}</strong>
    </div>
  {/if}

  <!-- Shared Conflict Resolver (Git OR Cloud) -->
  {#if (providerTab === 'github' && studio.gitConflicts.length > 0) || (providerTab !== 'github' && studio.cloudConflicts.items.length > 0)}
    {@const activeConflicts = providerTab === 'github' ? studio.gitConflicts : studio.cloudConflicts.items}
    <div class="space-y-3 rounded-xl border p-4" style="border-color: var(--danger); background: var(--danger-soft);">
      <div class="flex items-center gap-2 font-semibold" style="color: var(--danger);">
        <AlertTriangle size={16} />
        <span>{studio.t.git.conflicts}</span>
      </div>
      {#each activeConflicts as cf}
        <div class="rounded-xl border p-3" style="border-color: var(--border); background: var(--bg);">
          <div class="mb-2 font-semibold">{cf.path}</div>
          <div class="grid gap-2 md:grid-cols-2">
            <div>
              <div class="mb-1 flex justify-between font-medium">
                <span>Local (This Device)</span>
                <button
                  onclick={() => (providerTab === 'github' ? resolveGitConflict(cf.path, cf.local) : resolveCloudConflict(cf.path, cf.local))}
                  class="rounded px-2 py-0.5"
                  style="background: var(--accent); color: var(--accent-ink);"
                >
                  {studio.t.git.useLocal}
                </button>
              </div>
              <pre class="max-h-36 overflow-auto rounded border p-2 text-[11px]" style="border-color: var(--border);">{cf.local}</pre>
            </div>
            <div>
              <div class="mb-1 flex justify-between font-medium">
                <span>Remote (Cloud / Repo)</span>
                <button
                  onclick={() => (providerTab === 'github' ? resolveGitConflict(cf.path, cf.remote) : resolveCloudConflict(cf.path, cf.remote))}
                  class="rounded px-2 py-0.5"
                  style="background: var(--success); color: #fff;"
                >
                  {studio.t.git.useRemote}
                </button>
              </div>
              <pre class="max-h-36 overflow-auto rounded border p-2 text-[11px]" style="border-color: var(--border);">{cf.remote}</pre>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

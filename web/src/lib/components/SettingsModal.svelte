<script lang="ts">
  import { untrack } from 'svelte';
  import { X, Download, CheckCircle2, KeyRound, ChevronDown, ChevronRight, Plus, Trash2 } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';
  import { api, exportUrl } from '../api/client';
  import type { AppConfig, ProviderConfig, ThemeName } from '../types';

  let draft = $state<AppConfig | null>(null);
  let githubPat = $state('');
  let keyInputs = $state<Record<string, string>>({});
  let testStatus = $state<Record<string, string>>({});
  let expandedProviders = $state<Record<string, boolean>>({});
  let activeTab = $state<'appearance' | 'providers' | 'routing' | 'export'>('appearance');
  let saving = $state(false);
  let wasOpen = false;

  $effect(() => {
    const open = studio.settingsOpen;
    const cfg = studio.config;
    if (open && !wasOpen && cfg) {
      untrack(() => {
        const cloned = structuredClone($state.snapshot(cfg));
        const initialExpanded: Record<string, boolean> = {};
        for (const p of cloned.providers) {
          if (p.enabled && p.kind !== 'mock') {
            initialExpanded[p.id] = true;
          }
        }
        draft = cloned;
        expandedProviders = initialExpanded;
      });
    }
    wasOpen = open;
  });

  function previewAppearance() {
    if (!draft) return;
    studio.applyThemeVars(
      draft.ui.theme,
      draft.ui.font_family,
      draft.ui.font_size,
      draft.ui.line_height,
      draft.ui.reduced_motion,
    );
  }

  function closeModal() {
    if (studio.config) {
      const ui = studio.config.ui;
      studio.applyThemeVars(ui.theme, ui.font_family, ui.font_size, ui.line_height, ui.reduced_motion);
    }
    studio.settingsOpen = false;
  }

  async function saveAll() {
    if (!draft || saving) return;
    saving = true;
    try {
      await studio.persistSettings(draft, githubPat ? githubPat : undefined);
      githubPat = '';
      studio.settingsOpen = false;
      studio.notify(studio.lang === 'th' ? 'บันทึกการตั้งค่าแล้ว' : 'Settings saved');
    } catch (e) {
      studio.notify(String(e));
    } finally {
      saving = false;
    }
  }

  async function saveProviderKey(p: ProviderConfig) {
    const k = (keyInputs[p.id] || '').trim();
    if (k) {
      p.enabled = true;
    }
    try {
      const updated = await api<AppConfig>('/api/providers', {
        method: 'POST',
        body: JSON.stringify({ provider: p, api_key: k }),
      });
      if (draft) draft.providers = updated.providers;
      studio.config = updated;
      keyInputs[p.id] = '';
      testStatus[p.id] = studio.lang === 'th' ? 'บันทึกแล้ว' : 'Saved';
      studio.notify(`Updated ${p.name}`);
    } catch (e) {
      testStatus[p.id] = String(e);
      studio.notify(String(e));
    }
  }

  async function deleteProvider(id: string) {
    try {
      const updated = await api<AppConfig>(`/api/providers/${id}`, { method: 'DELETE' });
      if (draft) draft.providers = updated.providers;
      studio.config = updated;
      studio.notify(studio.lang === 'th' ? 'ลบ Provider แล้ว' : 'Provider removed');
    } catch (e) {
      studio.notify(String(e));
    }
  }

  function addCustomProvider() {
    if (!draft) return;
    const id = `custom-${Date.now().toString(36)}`;
    const next: ProviderConfig = {
      id,
      name: 'Custom OpenAI-Compatible',
      kind: 'openai-compat',
      base_url: 'https://api.openai.com/v1',
      default_model: 'gpt-4o-mini',
      enabled: true,
      has_api_key: false,
    };
    draft.providers = [...draft.providers, next];
    expandedProviders = { ...expandedProviders, [id]: true };
  }

  async function testProvider(p: ProviderConfig) {
    testStatus[p.id] = 'Testing...';
    try {
      await saveProviderKey(p);
      await api(`/api/providers/${p.id}/test`, {
        method: 'POST',
        body: JSON.stringify({ model: p.default_model }),
      });
      testStatus[p.id] = 'OK';
    } catch (e) {
      testStatus[p.id] = String(e);
    }
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape' && studio.settingsOpen) closeModal();
  }}
/>

{#if studio.settingsOpen && draft}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-2 sm:p-5"
    onclick={(e) => {
      if (e.target === e.currentTarget) closeModal();
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape') closeModal();
    }}
    tabindex="-1"
    role="dialog"
    aria-modal="true"
  >
    <div
      class="anim-sheet flex max-h-[90dvh] w-full max-w-3xl flex-col overflow-hidden rounded-2xl border shadow-2xl"
      style="background: var(--bg); border-color: var(--border); color: var(--ink);"
    >
      <!-- Pinned Header -->
      <div class="flex shrink-0 items-center justify-between border-b px-4 py-3 sm:px-5 sm:py-3.5" style="border-color: var(--border);">
        <h2 class="text-base font-semibold">{studio.t.settings.title}</h2>
        <button onclick={closeModal} class="rounded-lg p-1.5 hover:opacity-75" aria-label="Close">
          <X size={18} />
        </button>
      </div>

      <!-- Pinned Wrapping Tab Bar (never crushed by tall content) -->
      <div class="flex shrink-0 flex-wrap gap-1 border-b px-4 pt-1.5 sm:px-5 sm:pt-2" style="border-color: var(--border); background: var(--surface);">
        {#each [
          ['appearance', studio.t.settings.appearance],
          ['providers', studio.t.settings.providers],
          ['routing', studio.t.settings.routing],
          ['export', studio.t.settings.export],
        ] as [id, label]}
          <button
            onclick={() => (activeTab = id as typeof activeTab)}
            class="shrink-0 border-b-2 px-2.5 py-1.5 text-[11px] font-semibold transition-colors sm:px-3 sm:py-2 sm:text-xs"
            style="border-color: {activeTab === id ? 'var(--accent)' : 'transparent'}; color: {activeTab === id ? 'var(--accent)' : 'var(--muted)'};"
          >
            {label}
          </button>
        {/each}
      </div>

      <!-- Strictly Bounded Scroll Area (min-h-0 guarantees Header/Tabs/Footer stay intact) -->
      <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4 text-sm sm:p-5">
        {#if activeTab === 'appearance'}
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <span class="mb-2 block text-xs font-semibold" style="color: var(--muted);">Theme</span>
              <div class="grid grid-cols-3 gap-2">
                {#each [
                  ['paper', 'Paper', '#FAF8F4'],
                  ['sepia', 'Sepia', '#F3ECD8'],
                  ['warm-dark', 'Warm Dark', '#171614'],
                ] as [id, label, swatch]}
                  <button
                    onclick={() => {
                      if (draft) {
                        draft.ui.theme = id as ThemeName;
                        previewAppearance();
                      }
                    }}
                    class="flex flex-col items-center gap-1.5 rounded-xl border p-2.5 text-xs font-medium"
                    style="border-color: {draft.ui.theme === id ? 'var(--accent)' : 'var(--border)'};"
                  >
                    <span class="h-5 w-8 rounded border" style="background: {swatch}; border-color: var(--border);"></span>
                    <span>{label}</span>
                  </button>
                {/each}
              </div>
            </div>

            <div>
              <span class="mb-2 block text-xs font-semibold" style="color: var(--muted);">Language / ภาษา</span>
              <select
                bind:value={draft.ui.language}
                class="w-full rounded-xl border p-2.5 text-sm outline-none"
                style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
              >
                <option value="th">ภาษาไทย (Thai)</option>
                <option value="en">English</option>
              </select>
            </div>

            <div>
              <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">Prose Font</span>
              <select
                bind:value={draft.ui.font_family}
                onchange={previewAppearance}
                class="w-full rounded-xl border p-2.5 text-sm outline-none"
                style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
              >
                <option value="sarabun">Sarabun (สบายตา ภาษาไทย)</option>
                <option value="ibm-plex">IBM Plex Sans Thai</option>
                <option value="noto-serif">Noto Serif Thai (วรรณกรรม)</option>
                <option value="literata">Literata (Book Serif)</option>
              </select>
            </div>

            <div>
              <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">Font Size ({draft.ui.font_size}px) & Line Height ({draft.ui.line_height})</span>
              <div class="flex gap-2">
                <input
                  type="range"
                  min="15"
                  max="24"
                  bind:value={draft.ui.font_size}
                  oninput={previewAppearance}
                  class="w-1/2 accent-[var(--accent)]"
                />
                <input
                  type="range"
                  min="1.6"
                  max="2.2"
                  step="0.05"
                  bind:value={draft.ui.line_height}
                  oninput={previewAppearance}
                  class="w-1/2 accent-[var(--accent)]"
                />
              </div>
            </div>

            <div class="sm:col-span-2">
              <span class="mb-1 block text-xs font-semibold" style="color: var(--muted);">{studio.t.settings.approvalMode}</span>
              <select
                bind:value={draft.ui.approval_mode}
                class="w-full rounded-xl border p-2.5 text-sm outline-none"
                style="border-color: var(--border); background: var(--surface-elevated); color: var(--ink);"
              >
                <option value="review">{studio.t.settings.reviewDiff}</option>
                <option value="auto">{studio.t.settings.autoApply}</option>
              </select>
            </div>

            <label class="flex items-center gap-2 text-xs font-medium sm:col-span-2">
              <input type="checkbox" bind:checked={draft.ui.reduced_motion} onchange={previewAppearance} />
              <span>{studio.t.settings.reducedMotion}</span>
            </label>

            {#if studio.androidFolder.available}
              <div
                class="space-y-2 rounded-xl border p-3.5 text-xs sm:col-span-2"
                style="border-color: {studio.androidFolder.granted ? 'var(--success)' : 'var(--accent)'}; background: var(--surface-elevated);"
              >
                <div class="font-semibold" style="color: var(--ink);">
                  {studio.lang === 'th' ? 'โฟลเดอร์เก็บนิยายบน Android (Scoped Storage)' : 'Android Dedicated Novel Folder (SAF)'}
                </div>
                <p style="color: var(--muted);">
                  {#if studio.androidFolder.granted}
                    {studio.lang === 'th' ? 'โฟลเดอร์ปัจจุบัน:' : 'Active folder:'} <strong>{studio.androidFolder.label || studio.androidFolder.uri}</strong>
                    {#if studio.androidFolder.lastSync}
                      ({studio.androidFolder.lastSync})
                    {/if}
                  {:else}
                    {studio.lang === 'th'
                      ? 'ยังไม่ได้เลือกโฟลเดอร์บนเครื่อง — กดเพื่อสร้างหรือเลือกโฟลเดอร์เฉพาะที่แอพจะมีสิทธิ์อ่าน/เขียน/แก้ไข/ลบไฟล์นิยายได้'
                      : 'No folder selected yet — choose or create a dedicated folder with scoped read/write/edit/delete access.'}
                  {/if}
                </p>
                <div class="flex flex-wrap gap-2">
                  <button
                    onclick={() => studio.pickAndroidNovelFolder()}
                    class="rounded-lg px-3 py-1.5 font-semibold"
                    style="background: var(--accent); color: var(--accent-ink);"
                  >
                    {studio.androidFolder.granted
                      ? (studio.lang === 'th' ? 'เปลี่ยนโฟลเดอร์' : 'Change Folder')
                      : (studio.lang === 'th' ? 'สร้าง / เลือกโฟลเดอร์เก็บนิยาย' : 'Create / Choose Folder')}
                  </button>
                  {#if studio.androidFolder.granted}
                    <button
                      onclick={() => studio.syncAndroidNovelFolderNow()}
                      class="rounded-lg border px-3 py-1.5 font-semibold"
                      style="border-color: var(--border); background: var(--surface); color: var(--ink);"
                    >
                      {studio.lang === 'th' ? 'ซิงก์โฟลเดอร์ตอนนี้' : 'Sync Folder Now'}
                    </button>
                  {/if}
                </div>
              </div>
            {/if}
          </div>
        {:else if activeTab === 'providers'}
          <div class="rounded-xl border p-3.5" style="border-color: var(--border); background: var(--surface-elevated);">
            <div class="mb-2 flex flex-wrap items-center gap-2 text-xs font-semibold">
              <KeyRound size={14} style="color: var(--accent);" />
              <span>GitHub Personal Access Token (PAT) for Git Sync</span>
              {#if studio.hasGitHubToken}
                <span class="rounded px-1.5 py-0.5 text-[10px]" style="background: var(--success-soft); color: var(--success);">Saved locally</span>
              {/if}
            </div>
            <input
              type="password"
              bind:value={githubPat}
              placeholder="ghp_... (เก็บเฉพาะในเครื่อง ไม่เข้า Git Repo)"
              class="w-full rounded-lg border px-2.5 py-1.5 text-xs outline-none"
              style="border-color: var(--border); background: var(--bg); color: var(--ink);"
            />
          </div>

          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="text-xs font-semibold" style="color: var(--muted);">
              {studio.lang === 'th' ? 'AI Providers (คลิกเพื่อกรอก API Key)' : 'AI Providers (click any row to configure)'}
            </span>
            <button
              onclick={addCustomProvider}
              class="flex shrink-0 items-center gap-1 whitespace-nowrap rounded-lg border px-2.5 py-1 text-xs font-semibold"
              style="border-color: var(--accent); color: var(--accent);"
            >
              <Plus size={13} />
              <span>{studio.lang === 'th' ? 'เพิ่ม Custom API' : 'Add Custom API'}</span>
            </button>
          </div>

          <div class="space-y-2.5">
            {#each draft.providers as p}
              {@const isExpanded = !!expandedProviders[p.id]}
              <div class="rounded-xl border p-3 text-xs" style="border-color: var(--border); background: var(--surface-elevated);">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <div class="flex items-center gap-2">
                    <input type="checkbox" bind:checked={p.enabled} aria-label="Enable {p.name}" />
                    <button
                      onclick={() => (expandedProviders[p.id] = !isExpanded)}
                      class="flex items-center gap-1.5 font-semibold text-left"
                    >
                      {#if p.kind !== 'mock'}
                        {#if isExpanded}
                          <ChevronDown size={14} style="color: var(--muted);" />
                        {:else}
                          <ChevronRight size={14} style="color: var(--muted);" />
                        {/if}
                      {/if}
                      <span>{p.name}</span>
                      <span class="rounded px-1.5 py-0.5 text-[10px]" style="background: var(--surface); color: var(--muted);">{p.kind}</span>
                    </button>
                  </div>

                  <div class="flex items-center gap-2">
                    {#if p.has_api_key}
                      <span class="flex items-center gap-1 text-[11px]" style="color: var(--success);">
                        <CheckCircle2 size={12} /> Key Ready
                      </span>
                    {/if}
                    {#if p.kind !== 'mock'}
                      <button
                        onclick={() => (expandedProviders[p.id] = !isExpanded)}
                        class="rounded-md border px-2 py-0.5 text-[11px]"
                        style="border-color: var(--border); color: var(--muted);"
                      >
                        {isExpanded ? (studio.lang === 'th' ? 'ซ่อน' : 'Hide') : (studio.lang === 'th' ? 'ตั้งค่า' : 'Configure')}
                      </button>
                    {/if}
                    {#if p.id.startsWith('custom-')}
                      <button
                        onclick={() => deleteProvider(p.id)}
                        class="rounded p-1 opacity-70 hover:opacity-100"
                        style="color: var(--danger);"
                        title="Remove"
                      >
                        <Trash2 size={13} />
                      </button>
                    {/if}
                  </div>
                </div>

                {#if p.kind !== 'mock' && isExpanded}
                  <div class="mt-3 grid gap-2 border-t pt-3 sm:grid-cols-2" style="border-color: var(--border);">
                    {#if p.id.startsWith('custom-')}
                      <input
                        type="text"
                        bind:value={p.name}
                        placeholder="Provider Name"
                        class="rounded-lg border px-2 py-1.5 outline-none sm:col-span-2"
                        style="border-color: var(--border); background: var(--bg); color: var(--ink);"
                      />
                    {/if}
                    <input
                      type="text"
                      bind:value={p.base_url}
                      placeholder="Base URL"
                      class="rounded-lg border px-2 py-1.5 outline-none"
                      style="border-color: var(--border); background: var(--bg); color: var(--ink);"
                    />
                    <input
                      type="text"
                      bind:value={p.default_model}
                      placeholder="Default Model"
                      class="rounded-lg border px-2 py-1.5 outline-none"
                      style="border-color: var(--border); background: var(--bg); color: var(--ink);"
                    />
                    <input
                      type="password"
                      bind:value={keyInputs[p.id]}
                      placeholder={p.has_api_key ? '•••••••• (saved; enter new key to replace)' : 'API Key'}
                      class="rounded-lg border px-2 py-1.5 outline-none sm:col-span-2"
                      style="border-color: var(--border); background: var(--bg); color: var(--ink);"
                    />
                    <div class="flex flex-wrap items-center justify-between gap-2 sm:col-span-2">
                      <span class="text-[11px]" style="color: {testStatus[p.id] === 'OK' ? 'var(--success)' : 'var(--muted)'};">
                        {testStatus[p.id] || ''}
                      </span>
                      <div class="flex gap-2">
                        <button onclick={() => saveProviderKey(p)} class="rounded-lg border px-2.5 py-1 font-medium" style="border-color: var(--border);">
                          Save Provider
                        </button>
                        <button onclick={() => testProvider(p)} class="rounded-lg px-2.5 py-1 font-medium" style="background: var(--accent-soft); color: var(--accent);">
                          Test Connection
                        </button>
                      </div>
                    </div>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {:else if activeTab === 'routing'}
          <div class="space-y-3 text-xs">
            {#each [
              ['primary', 'Primary Writer (Drafting & Prose)'],
              ['utility', 'Utility Model (Eco Mode, Summaries & Fast Indexing)'],
              ['architect', 'Architect & Critic (Ultra Mode Planning & Continuity Audit)'],
            ] as [slotKey, title]}
              {@const k = slotKey as 'primary' | 'utility' | 'architect'}
              <div class="rounded-xl border p-3.5" style="border-color: var(--border); background: var(--surface-elevated);">
                <div class="mb-2 font-semibold">{title}</div>
                <div class="grid gap-2 sm:grid-cols-2">
                  <select
                    bind:value={draft.routing[k].provider_id}
                    class="rounded-lg border p-2 outline-none"
                    style="border-color: var(--border); background: var(--bg); color: var(--ink);"
                  >
                    {#each draft.providers.filter((x) => x.enabled || x.kind === 'mock') as p}
                      <option value={p.id}>{p.name}</option>
                    {/each}
                  </select>
                  <input
                    type="text"
                    bind:value={draft.routing[k].model}
                    placeholder="Model ID (leave blank for provider default)"
                    class="rounded-lg border p-2 outline-none"
                    style="border-color: var(--border); background: var(--bg); color: var(--ink);"
                  />
                </div>
              </div>
            {/each}
          </div>
        {:else if activeTab === 'export'}
          {#if studio.activeProject}
            <div class="space-y-3">
              <a
                href={exportUrl(studio.activeProject.id, 'epub')}
                download
                class="flex items-center justify-between rounded-xl border p-4 transition-colors hover:opacity-85"
                style="border-color: var(--border); background: var(--surface-elevated);"
              >
                <div>
                  <div class="font-semibold">Export EPUB 3 E-Book (.epub)</div>
                  <div class="text-xs" style="color: var(--muted);">พร้อมสำหรับเครื่องอ่าน E-Reader และมือถือ</div>
                </div>
                <Download size={18} style="color: var(--accent);" />
              </a>
              <a
                href={exportUrl(studio.activeProject.id, 'markdown')}
                download
                class="flex items-center justify-between rounded-xl border p-4 transition-colors hover:opacity-85"
                style="border-color: var(--border); background: var(--surface-elevated);"
              >
                <div>
                  <div class="font-semibold">Export Full Manuscript + Bible (.md)</div>
                  <div class="text-xs" style="color: var(--muted);">รวมทุกตอนและโปรไฟล์ตัวละครเป็นไฟล์เดียว</div>
                </div>
                <Download size={18} style="color: var(--accent);" />
              </a>
            </div>
          {/if}
        {/if}
      </div>

      <!-- Pinned Footer -->
      <div class="flex shrink-0 justify-end gap-2 border-t px-4 py-3 sm:px-5" style="border-color: var(--border);">
        <button onclick={closeModal} class="rounded-xl border px-4 py-2 text-xs font-medium" style="border-color: var(--border);">
          Cancel
        </button>
        <button
          onclick={saveAll}
          disabled={saving}
          class="rounded-xl px-5 py-2 text-xs font-semibold disabled:opacity-50"
          style="background: var(--accent); color: var(--accent-ink);"
        >
          {saving ? '...' : 'Save Settings'}
        </button>
      </div>
    </div>
  </div>
{/if}

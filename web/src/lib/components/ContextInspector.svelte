<script lang="ts">
  import { X, Layers, CheckCircle2, AlertCircle } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';

  let ctx = $derived(studio.contextPreview);
  let pct = $derived(
    ctx ? Math.min(100, Math.round((ctx.total_tokens / Math.max(1, ctx.budget_limit)) * 100)) : 0,
  );
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape' && studio.contextDrawerOpen) studio.contextDrawerOpen = false;
  }}
/>

{#if studio.contextDrawerOpen && ctx}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-3 sm:p-4"
    onclick={(e) => {
      if (e.target === e.currentTarget) studio.contextDrawerOpen = false;
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape') studio.contextDrawerOpen = false;
    }}
    tabindex="-1"
    role="dialog"
    aria-modal="true"
  >
    <div
      class="anim-sheet flex max-h-[85dvh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border shadow-xl"
      style="background: var(--bg); border-color: var(--border); color: var(--ink);"
    >
      <div class="flex shrink-0 items-center justify-between border-b px-5 py-3.5" style="border-color: var(--border);">
        <div class="flex items-center gap-2">
          <Layers size={18} style="color: var(--accent);" />
          <div>
            <h3 class="text-sm font-semibold">{studio.t.agent.contextInspector}</h3>
            <p class="text-xs" style="color: var(--muted);">
              Mode: {ctx.mode.toUpperCase()} • Skill: {ctx.skill_id} • {ctx.cache_friendly ? 'Prompt-Cache Aligned' : 'Eco Minimal'}
            </p>
          </div>
        </div>
        <button
          onclick={() => (studio.contextDrawerOpen = false)}
          class="rounded-lg p-1.5 hover:opacity-75"
          aria-label="Close"
        >
          <X size={18} />
        </button>
      </div>

      <div class="shrink-0 border-b px-5 py-3" style="border-color: var(--border); background: var(--surface);">
        <div class="mb-1.5 flex justify-between text-xs font-medium tabular-nums">
          <span>{studio.t.agent.totalTokens}: {ctx.total_tokens.toLocaleString()} / {ctx.budget_limit.toLocaleString()}</span>
          <span>{pct}%</span>
        </div>
        <div class="h-2 w-full overflow-hidden rounded-full" style="background: var(--border);">
          <div
            class="h-full rounded-full transition-all"
            style="width: {pct}%; background: var(--accent);"
          ></div>
        </div>
        <div class="mt-2 grid grid-cols-2 gap-2 text-center text-[11px] tabular-nums sm:grid-cols-4" style="color: var(--muted);">
          <div class="rounded-md p-1" style="background: var(--bg);">Tier 0 (Cache): {ctx.tier0_tokens}</div>
          <div class="rounded-md p-1" style="background: var(--bg);">Tier 1 (Scene): {ctx.tier1_tokens}</div>
          <div class="rounded-md p-1" style="background: var(--bg);">Tier 2 (Recall): {ctx.tier2_tokens}</div>
          <div class="rounded-md p-1" style="background: var(--bg);">Tier 3 (Summary): {ctx.tier3_tokens}</div>
        </div>
      </div>

      <div class="min-h-0 flex-1 space-y-2.5 overflow-y-auto p-5">
        {#each ctx.sections as sec}
          <div
            class="rounded-xl border p-3"
            style="border-color: var(--border); background: var(--surface-elevated); opacity: {sec.included ? 1 : 0.6};"
          >
            <div class="mb-1 flex items-center justify-between gap-2">
              <div class="flex items-center gap-1.5 text-xs font-semibold">
                {#if sec.included}
                  <CheckCircle2 size={14} style="color: var(--success);" />
                {:else}
                  <AlertCircle size={14} style="color: var(--warning);" />
                {/if}
                <span>[Tier {sec.tier}] {sec.title}</span>
              </div>
              <span class="rounded px-1.5 py-0.5 text-[11px] tabular-nums" style="background: var(--surface); color: var(--muted);">
                {sec.tokens} tok
              </span>
            </div>
            {#if sec.reason}
              <div class="mb-1 text-[11px]" style="color: var(--warning);">{sec.reason}</div>
            {/if}
            <p class="line-clamp-3 text-xs whitespace-pre-wrap" style="color: var(--muted);">
              {sec.preview}
            </p>
          </div>
        {/each}
      </div>
    </div>
  </div>
{/if}

<script lang="ts">
  import {
    Send,
    Square,
    Eye,
    Check,
    X,
    ChevronDown,
    ChevronRight,
    Leaf,
    Scale,
    Flame,
    Wrench,
    Sparkles,
    Trash2,
  } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';
  import { renderSafeMarkdown } from '../utils/markdown';
  import type { AgentMode } from '../types';

  let promptInput = $state('');
  let scrollEl: HTMLDivElement | undefined = $state();
  let expandedThinking = $state<Record<string, boolean>>({});

  const modes: Array<{ id: AgentMode; labelKey: 'eco' | 'balanced' | 'ultra'; descKey: 'ecoDesc' | 'balancedDesc' | 'ultraDesc' }> = [
    { id: 'eco', labelKey: 'eco', descKey: 'ecoDesc' },
    { id: 'balanced', labelKey: 'balanced', descKey: 'balancedDesc' },
    { id: 'ultra', labelKey: 'ultra', descKey: 'ultraDesc' },
  ];

  function selectMode(m: AgentMode) {
    studio.mode = m;
    studio.refreshContextPreview(promptInput);
  }

  async function submitPrompt() {
    const q = promptInput.trim();
    if (!q || studio.isStreaming) return;
    promptInput = '';
    await studio.sendAgentPrompt(q);
    scrollToBottom();
  }

  function scrollToBottom() {
    requestAnimationFrame(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    });
  }

  $effect(() => {
    studio.agentMessages.length;
    scrollToBottom();
  });

  let activeModeIndex = $derived(modes.findIndex((m) => m.id === studio.mode));
</script>

<div class="flex h-full flex-col overflow-hidden" style="background: var(--surface);">
  <!-- Mode Selector Pill (Eco / Balanced / Ultra) -->
  <div class="shrink-0 border-b p-3" style="border-color: var(--border);">
    <div
      class="relative grid grid-cols-3 rounded-xl p-1"
      style="background: var(--bg); border: 1px solid var(--border);"
    >
      <!-- Compositor-only gliding active indicator -->
      <div
        class="pointer-events-none absolute top-1 bottom-1 left-1 rounded-lg transition-transform"
        style="
          width: calc((100% - 8px) / 3);
          background: var(--accent);
          transform: translate3d({activeModeIndex * 100}%, 0, 0);
          transition-duration: var(--motion-duration);
          transition-timing-function: var(--motion-ease);
        "
      ></div>
      {#each modes as m}
        <button
          onclick={() => selectMode(m.id)}
          class="relative z-10 flex items-center justify-center gap-1.5 rounded-lg py-1.5 text-xs font-semibold transition-colors"
          style="color: {studio.mode === m.id ? 'var(--accent-ink)' : 'var(--muted)'};"
        >
          {#if m.id === 'eco'}
            <Leaf size={13} />
          {:else if m.id === 'balanced'}
            <Scale size={13} />
          {:else}
            <Flame size={13} />
          {/if}
          <span>{studio.t.modes[m.labelKey]}</span>
        </button>
      {/each}
    </div>
    <div class="mt-1.5 flex items-center justify-between gap-2 px-0.5 text-[11px]" style="color: var(--muted);">
      <span class="truncate">{studio.t.modes[modes[activeModeIndex]?.descKey || 'balancedDesc']}</span>
      {#if studio.contextPreview}
        <button
          onclick={() => {
            studio.refreshContextPreview(promptInput);
            studio.contextDrawerOpen = true;
          }}
          class="flex shrink-0 items-center gap-1 rounded-md px-1.5 py-0.5 font-medium tabular-nums transition-colors"
          style="background: var(--accent-soft); color: var(--accent);"
          title={studio.t.agent.contextInspector}
        >
          <Eye size={12} />
          <span>{studio.contextPreview.total_tokens.toLocaleString()} / {studio.contextPreview.budget_limit.toLocaleString()}</span>
        </button>
      {/if}
    </div>

    <!-- Skill Selector (Auto + Manual Override) -->
    <div class="mt-2.5 flex items-center gap-1.5">
      <div class="relative flex min-w-0 flex-1 items-center">
        {#if studio.selectedSkillId === 'auto'}
          <span
            class="pointer-events-none absolute left-2.5 flex items-center"
            style="color: var(--accent);"
          >
            <Sparkles size={13} />
          </span>
        {/if}
        <select
          value={studio.selectedSkillId}
          onchange={(e) => {
            studio.selectedSkillId = e.currentTarget.value;
            studio.refreshContextPreview(promptInput);
          }}
          class="w-full truncate rounded-lg border py-1.5 pr-2.5 text-xs font-medium outline-none {studio.selectedSkillId === 'auto' ? 'pl-7' : 'px-2.5'}"
          style="
            border-color: {studio.selectedSkillId === 'auto' ? 'var(--accent)' : 'var(--border)'};
            background: {studio.selectedSkillId === 'auto' ? 'var(--accent-soft)' : 'var(--bg)'};
            color: {studio.selectedSkillId === 'auto' ? 'var(--accent)' : 'var(--ink)'};
          "
        >
          <option value="auto" style="background: var(--bg); color: var(--ink);">
            {studio.t.agent.autoSkill}
          </option>
          <optgroup
            label={studio.lang === 'th' ? 'หรือเลือกเฉพาะทางเอง' : 'Or pick a specialist manually'}
            style="background: var(--bg); color: var(--muted);"
          >
            {#each studio.skills as sk}
              <option value={sk.id} style="background: var(--bg); color: var(--ink);">
                {studio.lang === 'th' ? sk.name_th || sk.name : sk.name} — ({sk.category})
              </option>
            {/each}
          </optgroup>
        </select>
      </div>
      {#if studio.selectedSkillId !== 'auto'}
        <button
          onclick={() => {
            studio.selectedSkillId = 'auto';
            studio.refreshContextPreview(promptInput);
          }}
          class="flex shrink-0 items-center gap-1 rounded-lg border px-2 py-1.5 text-[11px] font-semibold transition-colors"
          style="border-color: var(--accent); background: var(--accent-soft); color: var(--accent);"
          title={studio.t.agent.autoSkill}
        >
          <Sparkles size={12} />
          <span>{studio.t.agent.autoSkillShort}</span>
        </button>
      {/if}
      {#if studio.agentMessages.length > 0 && !studio.isStreaming}
        <button
          onclick={async () => {
            const ok = window.confirm(
              studio.lang === 'th' ? 'ต้องการล้างประวัติแชทเอเจนต์ของเรื่องนี้หรือไม่?' : 'Clear agent chat history for this novel?',
            );
            if (ok) await studio.clearAgentHistory();
          }}
          class="flex shrink-0 items-center justify-center rounded-lg border p-1.5 transition-colors hover:opacity-80"
          style="border-color: var(--border); color: var(--muted);"
          title={studio.lang === 'th' ? 'ล้างแชท (เริ่มคุยใหม่)' : 'Clear chat'}
          aria-label="Clear chat history"
        >
          <Trash2 size={13} />
        </button>
      {/if}
    </div>
  </div>

  <!-- Pending Proposals Banner inside Agent Desk -->
  {#if studio.proposals.filter((p) => p.status === 'pending').length > 0}
    <div class="shrink-0 border-b px-3 py-2" style="border-color: var(--border); background: var(--accent-soft);">
      {#each studio.proposals.filter((p) => p.status === 'pending') as prop}
        <div class="flex items-center justify-between gap-2 text-xs">
          <span class="truncate font-medium" style="color: var(--accent);">{prop.title}</span>
          <div class="flex shrink-0 gap-1">
            <button
              onclick={() => studio.rejectProposal(prop.id)}
              class="rounded p-1"
              style="color: var(--danger);"
              title={studio.t.agent.reject}
            >
              <X size={14} />
            </button>
            <button
              onclick={() => studio.applyProposal(prop.id)}
              class="rounded p-1"
              style="color: var(--success);"
              title={studio.t.agent.accept}
            >
              <Check size={14} />
            </button>
          </div>
        </div>
      {/each}
    </div>
  {/if}

  <!-- Conversation Stream -->
  <div bind:this={scrollEl} class="min-h-0 flex-1 space-y-3 overflow-y-auto p-3">
    {#if studio.agentMessages.length === 0}
      <div
        class="rounded-xl border p-4 text-xs leading-relaxed"
        style="border-color: var(--border); background: var(--bg); color: var(--muted);"
      >
        <p class="mb-2 font-semibold" style="color: var(--ink);">
          {studio.lang === 'th' ? 'พร้อมช่วยเขียนนิยายของคุณแล้ว' : 'Ready to Co-Write Your Story'}
        </p>
        <p class="mb-3">
          {studio.selectedSkillId === 'auto'
            ? studio.t.agent.autoSkillHint
            : studio.lang === 'th'
              ? 'เลือก Skill และโหมดด้านบนได้เลย เช่น:'
              : 'Pick a Skill and Mode above, or try one of these:'}
        </p>
        <div class="flex flex-wrap gap-1.5">
          {#each studio.lang === 'th'
            ? [
                'เขียนฉากต่อจากรอยร้าวโดยคุมมุมมองคิริน',
                'ช่วยวิเคราะห์และเพิ่มมิติย้อนแย้งให้คิริน',
                'ตรวจความต่อเนื่องของบทที่ 1 และ 2',
              ]
            : [
                'Continue this scene strictly in POV',
                'Stress-test the protagonist paradox & non-linear arc',
                'Audit continuity and knowledge boundaries',
              ] as starter}
            <button
              onclick={() => {
                promptInput = starter;
              }}
              class="rounded-lg border px-2.5 py-1 text-left text-[11px] transition-colors"
              style="border-color: var(--border); background: var(--surface); color: var(--ink);"
            >
              {starter}
            </button>
          {/each}
        </div>
      </div>
    {/if}

    {#each studio.agentMessages as msg}
      <div class="anim-fade">
        {#if msg.role === 'user'}
          <div
            class="ml-6 rounded-xl px-3.5 py-2.5 text-sm leading-relaxed"
            style="background: var(--surface-elevated); border: 1px solid var(--border); color: var(--ink);"
          >
            {msg.content}
          </div>
        {:else}
          <div
            class="rounded-xl border px-3.5 py-3 text-sm leading-relaxed"
            style="border-color: var(--border); background: var(--bg); color: var(--ink);"
          >
            <!-- Resolved Skill Badge (shows what Auto picked) -->
            {#if msg.skillId}
              <div class="mb-2 flex flex-wrap items-center gap-1.5">
                <span
                  class="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-semibold"
                  style="
                    background: {msg.autoPicked ? 'var(--accent-soft)' : 'var(--surface)'};
                    color: {msg.autoPicked ? 'var(--accent)' : 'var(--muted)'};
                    border: 1px solid {msg.autoPicked ? 'transparent' : 'var(--border)'};
                  "
                >
                  {#if msg.autoPicked}
                    <Sparkles size={10} />
                    <span>{studio.t.agent.autoPickedLabel} →</span>
                  {/if}
                  <span>{studio.lang === 'th' ? msg.skillNameTh || msg.skillName || msg.skillId : msg.skillName || msg.skillId}</span>
                </span>
              </div>
            {/if}

            <!-- Ultra Stages Timeline -->
            {#if msg.stages.length > 0}
              <div class="mb-2 flex flex-wrap gap-1">
                {#each msg.stages as st}
                  <span
                    class="rounded-md px-1.5 py-0.5 text-[10px] font-medium"
                    style="background: var(--accent-soft); color: var(--accent);"
                  >
                    {st}
                  </span>
                {/each}
              </div>
            {/if}

            <!-- Tool Chips -->
            {#if msg.tools.length > 0}
              <div class="mb-2 flex flex-wrap gap-1">
                {#each msg.tools as tc}
                  <span
                    class="inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[10px]"
                    style="border-color: var(--border); background: var(--surface); color: var(--muted);"
                  >
                    <Wrench size={10} />
                    {tc.name}
                  </span>
                {/each}
              </div>
            {/if}

            <!-- Collapsible Thinking -->
            {#if msg.thinking}
              <button
                onclick={() => (expandedThinking[msg.id] = !expandedThinking[msg.id])}
                class="mb-2 flex items-center gap-1 text-[11px] font-medium"
                style="color: var(--muted);"
              >
                {#if expandedThinking[msg.id]}
                  <ChevronDown size={13} />
                {:else}
                  <ChevronRight size={13} />
                {/if}
                {studio.lang === 'th' ? 'กระบวนการคิดของ Agent' : 'Agent Reasoning'}
              </button>
              {#if expandedThinking[msg.id]}
                <div
                  class="mb-2 rounded-lg p-2 text-xs whitespace-pre-wrap"
                  style="background: var(--surface); color: var(--muted);"
                >
                  {msg.thinking}
                </div>
              {/if}
            {/if}

            <div class="md-reply">
              {#if msg.content}
                {@html renderSafeMarkdown(msg.content)}
              {:else if studio.isStreaming}
                ...
              {/if}
            </div>

            {#if msg.usage}
              <div class="mt-2 flex items-center gap-3 border-t pt-1.5 text-[10px] tabular-nums" style="border-color: var(--border); color: var(--muted);">
                <span>In: {msg.usage.input_tokens}</span>
                <span>Cached: {msg.usage.cached_tokens}</span>
                <span>Out: {msg.usage.output_tokens}</span>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  </div>

  <!-- Composer -->
  <div class="shrink-0 border-t p-3" style="border-color: var(--border); background: var(--surface);">
    {#if studio.selectedText}
      <div
        class="mb-2 flex items-center justify-between rounded-lg px-2.5 py-1 text-[11px]"
        style="background: var(--accent-soft); color: var(--accent);"
      >
        <span class="truncate">Selected: "{studio.selectedText.slice(0, 60)}..."</span>
        <button onclick={() => (studio.selectedText = '')}><X size={12} /></button>
      </div>
    {/if}
    <div
      class="flex items-end gap-2 rounded-xl border p-2"
      style="border-color: var(--border); background: var(--bg);"
    >
      <textarea
        bind:value={promptInput}
        onkeydown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            submitPrompt();
          }
        }}
        rows={2}
        placeholder={studio.t.agent.askPlaceholder}
        class="max-h-32 min-h-[44px] flex-1 resize-none bg-transparent text-sm outline-none"
        style="color: var(--ink);"
      ></textarea>
      {#if studio.isStreaming}
        <button
          onclick={() => studio.stopAgent()}
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
          style="background: var(--danger); color: #fff;"
          title={studio.t.agent.stop}
        >
          <Square size={14} />
        </button>
      {:else}
        <button
          onclick={submitPrompt}
          disabled={!promptInput.trim()}
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition-opacity disabled:opacity-40"
          style="background: var(--accent); color: var(--accent-ink);"
          title={studio.t.agent.send}
        >
          <Send size={15} />
        </button>
      {/if}
    </div>
  </div>
</div>

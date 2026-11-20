<script lang="ts">
  import { X, Sparkles, PenLine, ShieldCheck, Users } from '@lucide/svelte';
  import { studio } from '../stores/studio.svelte';

  function trigger(skillId: string, prompt: string) {
    studio.quickAgentOpen = false;
    studio.rightDeskOpen = true;
    studio.mobileTab = 'agent';
    studio.sendAgentPrompt(prompt, skillId);
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape' && studio.quickAgentOpen) studio.quickAgentOpen = false;
  }}
/>

{#if studio.quickAgentOpen}
  <div
    class="fixed inset-0 z-40 flex items-end justify-center bg-black/35 md:items-center"
    onclick={(e) => {
      if (e.target === e.currentTarget) studio.quickAgentOpen = false;
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape') studio.quickAgentOpen = false;
    }}
    tabindex="-1"
    role="dialog"
    aria-modal="true"
  >
    <div
      class="anim-sheet safe-pb w-full max-w-lg rounded-t-2xl border p-4 shadow-2xl md:rounded-2xl"
      style="background: var(--bg); border-color: var(--border); box-shadow: var(--shadow-sheet);"
    >
      <div class="mb-3 flex items-center justify-between">
        <div class="flex items-center gap-2 text-sm font-semibold">
          <Sparkles size={16} style="color: var(--accent);" />
          <span>{studio.t.editor.quickAI}</span>
        </div>
        <button onclick={() => (studio.quickAgentOpen = false)} class="rounded-lg p-1">
          <X size={16} />
        </button>
      </div>

      <div class="grid gap-2 text-xs">
        <button
          onclick={() =>
            trigger(
              'scene-writer',
              studio.selectedText
                ? `ช่วยเขียนขยายและต่อยอดช่วงนี้โดยรักษา POV และน้ำเสียง:\n${studio.selectedText}`
                : 'เขียนฉากต่อจากย่อหน้าล่าสุดโดยอ้างอิง Beat Sheet และสถานะตัวละคร',
            )}
          class="flex items-center gap-2.5 rounded-xl border p-3 text-left transition-colors"
          style="border-color: var(--border); background: var(--surface-elevated);"
        >
          <PenLine size={16} style="color: var(--accent);" />
          <div>
            <div class="font-semibold">{studio.lang === 'th' ? 'เขียนฉากต่อ (Scene Writer)' : 'Continue Scene'}</div>
            <div style="color: var(--muted);">คุม POV และปลอดคำ AI Slop</div>
          </div>
        </button>

        <button
          onclick={() =>
            trigger(
              'prose-editor',
              studio.selectedText
                ? `เกลาสำนวนช่วงที่เลือกให้กระชับและคมขึ้น:\n${studio.selectedText}`
                : 'ตรวจเกลาสำนวนทั้งตอนล่าสุดและตัดคำซ้ำซาก',
            )}
          class="flex items-center gap-2.5 rounded-xl border p-3 text-left transition-colors"
          style="border-color: var(--border); background: var(--surface-elevated);"
        >
          <Sparkles size={16} style="color: var(--accent);" />
          <div>
            <div class="font-semibold">{studio.lang === 'th' ? 'เกลาสำนวน & แก้ AI Slop' : 'Polish Prose'}</div>
            <div style="color: var(--muted);">ส่งเป็น Diff ให้กดยอมรับทีละจุด</div>
          </div>
        </button>

        <button
          onclick={() => trigger('continuity-guard', 'ตรวจความต่อเนื่องและขอบเขตความรู้ตัวละครในตอนนี้เทียบกับ Timeline')}
          class="flex items-center gap-2.5 rounded-xl border p-3 text-left transition-colors"
          style="border-color: var(--border); background: var(--surface-elevated);"
        >
          <ShieldCheck size={16} style="color: var(--accent);" />
          <div>
            <div class="font-semibold">{studio.lang === 'th' ? 'ตรวจความต่อเนื่อง' : 'Audit Continuity'}</div>
            <div style="color: var(--muted);">เช็ก Unaware-Of, บาดแผล, ไอเทม</div>
          </div>
        </button>

        <button
          onclick={() => trigger('character-architect', 'ตรวจสอบปฏิกิริยาตัวละครในฉากนี้ว่าสอดคล้องกับ Core Paradox และกลไกป้องกันตัวหรือไม่')}
          class="flex items-center gap-2.5 rounded-xl border p-3 text-left transition-colors"
          style="border-color: var(--border); background: var(--surface-elevated);"
        >
          <Users size={16} style="color: var(--accent);" />
          <div>
            <div class="font-semibold">{studio.lang === 'th' ? 'เช็กมิติตัวละคร' : 'Check Character Depth'}</div>
            <div style="color: var(--muted);">เทียบกับ Mask vs True Self และ Non-Linear Arc</div>
          </div>
        </button>
      </div>
    </div>
  </div>
{/if}

import th from '../i18n/th.json';
import en from '../i18n/en.json';
import { api, initAuthToken, streamAgentRun } from '../api/client';
import type {
  AgentEvent,
  AgentMode,
  AppConfig,
  Chapter,
  Character,
  CharacterState,
  CloudSyncConfig,
  ConflictFile,
  ContextBreakdown,
  GitStatusResponse,
  Language,
  LoreEntry,
  Outline,
  PlotThread,
  ProjectSummary,
  Proposal,
  Relationship,
  Skill,
  ThemeName,
  TimelineEvent,
} from '../types';

export interface AgentTurnMessage {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  thinking?: string;
  mode: AgentMode;
  skillId?: string;
  skillName?: string;
  skillNameTh?: string;
  autoPicked?: boolean;
  stages: string[];
  tools: Array<{ name: string; args: string; output?: string }>;
  usage?: { input_tokens: number; cached_tokens: number; output_tokens: number };
}

class StudioStore {
  config = $state<AppConfig | null>(null);
  hasGitHubToken = $state(false);
  projects = $state<ProjectSummary[]>([]);
  activeProject = $state<ProjectSummary | null>(null);
  outline = $state<Outline | null>(null);
  chapters = $state<Chapter[]>([]);
  activeChapterId = $state<string>('');
  characters = $state<Character[]>([]);
  lore = $state<LoreEntry[]>([]);
  timeline = $state<TimelineEvent[]>([]);
  threads = $state<PlotThread[]>([]);
  relationships = $state<Relationship[]>([]);
  states = $state<CharacterState[]>([]);
  skills = $state<Skill[]>([]);
  proposals = $state<Proposal[]>([]);
  gitStatus = $state<GitStatusResponse | null>(null);
  gitConflicts = $state<ConflictFile[]>([]);
  cloudConfig = $state<CloudSyncConfig | null>(null);
  gdriveRedirectURI = $state('');
  hasGDriveSecret = $state(false);
  cloudConflicts = $state<{ provider: string; items: ConflictFile[] }>({ provider: '', items: [] });

  // UI state
  theme = $state<ThemeName>('paper');
  lang = $state<Language>('th');
  fontFamily = $state('sarabun');
  fontSize = $state(18);
  lineHeight = $state(1.88);
  reducedMotion = $state(false);

  leftRailOpen = $state(true);
  rightDeskOpen = $state(true);
  focusMode = $state(false);
  leftSection = $state<'chapters' | 'bible' | 'continuity' | 'skills' | 'git'>('chapters');
  mobileTab = $state<'write' | 'agent' | 'bible' | 'studio'>('write');
  settingsOpen = $state(false);
  contextDrawerOpen = $state(false);
  quickAgentOpen = $state(false);
  newNovelModalOpen = $state(false);
  editNovelModalOpen = $state(false);
  searchModalOpen = $state(false);

  // Agent State
  mode = $state<AgentMode>('balanced');
  selectedSkillId = $state('auto');
  selectedText = $state('');
  agentMessages = $state<AgentTurnMessage[]>([]);
  isStreaming = $state(false);
  contextPreview = $state<ContextBreakdown | null>(null);
  saveIndicator = $state<'idle' | 'saving' | 'saved'>('idle');
  toastMessage = $state('');
  androidFolder = $state<{
    available: boolean;
    granted: boolean;
    label: string;
    uri: string;
    lastSync: string;
  }>({
    available: false,
    granted: false,
    label: '',
    uri: '',
    lastSync: '',
  });

  private abortCtrl: AbortController | null = null;
  private saveTimer: ReturnType<typeof setTimeout> | null = null;
  private saveSeq = 0;

  get t() {
    return this.lang === 'en' ? en : th;
  }

  get activeChapter(): Chapter | null {
    return this.chapters.find((c) => c.id === this.activeChapterId) || this.chapters[0] || null;
  }

  notify(msg: string) {
    this.toastMessage = msg;
    setTimeout(() => {
      if (this.toastMessage === msg) this.toastMessage = '';
    }, 3200);
  }

  refreshAndroidFolderState() {
    const native = window.InkweaveNative;
    if (!native?.pickNovelFolder) {
      this.androidFolder = { available: false, granted: false, label: '', uri: '', lastSync: '' };
      return;
    }
    try {
      const raw = native.getNovelFolder?.() || '{}';
      const parsed = JSON.parse(raw);
      this.androidFolder = {
        available: true,
        granted: Boolean(parsed.granted),
        label: String(parsed.label || ''),
        uri: String(parsed.uri || ''),
        lastSync: String(parsed.lastSync || ''),
      };
    } catch {
      this.androidFolder = { available: true, granted: false, label: '', uri: '', lastSync: '' };
    }
  }

  pickAndroidNovelFolder() {
    window.InkweaveNative?.pickNovelFolder?.();
  }

  syncAndroidNovelFolderNow() {
    window.InkweaveNative?.syncNovelFolderNow?.();
  }

  async bootstrap() {
    initAuthToken();
    this.refreshAndroidFolderState();
    window.__inkweaveOnAndroidFolderChanged = async (payloadJson: string) => {
      try {
        const parsed = JSON.parse(payloadJson || '{}');
        this.androidFolder = {
          available: true,
          granted: Boolean(parsed.granted),
          label: String(parsed.label || ''),
          uri: String(parsed.uri || ''),
          lastSync: String(parsed.lastSync || ''),
        };
        if (parsed.message) {
          this.notify(String(parsed.message));
        }
        await this.loadProjects();
        if (this.activeProject) {
          const exists = this.projects.find((p) => p.id === this.activeProject?.id);
          if (exists) {
            await this.openProject(exists.id);
          } else if (this.projects.length > 0) {
            await this.openProject(this.projects[0].id);
          }
        } else if (this.projects.length > 0) {
          await this.openProject(this.projects[0].id);
        }
      } catch {
        this.refreshAndroidFolderState();
      }
    };
    await this.loadSettings();
    await this.loadProjects();
    if (this.projects.length > 0 && !this.activeProject) {
      let lastId = '';
      try {
        lastId = localStorage.getItem('inkweave_active_project') || '';
      } catch {
        // ignore storage errors
      }
      const target = this.projects.find((p) => p.id === lastId) || this.projects[0];
      await this.openProject(target.id);
    }
  }

  async loadSettings() {
    const res = await api<{ config: AppConfig; has_github_token: boolean }>('/api/settings');
    this.config = res.config;
    this.hasGitHubToken = res.has_github_token;
    const ui = res.config.ui;
    this.applyThemeVars(ui.theme, ui.font_family, ui.font_size, ui.line_height, ui.reduced_motion);
    this.lang = (ui.language as Language) || 'th';
    this.mode = (ui.default_mode as AgentMode) || 'balanced';
    await this.loadCloudConfig();
  }

  async loadCloudConfig() {
    try {
      const res = await api<{
        cloud_sync: CloudSyncConfig;
        gdrive_redirect_uri: string;
        has_gdrive_secret: boolean;
      }>('/api/cloud/config');
      this.cloudConfig = res.cloud_sync;
      this.gdriveRedirectURI = res.gdrive_redirect_uri;
      this.hasGDriveSecret = res.has_gdrive_secret;
    } catch {
      // ignore if older server
    }
  }

  applyThemeVars(theme: ThemeName, font: string, size: number, lineH: number, reduced: boolean) {
    this.theme = theme;
    this.fontFamily = font;
    this.fontSize = size;
    this.lineHeight = lineH;
    this.reducedMotion = reduced;
    const root = document.documentElement;
    root.setAttribute('data-theme', theme);
    root.setAttribute('data-font', font);
    root.setAttribute('data-reduced-motion', String(reduced));
    root.style.setProperty('--prose-size', `${size}px`);
    root.style.setProperty('--prose-line-height', `${lineH}`);
    const bgMap: Record<ThemeName, string> = {
      paper: '#FAF8F4',
      sepia: '#F3ECD8',
      'warm-dark': '#171614',
    };
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute('content', bgMap[theme]);
    window.InkweaveNative?.setTheme?.(theme);
  }

  async persistSettings(nextCfg: AppConfig, githubToken?: string) {
    const res = await api<{ config: AppConfig; has_github_token: boolean }>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify({
        config: nextCfg,
        ...(githubToken !== undefined ? { github_token: githubToken } : {}),
      }),
    });
    this.config = res.config;
    this.hasGitHubToken = res.has_github_token;
    const ui = res.config.ui;
    this.applyThemeVars(ui.theme, ui.font_family, ui.font_size, ui.line_height, ui.reduced_motion);
    this.lang = ui.language;
  }

  async loadProjects() {
    this.projects = await api<ProjectSummary[]>('/api/projects');
  }

  async openProject(id: string) {
    const [proj, outline, chapters, chars, lore, timeline, threads, rels, states, skills, props, history] =
      await Promise.all([
        api<ProjectSummary>(`/api/projects/${id}`),
        api<Outline>(`/api/projects/${id}/outline`),
        api<Chapter[]>(`/api/projects/${id}/chapters`),
        api<Character[]>(`/api/projects/${id}/characters`),
        api<LoreEntry[]>(`/api/projects/${id}/lore`),
        api<TimelineEvent[]>(`/api/projects/${id}/continuity/timeline`),
        api<PlotThread[]>(`/api/projects/${id}/continuity/threads`),
        api<Relationship[]>(`/api/projects/${id}/continuity/relationships`),
        api<CharacterState[]>(`/api/projects/${id}/continuity/states`),
        api<Skill[]>(`/api/projects/${id}/skills`),
        api<Proposal[]>(`/api/projects/${id}/proposals`),
        api<Array<{ id: string; role: 'user' | 'assistant'; content: string; mode: AgentMode }>>(`/api/projects/${id}/agent/history`),
      ]);

    this.activeProject = proj;
    try {
      localStorage.setItem('inkweave_active_project', proj.id);
    } catch {
      // ignore storage errors
    }
    this.outline = outline;
    this.chapters = chapters || [];
    this.activeChapterId = this.chapters[0]?.id || '';
    this.characters = chars || [];
    this.lore = lore || [];
    this.timeline = timeline || [];
    this.threads = threads || [];
    this.relationships = rels || [];
    this.states = states || [];
    this.skills = skills || [];
    this.proposals = props || [];
    this.agentMessages = (history || []).map((m) => ({
      id: m.id,
      role: m.role,
      content: m.content,
      mode: m.mode || 'balanced',
      stages: [],
      tools: [],
    }));
    this.gitConflicts = [];
    await this.refreshGitStatus();
    await this.refreshContextPreview('');
  }

  async createStarterProject() {
    const proj = await api<ProjectSummary>('/api/projects/starter', { method: 'POST' });
    await this.loadProjects();
    await this.openProject(proj.id);
  }

  async createProject(title: string, genre: string, language: string, synopsis: string) {
    const proj = await api<ProjectSummary>('/api/projects', {
      method: 'POST',
      body: JSON.stringify({ title, genre, language, synopsis }),
    });
    await this.loadProjects();
    await this.openProject(proj.id);
    this.notify(
      this.lang === 'th' ? `สร้างเรื่อง "${proj.title}" เรียบร้อยแล้ว` : `Created "${proj.title}"`,
    );
    return proj;
  }

  async deleteProject(id: string) {
    await api(`/api/projects/${id}`, { method: 'DELETE' });
    await this.loadProjects();
    if (this.activeProject?.id === id) {
      if (this.projects.length > 0) {
        await this.openProject(this.projects[0].id);
      } else {
        this.activeProject = null;
        this.chapters = [];
        this.activeChapterId = '';
      }
    }
    this.notify(this.lang === 'th' ? 'ลบเรื่องนิยายเรียบร้อยแล้ว' : 'Deleted novel');
  }

  async updateProject(patch: Partial<ProjectSummary>) {
    if (!this.activeProject) return;
    const payload = { ...this.activeProject, ...patch };
    const updated = await api<ProjectSummary>(`/api/projects/${this.activeProject.id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    });
    this.activeProject = updated;
    this.projects = this.projects.map((p) => (p.id === updated.id ? updated : p));
    this.notify(this.lang === 'th' ? 'บันทึกข้อมูลเรื่องนิยายแล้ว' : 'Saved novel settings');
    return updated;
  }

  async saveOutline(nextOutline: Outline) {
    if (!this.activeProject) return;
    this.outline = await api<Outline>(`/api/projects/${this.activeProject.id}/outline`, {
      method: 'PUT',
      body: JSON.stringify(nextOutline),
    });
    this.notify(this.lang === 'th' ? 'บันทึกโครงจังหวะฉาก (Beats) แล้ว' : 'Saved scene beats');
  }

  scheduleChapterSave(chapter: Chapter) {
    this.saveIndicator = 'saving';
    if (this.saveTimer) clearTimeout(this.saveTimer);
    const seq = ++this.saveSeq;
    const snapshot = { ...chapter };
    this.saveTimer = setTimeout(async () => {
      if (!this.activeProject) return;
      try {
        const updated = await api<Chapter>(
          `/api/projects/${this.activeProject.id}/chapters/${snapshot.id}`,
          { method: 'PUT', body: JSON.stringify(snapshot) },
        );
        this.chapters = this.chapters.map((c) => {
          if (c.id !== updated.id) return c;
          if (seq !== this.saveSeq) {
            return {
              ...updated,
              title: c.title,
              pov: c.pov,
              timeline_day: c.timeline_day,
              status: c.status,
              content: c.content,
            };
          }
          return updated;
        });
        if (seq === this.saveSeq) {
          this.saveIndicator = 'saved';
        }
      } catch (e) {
        if (seq === this.saveSeq) {
          this.saveIndicator = 'idle';
        }
        this.notify(String(e));
      }
    }, 550);
  }

  async addChapter(title: string) {
    if (!this.activeProject) return;
    const created = await api<Chapter>(`/api/projects/${this.activeProject.id}/chapters`, {
      method: 'POST',
      body: JSON.stringify({ title, status: 'draft', content: '' }),
    });
    this.chapters = [...this.chapters, created];
    this.activeChapterId = created.id;
    await this.loadProjects();
  }

  async deleteChapter(chapterId: string) {
    if (!this.activeProject) return;
    if (this.activeChapterId === chapterId && this.saveTimer) {
      clearTimeout(this.saveTimer);
      this.saveTimer = null;
      this.saveIndicator = 'idle';
    }
    const idx = this.chapters.findIndex((c) => c.id === chapterId);
    await api(`/api/projects/${this.activeProject.id}/chapters/${chapterId}`, { method: 'DELETE' });
    const remaining = this.chapters.filter((c) => c.id !== chapterId);
    this.chapters = remaining;
    if (this.activeChapterId === chapterId) {
      const next = remaining[Math.min(idx, remaining.length - 1)] || null;
      this.activeChapterId = next ? next.id : '';
    }
    await this.loadProjects();
    await this.refreshContextPreview('');
    this.notify(this.lang === 'th' ? 'ลบตอนเรียบร้อยแล้ว' : 'Deleted chapter');
  }

  async reorderChapters(orderedIds: string[]) {
    if (!this.activeProject) return;
    const updated = await api<Chapter[]>(`/api/projects/${this.activeProject.id}/chapters/reorder`, {
      method: 'POST',
      body: JSON.stringify({ ordered_ids: orderedIds }),
    });
    this.chapters = updated || [];
  }

  async clearAgentHistory() {
    if (!this.activeProject) return;
    await api(`/api/projects/${this.activeProject.id}/agent/history`, { method: 'DELETE' });
    this.agentMessages = [];
    this.notify(this.lang === 'th' ? 'ล้างประวัติแชทเอเจนต์แล้ว' : 'Cleared agent chat history');
  }

  async refreshContextPreview(promptText: string) {
    if (!this.activeProject) return;
    try {
      this.contextPreview = await api<ContextBreakdown>(
        `/api/projects/${this.activeProject.id}/context/preview`,
        {
          method: 'POST',
          body: JSON.stringify({
            mode: this.mode,
            skill_id: this.selectedSkillId,
            chapter_id: this.activeChapterId,
            prompt: promptText,
            selected_text: this.selectedText,
          }),
        },
      );
    } catch {
      // ignore preview error
    }
  }

  async sendAgentPrompt(prompt: string, overrideSkill?: string) {
    if (!this.activeProject || !prompt.trim() || this.isStreaming) return;
    const skillId = overrideSkill || this.selectedSkillId;
    if (overrideSkill && this.selectedSkillId !== 'auto') {
      this.selectedSkillId = overrideSkill;
    }

    const userMsg: AgentTurnMessage = {
      id: `u-${Date.now()}`,
      role: 'user',
      content: prompt,
      mode: this.mode,
      stages: [],
      tools: [],
    };
    const asstMsg: AgentTurnMessage = {
      id: `a-${Date.now()}`,
      role: 'assistant',
      content: '',
      thinking: '',
      mode: this.mode,
      stages: [],
      tools: [],
    };
    this.agentMessages = [...this.agentMessages, userMsg, asstMsg];
    this.isStreaming = true;
    this.abortCtrl = new AbortController();

    const updateAsst = (fn: (m: AgentTurnMessage) => void) => {
      const idx = this.agentMessages.length - 1;
      const copy = { ...this.agentMessages[idx] };
      fn(copy);
      this.agentMessages = [...this.agentMessages.slice(0, idx), copy];
    };

    try {
      await streamAgentRun(
        this.activeProject.id,
        {
          chapter_id: this.activeChapterId,
          skill_id: skillId,
          mode: this.mode,
          prompt,
          selected_text: this.selectedText,
        },
        (ev: AgentEvent) => {
          switch (ev.type) {
            case 'skill_selected':
              updateAsst((m) => {
                m.skillId = ev.skill_id;
                m.skillName = ev.skill_name;
                m.skillNameTh = ev.skill_name_th;
                m.autoPicked = ev.auto_picked;
              });
              break;
            case 'context_summary':
              if (ev.context) this.contextPreview = ev.context;
              break;
            case 'stage':
              if (ev.label) {
                updateAsst((m) => {
                  m.stages = [...m.stages, ev.label!];
                });
              }
              break;
            case 'thinking_delta':
              updateAsst((m) => {
                m.thinking = (m.thinking || '') + (ev.thinking_delta || '');
              });
              break;
            case 'token_delta':
              updateAsst((m) => {
                m.content += ev.delta || '';
              });
              break;
            case 'tool_start':
              updateAsst((m) => {
                m.tools = [...m.tools, { name: ev.tool_name || '', args: ev.tool_args || '' }];
              });
              break;
            case 'tool_result':
              updateAsst((m) => {
                const list = [...m.tools];
                if (list.length > 0) {
                  list[list.length - 1] = { ...list[list.length - 1], output: ev.tool_output };
                  m.tools = list;
                }
              });
              break;
            case 'proposal':
              if (ev.proposal) {
                this.proposals = [ev.proposal, ...this.proposals];
              }
              break;
            case 'usage':
              if (ev.usage) {
                updateAsst((m) => {
                  m.usage = ev.usage;
                });
              }
              break;
            case 'error':
              updateAsst((m) => {
                m.content += `\n\n⚠️ ${ev.error}`;
              });
              break;
          }
        },
        this.abortCtrl.signal,
      );
      // Reload characters/lore/continuity in case write tools updated them
      await this.refreshProjectCollections();
    } catch (err) {
      if ((err as Error).name !== 'AbortError') {
        updateAsst((m) => {
          m.content += `\n\n⚠️ ${String(err)}`;
        });
      }
    } finally {
      this.isStreaming = false;
      this.abortCtrl = null;
    }
  }

  stopAgent() {
    this.abortCtrl?.abort();
    this.isStreaming = false;
  }

  async refreshProjectCollections() {
    if (!this.activeProject) return;
    const id = this.activeProject.id;
    const [chars, lore, tl, th, rels, sts] = await Promise.all([
      api<Character[]>(`/api/projects/${id}/characters`),
      api<LoreEntry[]>(`/api/projects/${id}/lore`),
      api<TimelineEvent[]>(`/api/projects/${id}/continuity/timeline`),
      api<PlotThread[]>(`/api/projects/${id}/continuity/threads`),
      api<Relationship[]>(`/api/projects/${id}/continuity/relationships`),
      api<CharacterState[]>(`/api/projects/${id}/continuity/states`),
    ]);
    this.characters = chars || [];
    this.lore = lore || [];
    this.timeline = tl || [];
    this.threads = th || [];
    this.relationships = rels || [];
    this.states = sts || [];
  }

  async applyProposal(proposalId: string) {
    if (!this.activeProject) return;
    const res = await api<{ proposal: Proposal; chapter: Chapter }>(
      `/api/projects/${this.activeProject.id}/proposals/${proposalId}/apply`,
      { method: 'POST' },
    );
    this.proposals = this.proposals.filter((p) => p.id !== proposalId);
    if (res.chapter) {
      this.chapters = this.chapters.map((c) => (c.id === res.chapter.id ? res.chapter : c));
    }
    this.notify(this.lang === 'th' ? 'นำการแก้ไขเข้าสู่ต้นฉบับแล้ว (บันทึก Snapshot ไว้แล้ว)' : 'Applied edit to manuscript (snapshot saved)');
  }

  async rejectProposal(proposalId: string) {
    if (!this.activeProject) return;
    await api(`/api/projects/${this.activeProject.id}/proposals/${proposalId}/reject`, { method: 'POST' });
    this.proposals = this.proposals.filter((p) => p.id !== proposalId);
  }

  async saveCharacter(c: Character) {
    if (!this.activeProject) return;
    const saved = await api<Character>(`/api/projects/${this.activeProject.id}/characters/${c.id || ''}`, {
      method: c.id ? 'PUT' : 'POST',
      body: JSON.stringify(c),
    });
    const exists = this.characters.some((x) => x.id === saved.id);
    this.characters = exists
      ? this.characters.map((x) => (x.id === saved.id ? saved : x))
      : [...this.characters, saved];
    await this.loadProjects();
    this.notify(this.lang === 'th' ? 'บันทึกข้อมูลตัวละครแล้ว' : 'Saved character profile');
  }

  async deleteCharacter(charId: string) {
    if (!this.activeProject) return;
    await api(`/api/projects/${this.activeProject.id}/characters/${charId}`, { method: 'DELETE' });
    this.characters = this.characters.filter((c) => c.id !== charId);
    await this.loadProjects();
    this.notify(this.lang === 'th' ? 'ลบตัวละครเรียบร้อยแล้ว' : 'Deleted character');
  }

  async deleteLore(category: LoreEntry['category'], loreId: string) {
    if (!this.activeProject) return;
    await api(`/api/projects/${this.activeProject.id}/lore/${category}/${loreId}`, { method: 'DELETE' });
    this.lore = this.lore.filter((l) => !(l.id === loreId && l.category === category));
    this.notify(this.lang === 'th' ? 'ลบข้อมูลคลังเรื่องเรียบร้อยแล้ว' : 'Deleted lore entry');
  }

  async saveRelationships(list: Relationship[]) {
    if (!this.activeProject) return;
    this.relationships = await api<Relationship[]>(
      `/api/projects/${this.activeProject.id}/continuity/relationships`,
      { method: 'PUT', body: JSON.stringify(list) },
    );
    this.notify(this.lang === 'th' ? 'บันทึกความสัมพันธ์แล้ว' : 'Saved relationships');
  }

  async saveStates(list: CharacterState[]) {
    if (!this.activeProject) return;
    this.states = await api<CharacterState[]>(
      `/api/projects/${this.activeProject.id}/continuity/states`,
      { method: 'PUT', body: JSON.stringify(list) },
    );
    this.notify(this.lang === 'th' ? 'บันทึกสถานะตัวละครแล้ว' : 'Saved character states');
  }

  async refreshGitStatus() {
    if (!this.activeProject) return;
    try {
      this.gitStatus = await api<GitStatusResponse>(`/api/projects/${this.activeProject.id}/git/status`);
    } catch {
      this.gitStatus = null;
    }
  }
}

export const studio = new StudioStore();

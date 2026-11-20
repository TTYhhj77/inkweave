export type AgentMode = 'eco' | 'balanced' | 'ultra';
export type ThemeName = 'paper' | 'sepia' | 'warm-dark';
export type Language = 'th' | 'en';
export type ApprovalMode = 'review' | 'auto';

export interface ProjectSummary {
  id: string;
  slug: string;
  title: string;
  author: string;
  synopsis: string;
  language: string;
  genre: string;
  pov: string;
  tense: string;
  style_rules: string[];
  anti_cliches: string[];
  default_mode: AgentMode;
  created_at: string;
  updated_at: string;
  chapter_count: number;
  total_words: number;
  total_chars: number;
  character_count: number;
  has_git: boolean;
  path: string;
}

export interface Chapter {
  id: string;
  number: number;
  slug: string;
  title: string;
  pov: string;
  timeline_day: string;
  status: string;
  summary: string;
  word_count: number;
  char_count: number;
  updated_at: string;
  content: string;
  file_name: string;
}

export interface SceneBeat {
  id: string;
  summary: string;
  pov?: string;
  turning?: string;
  completed: boolean;
}

export interface ChapterOutline {
  chapter_id: string;
  title: string;
  goal: string;
  target_words: number;
  beats: SceneBeat[];
}

export interface Outline {
  premise: string;
  acts: Array<{
    id: string;
    title: string;
    summary: string;
    arcs: Array<{
      id: string;
      title: string;
      summary: string;
      chapters: ChapterOutline[];
    }>;
  }>;
}

export interface CharacterArcPhase {
  id: string;
  phase_order: number;
  label: string;
  chapter_ref: string;
  trajectory_value: number;
  internal_shift: string;
  trigger_event: string;
  is_regression: boolean;
}

export interface Character {
  id: string;
  slug: string;
  name: string;
  aliases: string[];
  role: string;
  age: string;
  appearance: string;
  tags: string[];
  summary: string;
  core_paradox: string;
  mask: string;
  true_self: string;
  blind_spot: string;
  formative_wound: string;
  misbelief: string;
  buried_truth: string;
  external_want: string;
  internal_need: string;
  moral_lines: string[];
  breaking_points: string[];
  voice_guide: string;
  speech_examples: string[];
  avoids_saying: string;
  defense_mechanisms: string[];
  secrets: string[];
  arc_type: string;
  arc_phases: CharacterArcPhase[];
  bio_markdown: string;
}

export interface LoreEntry {
  id: string;
  category: 'locations' | 'factions' | 'items' | 'rules';
  slug: string;
  name: string;
  aliases: string[];
  tags: string[];
  summary: string;
  attributes: Record<string, string>;
  content: string;
}

export interface TimelineEvent {
  id: string;
  order: number;
  day_label: string;
  chapter_id: string;
  location: string;
  participants: string[];
  summary: string;
  impact: string;
  locked: boolean;
}

export interface PlotThread {
  id: string;
  title: string;
  type: string;
  status: string;
  planted_chapter: string;
  payoff_chapter: string;
  related_characters: string[];
  notes: string;
}

export interface Relationship {
  id: string;
  from_char_id: string;
  to_char_id: string;
  label: string;
  trust: number;
  tension: number;
  power_dynamic: string;
  public_face: string;
  hidden_agenda: string;
  updated_chapter: string;
}

export interface CharacterState {
  chapter_id: string;
  character_id: string;
  location: string;
  emotional_state: string;
  physical_condition: string;
  inventory: string[];
  knows: string[];
  unaware_of: string[];
  active_goal: string;
  notes: string;
}

export interface Skill {
  id: string;
  name: string;
  name_th: string;
  description: string;
  description_th: string;
  category: string;
  recommended_modes: string[];
  tools: string[];
  built_in: boolean;
  custom: boolean;
  instructions: string;
}

export interface ProviderConfig {
  id: string;
  name: string;
  kind: string;
  base_url: string;
  default_model: string;
  enabled: boolean;
  has_api_key: boolean;
}

export type CloudProviderKind = 'github' | 'gdrive' | 'webdav' | 'folder';

export interface CloudSyncConfig {
  active_provider: CloudProviderKind;
  google_drive: {
    client_id: string;
    root_folder: string;
    account_email: string;
    connected: boolean;
  };
  webdav: {
    base_url: string;
    username: string;
    root_path: string;
    enabled: boolean;
    has_password: boolean;
  };
  local_folder: {
    path: string;
    enabled: boolean;
  };
}

export interface RemoteNovel {
  slug: string;
  title: string;
  file_count: number;
  updated_at: string;
}

export interface CloudSyncStatus {
  provider: string;
  last_sync_at?: string;
  tracked_count: number;
  modified_since_sync: string[];
}

export interface CloudSyncResult {
  provider: string;
  status: 'ok' | 'conflicts';
  message: string;
  uploaded: number;
  downloaded: number;
  deleted: number;
  conflicts?: ConflictFile[];
  synced_at: string;
}

export interface AppConfig {
  providers: ProviderConfig[];
  routing: {
    primary: { provider_id: string; model: string };
    utility: { provider_id: string; model: string };
    architect: { provider_id: string; model: string };
  };
  ui: {
    theme: ThemeName;
    language: Language;
    font_family: string;
    font_size: number;
    line_height: number;
    reduced_motion: boolean;
    default_mode: AgentMode;
    approval_mode: ApprovalMode;
    ultra_budget_cap: number;
  };
  cloud_sync?: CloudSyncConfig;
  git_author: {
    name: string;
    email: string;
  };
}

export interface ContextSection {
  id: string;
  tier: number;
  title: string;
  tokens: number;
  included: boolean;
  reason?: string;
  preview: string;
}

export interface ContextBreakdown {
  mode: AgentMode;
  skill_id: string;
  total_tokens: number;
  budget_limit: number;
  tier0_tokens: number;
  tier1_tokens: number;
  tier2_tokens: number;
  tier3_tokens: number;
  cache_friendly: boolean;
  sections: ContextSection[];
  truncated_items: string[];
}

export interface DiffHunk {
  type: 'equal' | 'delete' | 'insert';
  text: string;
}

export interface Proposal {
  id: string;
  project_id: string;
  tool_name: string;
  target_kind: string;
  target_id: string;
  title: string;
  summary: string;
  before_text: string;
  after_text: string;
  hunks: DiffHunk[];
  status: 'pending' | 'applied' | 'rejected';
  created_at: string;
}

export interface Snapshot {
  id: string;
  chapter_id: string;
  reason: string;
  created_at: string;
  word_count: number;
  preview: string;
}

export interface AgentEvent {
  type: string;
  stage?: string;
  label?: string;
  skill_id?: string;
  skill_name?: string;
  skill_name_th?: string;
  auto_picked?: boolean;
  route_method?: string;
  delta?: string;
  thinking_delta?: string;
  tool_name?: string;
  tool_args?: string;
  tool_output?: string;
  context?: ContextBreakdown;
  proposal?: Proposal;
  usage?: {
    input_tokens: number;
    cached_tokens: number;
    output_tokens: number;
  };
  error?: string;
  final_text?: string;
}

export interface ConflictFile {
  path: string;
  base: string;
  local: string;
  remote: string;
}

export interface GitStatusResponse {
  status: {
    initialized: boolean;
    branch: string;
    remote_url: string;
    dirty_files: string[];
    has_conflicts: boolean;
  };
  smart_message: string;
}

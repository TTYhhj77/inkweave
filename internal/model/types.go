package model

import "time"

// AgentMode selects the speed, token budget, and multi-stage rigor of the AI agent.
type AgentMode string

const (
	ModeEco      AgentMode = "eco"
	ModeBalanced AgentMode = "balanced"
	ModeUltra    AgentMode = "ultra"
)

// ApprovalMode controls whether agent write tools queue reviewable diffs or apply immediately.
type ApprovalMode string

const (
	ApprovalReview    ApprovalMode = "review"
	ApprovalAutoApply ApprovalMode = "auto"
)

// Project represents inkweave.yaml at the root of a novel repository.
type Project struct {
	ID          string    `yaml:"id" json:"id"`
	Slug        string    `yaml:"slug" json:"slug"`
	Title       string    `yaml:"title" json:"title"`
	Author      string    `yaml:"author" json:"author"`
	Synopsis    string    `yaml:"synopsis" json:"synopsis"`
	Language    string    `yaml:"language" json:"language"` // "th" or "en"
	Genre       string    `yaml:"genre" json:"genre"`
	POV         string    `yaml:"pov" json:"pov"`
	Tense       string    `yaml:"tense" json:"tense"`
	StyleRules  []string  `yaml:"style_rules" json:"style_rules"`
	AntiCliches []string  `yaml:"anti_cliches" json:"anti_cliches"`
	DefaultMode AgentMode `yaml:"default_mode" json:"default_mode"`
	CreatedAt   time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt   time.Time `yaml:"updated_at" json:"updated_at"`
}

// ProjectSummary provides lightweight list metadata for the Project Library.
type ProjectSummary struct {
	Project
	ChapterCount   int    `json:"chapter_count"`
	TotalWords     int    `json:"total_words"`
	TotalChars     int    `json:"total_chars"`
	CharacterCount int    `json:"character_count"`
	HasGit         bool   `json:"has_git"`
	Path           string `json:"path"`
}

// Outline represents manuscript/outline.yaml.
type Outline struct {
	Premise string `yaml:"premise" json:"premise"`
	Acts    []Act  `yaml:"acts" json:"acts"`
}

type Act struct {
	ID      string `yaml:"id" json:"id"`
	Title   string `yaml:"title" json:"title"`
	Summary string `yaml:"summary" json:"summary"`
	Arcs    []Arc  `yaml:"arcs" json:"arcs"`
}

type Arc struct {
	ID       string           `yaml:"id" json:"id"`
	Title    string           `yaml:"title" json:"title"`
	Summary  string           `yaml:"summary" json:"summary"`
	Chapters []ChapterOutline `yaml:"chapters" json:"chapters"`
}

type ChapterOutline struct {
	ChapterID   string      `yaml:"chapter_id" json:"chapter_id"`
	Title       string      `yaml:"title" json:"title"`
	Goal        string      `yaml:"goal" json:"goal"`
	TargetWords int         `yaml:"target_words" json:"target_words"`
	Beats       []SceneBeat `yaml:"beats" json:"beats"`
}

type SceneBeat struct {
	ID          string `yaml:"id" json:"id"`
	Summary     string `yaml:"summary" json:"summary"`
	POV         string `yaml:"pov,omitempty" json:"pov,omitempty"`
	Turning     string `yaml:"turning,omitempty" json:"turning,omitempty"`
	Completed   bool   `yaml:"completed" json:"completed"`
}

// Chapter represents a chapter file in manuscript/chapters/NNN-slug.md.
type Chapter struct {
	ID          string    `yaml:"id" json:"id"`
	Number      int       `yaml:"number" json:"number"`
	Slug        string    `yaml:"slug" json:"slug"`
	Title       string    `yaml:"title" json:"title"`
	POV         string    `yaml:"pov" json:"pov"`
	TimelineDay string    `yaml:"timeline_day" json:"timeline_day"`
	Status      string    `yaml:"status" json:"status"` // outline | draft | revising | final
	Summary     string    `yaml:"summary" json:"summary"`
	WordCount   int       `yaml:"word_count" json:"word_count"`
	CharCount   int       `yaml:"char_count" json:"char_count"`
	UpdatedAt   time.Time `yaml:"updated_at" json:"updated_at"`
	Content     string    `yaml:"-" json:"content"`
	FileName    string    `yaml:"-" json:"file_name"`
}

// Character represents a multi-dimensional, non-linear character profile in bible/characters/<slug>.md.
type Character struct {
	ID                string              `yaml:"id" json:"id"`
	Slug              string              `yaml:"slug" json:"slug"`
	Name              string              `yaml:"name" json:"name"`
	Aliases           []string            `yaml:"aliases" json:"aliases"`
	Role              string              `yaml:"role" json:"role"` // protagonist | antagonist | foil | mentor | ensemble
	Age               string              `yaml:"age" json:"age"`
	Appearance        string              `yaml:"appearance" json:"appearance"`
	Tags              []string            `yaml:"tags" json:"tags"`
	Summary           string              `yaml:"summary" json:"summary"`
	CoreParadox       string              `yaml:"core_paradox" json:"core_paradox"`
	Mask              string              `yaml:"mask" json:"mask"`
	TrueSelf          string              `yaml:"true_self" json:"true_self"`
	BlindSpot         string              `yaml:"blind_spot" json:"blind_spot"`
	FormativeWound    string              `yaml:"formative_wound" json:"formative_wound"`
	Misbelief         string              `yaml:"misbelief" json:"misbelief"`
	BuriedTruth       string              `yaml:"buried_truth" json:"buried_truth"`
	ExternalWant      string              `yaml:"external_want" json:"external_want"`
	InternalNeed      string              `yaml:"internal_need" json:"internal_need"`
	MoralLines        []string            `yaml:"moral_lines" json:"moral_lines"`
	BreakingPoints    []string            `yaml:"breaking_points" json:"breaking_points"`
	VoiceGuide        string              `yaml:"voice_guide" json:"voice_guide"`
	SpeechExamples    []string            `yaml:"speech_examples" json:"speech_examples"`
	AvoidsSaying      string              `yaml:"avoids_saying" json:"avoids_saying"`
	DefenseMechanisms []string            `yaml:"defense_mechanisms" json:"defense_mechanisms"`
	Secrets           []string            `yaml:"secrets" json:"secrets"`
	ArcType           string              `yaml:"arc_type" json:"arc_type"` // positive | negative | flat | spiral | oscillating | fractured | redemption | tragic
	ArcPhases         []CharacterArcPhase `yaml:"arc_phases" json:"arc_phases"`
	UpdatedAt         time.Time           `yaml:"updated_at" json:"updated_at"`
	BioMarkdown       string              `yaml:"-" json:"bio_markdown"`
	FileName          string              `yaml:"-" json:"file_name"`
}

// CharacterArcPhase captures non-linear psychological progression and regressions across chapters.
type CharacterArcPhase struct {
	ID              string `yaml:"id" json:"id"`
	PhaseOrder      int    `yaml:"phase_order" json:"phase_order"`
	Label           string `yaml:"label" json:"label"`
	ChapterRef      string `yaml:"chapter_ref" json:"chapter_ref"`
	TrajectoryValue int    `yaml:"trajectory_value" json:"trajectory_value"` // -100..100
	InternalShift   string `yaml:"internal_shift" json:"internal_shift"`
	TriggerEvent    string `yaml:"trigger_event" json:"trigger_event"`
	IsRegression    bool   `yaml:"is_regression" json:"is_regression"`
}

// LoreEntry represents locations, factions, items, or world rules in bible/<category>/<slug>.md.
type LoreEntry struct {
	ID         string            `yaml:"id" json:"id"`
	Category   string            `yaml:"category" json:"category"` // locations | factions | items | rules
	Slug       string            `yaml:"slug" json:"slug"`
	Name       string            `yaml:"name" json:"name"`
	Aliases    []string          `yaml:"aliases" json:"aliases"`
	Tags       []string          `yaml:"tags" json:"tags"`
	Summary    string            `yaml:"summary" json:"summary"`
	Attributes map[string]string `yaml:"attributes" json:"attributes"`
	UpdatedAt  time.Time         `yaml:"updated_at" json:"updated_at"`
	Content    string            `yaml:"-" json:"content"`
	FileName   string            `yaml:"-" json:"file_name"`
}

// TimelineEvent records chronological events in continuity/timeline.yaml.
type TimelineEvent struct {
	ID           string   `yaml:"id" json:"id"`
	Order        int      `yaml:"order" json:"order"`
	DayLabel     string   `yaml:"day_label" json:"day_label"`
	ChapterID    string   `yaml:"chapter_id" json:"chapter_id"`
	Location     string   `yaml:"location" json:"location"`
	Participants []string `yaml:"participants" json:"participants"`
	Summary      string   `yaml:"summary" json:"summary"`
	Impact       string   `yaml:"impact" json:"impact"`
	Locked       bool     `yaml:"locked" json:"locked"`
}

// PlotThread tracks Chekhov's guns, mysteries, and subplots in continuity/threads.yaml.
type PlotThread struct {
	ID                string   `yaml:"id" json:"id"`
	Title             string   `yaml:"title" json:"title"`
	Type              string   `yaml:"type" json:"type"`     // mystery | subplot | foreshadowing | promise | secret
	Status            string   `yaml:"status" json:"status"` // planted | active | escalated | paid_off | abandoned
	PlantedChapter    string   `yaml:"planted_chapter" json:"planted_chapter"`
	PayoffChapter     string   `yaml:"payoff_chapter" json:"payoff_chapter"`
	RelatedCharacters []string `yaml:"related_characters" json:"related_characters"`
	Notes             string   `yaml:"notes" json:"notes"`
}

// Relationship captures asymmetric, evolving character ties in continuity/relationships.yaml.
type Relationship struct {
	ID             string              `yaml:"id" json:"id"`
	FromCharID     string              `yaml:"from_char_id" json:"from_char_id"`
	ToCharID       string              `yaml:"to_char_id" json:"to_char_id"`
	Label          string              `yaml:"label" json:"label"`
	Trust          int                 `yaml:"trust" json:"trust"`     // -100..100
	Tension        int                 `yaml:"tension" json:"tension"` // 0..100
	PowerDynamic   string              `yaml:"power_dynamic" json:"power_dynamic"`
	PublicFace     string              `yaml:"public_face" json:"public_face"`
	HiddenAgenda   string              `yaml:"hidden_agenda" json:"hidden_agenda"`
	UpdatedChapter string              `yaml:"updated_chapter" json:"updated_chapter"`
	History        []RelationshipShift `yaml:"history" json:"history"`
}

type RelationshipShift struct {
	ChapterID string `yaml:"chapter_id" json:"chapter_id"`
	Event     string `yaml:"event" json:"event"`
	NewLabel  string `yaml:"new_label" json:"new_label"`
	Trust     int    `yaml:"trust" json:"trust"`
	Tension   int    `yaml:"tension" json:"tension"`
}

// CharacterState snapshots a character's exact state and knowledge boundary per chapter.
type CharacterState struct {
	ChapterID         string   `yaml:"chapter_id" json:"chapter_id"`
	CharacterID       string   `yaml:"character_id" json:"character_id"`
	Location          string   `yaml:"location" json:"location"`
	EmotionalState    string   `yaml:"emotional_state" json:"emotional_state"`
	PhysicalCondition string   `yaml:"physical_condition" json:"physical_condition"`
	Inventory         []string `yaml:"inventory" json:"inventory"`
	Knows             []string `yaml:"knows" json:"knows"`
	UnawareOf         []string `yaml:"unaware_of" json:"unaware_of"`
	ActiveGoal        string   `yaml:"active_goal" json:"active_goal"`
	Notes             string   `yaml:"notes" json:"notes"`
}

// Skill defines a modular Markdown+YAML skill in skills/*.md.
type Skill struct {
	ID               string   `yaml:"id" json:"id"`
	Name             string   `yaml:"name" json:"name"`
	NameTH           string   `yaml:"name_th" json:"name_th"`
	Description      string   `yaml:"description" json:"description"`
	DescriptionTH    string   `yaml:"description_th" json:"description_th"`
	Category         string   `yaml:"category" json:"category"` // craft | character | continuity | planning | memory
	RecommendedModes []string `yaml:"recommended_modes" json:"recommended_modes"`
	Tools            []string `yaml:"tools" json:"tools"`
	BuiltIn          bool     `yaml:"-" json:"built_in"`
	Custom           bool     `yaml:"-" json:"custom"`
	Instructions     string   `yaml:"-" json:"instructions"`
}

// ProviderConfig defines a user's LLM API endpoint (non-secret fields).
type ProviderConfig struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"` // openai | anthropic | gemini | openai-compat | mock
	BaseURL     string `json:"base_url"`
	DefaultModel string `json:"default_model"`
	Enabled     bool   `json:"enabled"`
	HasAPIKey   bool   `json:"has_api_key"`
}

// ModelSlot binds a logical agent role to a provider + model ID.
type ModelSlot struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

// ModelRouting maps logical slots for Eco / Balanced / Ultra optimization.
type ModelRouting struct {
	Primary   ModelSlot `json:"primary"`
	Utility   ModelSlot `json:"utility"`
	Architect ModelSlot `json:"architect"`
}

// UIPreferences stores non-secret Studio appearance and ergonomics settings.
type UIPreferences struct {
	Theme          string       `json:"theme"` // paper | sepia | warm-dark
	Language       string       `json:"language"` // th | en
	FontFamily     string       `json:"font_family"` // sarabun | ibm-plex | noto-serif | literata
	FontSize       int          `json:"font_size"`
	LineHeight     float64      `json:"line_height"`
	ReducedMotion  bool         `json:"reduced_motion"`
	DefaultMode    AgentMode    `json:"default_mode"`
	ApprovalMode   ApprovalMode `json:"approval_mode"`
	UltraBudgetCap int          `json:"ultra_budget_cap"`
}

// CloudSyncConfig holds non-secret sync configuration for Google Drive, WebDAV, and Synced Folders.
type CloudSyncConfig struct {
	ActiveProvider string `json:"active_provider"` // github | gdrive | webdav | folder
	GoogleDrive    struct {
		ClientID     string `json:"client_id"`
		RootFolder   string `json:"root_folder"` // default "Inkweave"
		AccountEmail string `json:"account_email"`
		Connected    bool   `json:"connected"`
	} `json:"google_drive"`
	WebDAV struct {
		BaseURL  string `json:"base_url"`
		Username string `json:"username"`
		RootPath string `json:"root_path"` // default "/Inkweave"
		Enabled  bool   `json:"enabled"`
		HasPass  bool   `json:"has_password"`
	} `json:"webdav"`
	LocalFolder struct {
		Path    string `json:"path"`
		Enabled bool   `json:"enabled"`
	} `json:"local_folder"`
}

// AppConfig represents global app settings persisted outside novel repositories.
type AppConfig struct {
	Providers []ProviderConfig `json:"providers"`
	Routing   ModelRouting     `json:"routing"`
	UI        UIPreferences    `json:"ui"`
	CloudSync CloudSyncConfig  `json:"cloud_sync"`
	GitAuthor struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"git_author"`
}

// Snapshot stores a restorable backup of a chapter prior to AI or major edits.
type Snapshot struct {
	ID        string    `json:"id"`
	ChapterID string    `json:"chapter_id"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
	WordCount int       `json:"word_count"`
	Preview   string    `json:"preview"`
	Content   string    `json:"content,omitempty"`
}

// DiffHunk represents a structured line diff chunk for UI Accept/Reject review.
type DiffHunk struct {
	Type string `json:"type"` // equal | delete | insert
	Text string `json:"text"`
}

// Proposal represents a pending AI write mutation awaiting author approval.
type Proposal struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	SessionID   string     `json:"session_id"`
	ToolName    string     `json:"tool_name"`
	TargetKind  string     `json:"target_kind"` // chapter | character | lore | outline | state | timeline | thread | relationship
	TargetID    string     `json:"target_id"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	BeforeText  string     `json:"before_text"`
	AfterText   string     `json:"after_text"`
	PayloadJSON string     `json:"payload_json"`
	Hunks       []DiffHunk `json:"hunks"`
	Status      string     `json:"status"` // pending | applied | rejected
	CreatedAt   time.Time  `json:"created_at"`
}

// ContextSection represents one slice of assembled memory shown in Context Inspector.
type ContextSection struct {
	ID       string `json:"id"`
	Tier     int    `json:"tier"` // 0..3
	Title    string `json:"title"`
	Tokens   int    `json:"tokens"`
	Included bool   `json:"included"`
	Reason   string `json:"reason,omitempty"`
	Preview  string `json:"preview"`
}

// ContextBreakdown gives the user full visibility into what was sent to the LLM.
type ContextBreakdown struct {
	Mode           AgentMode        `json:"mode"`
	SkillID        string           `json:"skill_id"`
	TotalTokens    int              `json:"total_tokens"`
	BudgetLimit    int              `json:"budget_limit"`
	Tier0Tokens    int              `json:"tier0_tokens"`
	Tier1Tokens    int              `json:"tier1_tokens"`
	Tier2Tokens    int              `json:"tier2_tokens"`
	Tier3Tokens    int              `json:"tier3_tokens"`
	CacheFriendly  bool             `json:"cache_friendly"`
	Sections       []ContextSection `json:"sections"`
	TruncatedItems []string         `json:"truncated_items"`
}

// TokenLedgerEntry logs actual token usage and prompt cache hits per stage.
type TokenLedgerEntry struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	Mode         AgentMode `json:"mode"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Stage        string    `json:"stage"`
	InputTokens  int       `json:"input_tokens"`
	CachedTokens int       `json:"cached_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CreatedAt    time.Time `json:"created_at"`
}

// ChatMessage records conversation history in SQLite.
type ChatMessage struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	ChapterID  string    `json:"chapter_id"`
	Role       string    `json:"role"` // user | assistant
	Content    string    `json:"content"`
	ToolEvents string    `json:"tool_events_json,omitempty"`
	Mode       AgentMode `json:"mode"`
	CreatedAt  time.Time `json:"created_at"`
}

// SearchHit represents a single search match across manuscript, bible, or continuity.
type SearchHit struct {
	DocID   string  `json:"doc_id"`
	Kind    string  `json:"kind"` // chapter | character | location | faction | item | rule | timeline | thread
	RefID   string  `json:"ref_id"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

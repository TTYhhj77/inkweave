package agent

import "github.com/inkweave/inkweave/internal/model"

// ModeSpec documents the token budgets and stage rigor for each AgentMode.
type ModeSpec struct {
	Mode            model.AgentMode `json:"mode"`
	BudgetCap       int             `json:"budget_cap"`
	MaxToolSteps    int             `json:"max_tool_steps"`
	EnableCache     bool            `json:"enable_cache"`
	EnableCritic    bool            `json:"enable_critic"`
	EnableArchitect bool            `json:"enable_architect"`
	PrimaryRole     string          `json:"primary_role"`
}

func SpecForMode(mode model.AgentMode, ultraCap int) ModeSpec {
	switch mode {
	case model.ModeEco:
		return ModeSpec{
			Mode:            model.ModeEco,
			BudgetCap:       6000,
			MaxToolSteps:    1,
			EnableCache:     false,
			EnableCritic:    false,
			EnableArchitect: false,
			PrimaryRole:     "utility",
		}
	case model.ModeUltra:
		capVal := 48000
		if ultraCap >= 16000 {
			capVal = ultraCap
		}
		return ModeSpec{
			Mode:            model.ModeUltra,
			BudgetCap:       capVal,
			MaxToolSteps:    6,
			EnableCache:     true,
			EnableCritic:    true,
			EnableArchitect: true,
			PrimaryRole:     "primary",
		}
	default:
		return ModeSpec{
			Mode:            model.ModeBalanced,
			BudgetCap:       18000,
			MaxToolSteps:    4,
			EnableCache:     true,
			EnableCritic:    false,
			EnableArchitect: false,
			PrimaryRole:     "primary",
		}
	}
}

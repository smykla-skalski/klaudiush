// Package suggest generates a KLAUDIUSH.md file documenting active validation rules
// for Claude Code. This gives Claude upfront knowledge of conventions, avoiding
// trial-and-error validation failures.
package suggest

import (
	"sort"

	"github.com/smykla-skalski/klaudiush/internal/patterns"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// Default values matching the validators.
const (
	defaultTitleMaxLength    = 50
	defaultBodyMaxLineLength = 72
	defaultBodyLineTolerance = 5
	defaultCommitStyle       = "conventional"
)

// Default valid commit types from commitlint config-conventional.
var defaultValidTypes = []string{
	"build", "chore", "ci", "docs", "feat", "fix",
	"perf", "refactor", "revert", "style", "test",
}

// Default branch types.
var defaultValidBranchTypes = []string{
	"feat", "fix", "docs", "style", "refactor",
	"test", "chore", "ci", "build", "perf",
}

// Default protected branches.
var defaultProtectedBranches = []string{"main", "master"}

// SuggestData is the top-level data struct consumed by the template.
type SuggestData struct {
	Version   string
	Hash      string
	Commit    *CommitRulesData
	Push      *PushRulesData
	Branch    *BranchRulesData
	PR        *PRRulesData
	Linters   []FileLinterData
	Secrets   *SecretsRulesData
	Shell     *ShellRulesData
	Rules    []CustomRuleData
	Cascades []CascadeData
}

// CommitRulesData holds git commit validation rules.
type CommitRulesData struct {
	RequiredFlags      []string
	CheckStagingArea   bool
	TitleMaxLength     int
	BodyMaxLineLength  int
	BodyLineTolerance  int
	ConventionalCommit bool
	CommitStyle        string
	RequireScope       bool
	ValidTypes         []string
	ForbiddenPatterns  []string
	BlockInfraScope    bool
	BlockPRReferences  bool
	BlockAIAttribution bool
}

// PushRulesData holds git push validation rules.
type PushRulesData struct {
	BlockedRemotes        []string
	AllowedRemotePriority []string
	RequireTracking       bool
}

// BranchRulesData holds branch naming rules.
type BranchRulesData struct {
	RequireType       bool
	ValidTypes        []string
	ProtectedBranches []string
	AllowUppercase    bool
}

// PRRulesData holds PR validation rules.
type PRRulesData struct {
	TitleMaxLength int
	RequireBody    bool
}

// FileLinterData describes a single file linter.
type FileLinterData struct {
	Name     string
	FileType string
	Tool     string
}

// SecretsRulesData holds secrets detection configuration.
type SecretsRulesData struct {
	UseGitleaks      bool
	BlockOnDetection bool
	AllowListCount   int
}

// ShellRulesData holds shell validation configuration.
type ShellRulesData struct {
	CheckAllCommands bool
}

// CustomRuleData describes a user-defined rule.
type CustomRuleData struct {
	Name      string
	Validator string
	Action    string
	Priority  int
}

// CascadeData describes a known failure cascade.
type CascadeData struct {
	SourceCode string
	TargetCode string
	SourceDesc string
	TargetDesc string
}

// Collect extracts all relevant info from config into a SuggestData struct.
func Collect(cfg *config.Config, ver string) *SuggestData {
	if cfg == nil {
		cfg = &config.Config{}
	}

	data := &SuggestData{
		Version: ver,
	}

	validators := cfg.GetValidators()

	data.Commit = collectCommitRules(validators.GetGit())
	data.Push = collectPushRules(validators.GetGit())
	data.Branch = collectBranchRules(validators.GetGit())
	data.PR = collectPRRules(validators.GetGit())
	data.Linters = collectLinters(validators.GetFile())
	data.Secrets = collectSecrets(validators.GetSecrets())
	data.Shell = collectShell(validators)
	data.Rules = collectCustomRules(cfg.GetRules())
	data.Cascades = collectCascades()

	return data
}

func collectCommitRules(git *config.GitConfig) *CommitRulesData {
	commit := git.Commit

	data := &CommitRulesData{
		RequiredFlags:      []string{"-s", "-S"},
		CheckStagingArea:   true,
		TitleMaxLength:     defaultTitleMaxLength,
		BodyMaxLineLength:  defaultBodyMaxLineLength,
		BodyLineTolerance:  defaultBodyLineTolerance,
		ConventionalCommit: true,
		CommitStyle:        defaultCommitStyle,
		RequireScope:       true,
		ValidTypes:         defaultValidTypes,
		BlockInfraScope:    true,
		BlockPRReferences:  true,
		BlockAIAttribution: true,
	}

	if commit == nil {
		return data
	}

	if len(commit.RequiredFlags) > 0 {
		data.RequiredFlags = commit.RequiredFlags
	}

	if commit.CheckStagingArea != nil {
		data.CheckStagingArea = *commit.CheckStagingArea
	}

	msg := commit.Message
	if msg == nil {
		return data
	}

	if msg.TitleMaxLength != nil {
		data.TitleMaxLength = *msg.TitleMaxLength
	}

	if msg.BodyMaxLineLength != nil {
		data.BodyMaxLineLength = *msg.BodyMaxLineLength
	}

	if msg.BodyLineTolerance != nil {
		data.BodyLineTolerance = *msg.BodyLineTolerance
	}

	if msg.ConventionalCommits != nil {
		data.ConventionalCommit = *msg.ConventionalCommits
	}

	if msg.CommitStyle != "" {
		data.CommitStyle = msg.CommitStyle
	}

	if msg.RequireScope != nil {
		data.RequireScope = *msg.RequireScope
	}

	if len(msg.ValidTypes) > 0 {
		data.ValidTypes = msg.ValidTypes
	}

	if len(msg.ForbiddenPatterns) > 0 {
		data.ForbiddenPatterns = msg.ForbiddenPatterns
	}

	if msg.BlockInfraScopeMisuse != nil {
		data.BlockInfraScope = *msg.BlockInfraScopeMisuse
	}

	if msg.BlockPRReferences != nil {
		data.BlockPRReferences = *msg.BlockPRReferences
	}

	if msg.BlockAIAttribution != nil {
		data.BlockAIAttribution = *msg.BlockAIAttribution
	}

	return data
}

func collectPushRules(git *config.GitConfig) *PushRulesData {
	data := &PushRulesData{
		AllowedRemotePriority: []string{"origin", "upstream"},
		RequireTracking:       true,
	}

	push := git.Push
	if push == nil {
		return data
	}

	if len(push.BlockedRemotes) > 0 {
		data.BlockedRemotes = push.BlockedRemotes
	}

	if len(push.AllowedRemotePriority) > 0 {
		data.AllowedRemotePriority = push.AllowedRemotePriority
	}

	if push.RequireTracking != nil {
		data.RequireTracking = *push.RequireTracking
	}

	return data
}

func collectBranchRules(git *config.GitConfig) *BranchRulesData {
	data := &BranchRulesData{
		RequireType:       true,
		ValidTypes:        defaultValidBranchTypes,
		ProtectedBranches: defaultProtectedBranches,
	}

	branch := git.Branch
	if branch == nil {
		return data
	}

	if branch.RequireType != nil {
		data.RequireType = *branch.RequireType
	}

	if len(branch.ValidTypes) > 0 {
		data.ValidTypes = branch.ValidTypes
	}

	if len(branch.ProtectedBranches) > 0 {
		data.ProtectedBranches = branch.ProtectedBranches
	}

	if branch.AllowUppercase != nil {
		data.AllowUppercase = *branch.AllowUppercase
	}

	return data
}

func collectPRRules(git *config.GitConfig) *PRRulesData {
	data := &PRRulesData{
		TitleMaxLength: defaultTitleMaxLength,
		RequireBody:    true,
	}

	pr := git.PR
	if pr == nil {
		return data
	}

	if pr.TitleMaxLength != nil {
		data.TitleMaxLength = *pr.TitleMaxLength
	}

	if pr.RequireBody != nil {
		data.RequireBody = *pr.RequireBody
	}

	return data
}

func collectLinters(file *config.FileConfig) []FileLinterData {
	type linterEntry struct {
		data    FileLinterData
		enabled bool
	}

	entries := []linterEntry{
		{FileLinterData{"Markdown", "*.md", "markdownlint + custom rules"}, isLinterEnabled(file, "markdown")},
		{FileLinterData{"ShellScript", "*.sh, *.bash", "shellcheck"}, isLinterEnabled(file, "shellscript")},
		{FileLinterData{"Terraform", "*.tf", "tofu/terraform fmt + tflint"}, isLinterEnabled(file, "terraform")},
		{FileLinterData{"Workflow", ".github/workflows/*.yml", "actionlint"}, isLinterEnabled(file, "workflow")},
		{FileLinterData{"Go", "*.go", "gofumpt"}, isLinterEnabled(file, "gofumpt")},
		{FileLinterData{"Python", "*.py", "ruff"}, isLinterEnabled(file, "python")},
		{FileLinterData{"JavaScript", "*.js, *.ts, *.jsx, *.tsx", "oxlint"}, isLinterEnabled(file, "javascript")},
		{FileLinterData{"Rust", "*.rs", "rustfmt"}, isLinterEnabled(file, "rust")},
		{FileLinterData{"LinterIgnore", "all", "pattern detection"}, isLinterEnabled(file, "linterignore")},
	}

	var result []FileLinterData

	for _, e := range entries {
		if e.enabled {
			result = append(result, e.data)
		}
	}

	return result
}

// isLinterEnabled checks if a specific file linter is enabled.
// Returns true if config is nil (default: enabled).
func isLinterEnabled(file *config.FileConfig, name string) bool {
	if file == nil {
		return true
	}

	switch name {
	case "markdown":
		return file.Markdown == nil || file.Markdown.IsEnabled()
	case "shellscript":
		return file.ShellScript == nil || file.ShellScript.IsEnabled()
	case "terraform":
		return file.Terraform == nil || file.Terraform.IsEnabled()
	case "workflow":
		return file.Workflow == nil || file.Workflow.IsEnabled()
	case "gofumpt":
		return file.Gofumpt == nil || file.Gofumpt.IsEnabled()
	case "python":
		return file.Python == nil || file.Python.IsEnabled()
	case "javascript":
		return file.JavaScript == nil || file.JavaScript.IsEnabled()
	case "rust":
		return file.Rust == nil || file.Rust.IsEnabled()
	case "linterignore":
		return file.LinterIgnore == nil || file.LinterIgnore.IsEnabled()
	default:
		return true
	}
}

func collectSecrets(secrets *config.SecretsConfig) *SecretsRulesData {
	data := &SecretsRulesData{
		BlockOnDetection: true,
	}

	if secrets == nil || secrets.Secrets == nil {
		return data
	}

	s := secrets.Secrets
	data.UseGitleaks = s.IsUseGitleaksEnabled()
	data.BlockOnDetection = s.IsBlockOnDetectionEnabled()
	data.AllowListCount = len(s.AllowList)

	return data
}

func collectShell(validators *config.ValidatorsConfig) *ShellRulesData {
	data := &ShellRulesData{}

	if validators.Shell == nil || validators.Shell.Backtick == nil {
		return data
	}

	data.CheckAllCommands = validators.Shell.Backtick.CheckAllCommands

	return data
}

func collectCustomRules(rules *config.RulesConfig) []CustomRuleData {
	if rules == nil || len(rules.Rules) == 0 {
		return nil
	}

	var result []CustomRuleData

	for _, rule := range rules.Rules {
		if !rule.IsRuleEnabled() {
			continue
		}

		validatorType := ""
		if rule.Match != nil {
			validatorType = rule.Match.ValidatorType
		}

		action := "block"
		if rule.Action != nil {
			action = rule.Action.GetActionType()
		}

		result = append(result, CustomRuleData{
			Name:      rule.Name,
			Validator: validatorType,
			Action:    action,
			Priority:  rule.Priority,
		})
	}

	return result
}

func collectCascades() []CascadeData {
	descs := patterns.CodeDescriptions()
	seedData := patterns.SeedPatterns()

	var cascades []CascadeData

	for _, p := range seedData.Patterns {
		cascades = append(cascades, CascadeData{
			SourceCode: p.SourceCode,
			TargetCode: p.TargetCode,
			SourceDesc: descs[p.SourceCode],
			TargetDesc: descs[p.TargetCode],
		})
	}

	// Sort for deterministic output
	sort.Slice(cascades, func(i, j int) bool {
		if cascades[i].SourceCode != cascades[j].SourceCode {
			return cascades[i].SourceCode < cascades[j].SourceCode
		}

		return cascades[i].TargetCode < cascades[j].TargetCode
	})

	return cascades
}



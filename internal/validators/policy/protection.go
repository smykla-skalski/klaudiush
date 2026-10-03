package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/protection"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// ProtectionValidatorName is the runtime name of the protection validator.
const ProtectionValidatorName = "protection"

const (
	protectedRepair = "Do not change this file. If the change is needed, ask the user to make " +
		"it, or to list the path in protection.allow"
	opaqueRepair = "Split the command into plain commands klaudiush can inspect"
	policyRepair = "Ask the user to run this klaudiush command themselves"
)

// ProtectionValidator blocks tool calls and shell commands that would
// change a file enforcing policy, klaudiush commands that change policy,
// and (Claude ConfigChange) settings changes made mid-session.
type ProtectionValidator struct {
	*validator.BaseValidator
	cfg     *config.ProtectionConfig
	locator Locator
}

// NewProtectionValidator creates a ProtectionValidator.
func NewProtectionValidator(
	log logger.Logger,
	cfg *config.ProtectionConfig,
	locator Locator,
) *ProtectionValidator {
	return &ProtectionValidator{
		BaseValidator: validator.NewBaseValidator(ProtectionValidatorName, log),
		cfg:           cfg,
		locator:       locator,
	}
}

// Category returns the validator category.
func (*ProtectionValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}

// Validate checks the hook against the protected set.
func (v *ProtectionValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	if hookCtx.Event == hook.CanonicalEventConfigChange {
		return v.validateConfigChange(hookCtx)
	}

	opts := v.locator(hookCtx)
	if dir := shellDir(hookCtx, opts.WorkDir); dir != "" {
		opts.WorkDir = dir
	}

	set, err := protection.NewSet(opts)
	if err != nil {
		return validator.FailWithRef(
			validator.RefProtectedFile,
			"Protection configuration is invalid, so policy files cannot be told apart: "+
				err.Error(),
		)
	}

	switch {
	case hookCtx.IsAfterTool():
		return v.validateChanged(set, hookCtx)
	case hookCtx.IsBashTool():
		return v.validateCommand(ctx, set, hookCtx)
	default:
		return v.validateTool(set, hookCtx)
	}
}

// validateConfigChange keeps a changed settings file from taking effect
// mid-session. Claude cannot block policy_settings, so those pass.
func (v *ProtectionValidator) validateConfigChange(hookCtx *hook.Context) *validator.Result {
	change := hookCtx.ConfigChange
	if change == nil || change.Source == config.ConfigSourcePolicySettings ||
		!v.cfg.BlocksConfigChange(change.Source) {
		return validator.Pass()
	}

	if change.FilePath != "" {
		set, err := protection.NewSet(v.locator(hookCtx))
		if err == nil && set.Allowed(change.FilePath) {
			return validator.Pass()
		}
	}

	file := change.FilePath
	if file == "" {
		file = change.Source
	}

	return validator.FailWithRef(
		validator.RefConfigChangeBlocked,
		fmt.Sprintf("Settings change to %s (%s) was not applied while protection is enabled",
			file, change.Source),
	).AddFinding(validator.Finding{
		Reference: validator.RefConfigChangeBlocked,
		Location:  file,
		Message:   "Hook settings changed during the session",
		Required:  "protection.config_change_sources excludes " + change.Source,
		Repair:    "Restart the session to load the change",
	})
}

func (*ProtectionValidator) validateCommand(
	ctx context.Context,
	set *protection.Set,
	hookCtx *hook.Context,
) *validator.Result {
	parsed, err := hookCtx.ParsedCommand()
	if err != nil || parsed.Truncated {
		return validator.FailWithRef(
			validator.RefProtectedFile,
			"Command cannot be fully inspected, so it may change protected policy files",
		).AddFinding(validator.Finding{
			Reference: validator.RefProtectedFile,
			Location:  "command",
			Message:   "Part of the command is opaque to klaudiush",
			Repair:    opaqueRepair,
		})
	}

	violations := set.CheckCommand(ctx, parsed, hookCtx.GetCommand())
	if len(violations) == 0 {
		return validator.Pass()
	}

	return commandResult(violations)
}

func commandResult(violations []protection.Violation) *validator.Result {
	var (
		files    []validator.Finding
		commands []validator.Finding
		names    []string
	)

	for _, violation := range violations {
		if violation.Command != "" {
			commands = append(commands, validator.Finding{
				Reference: validator.RefPolicyCommand,
				Location:  "klaudiush " + violation.Command,
				Message:   "klaudiush " + violation.Command + " changes klaudiush policy",
				Repair:    policyRepair,
			})

			continue
		}

		names = append(names, violation.Path)
		files = append(files, fileFinding(violation.Match, violation.Program+" "+violation.Target))
	}

	if len(files) == 0 {
		return validator.FailWithRef(
			validator.RefPolicyCommand,
			"Command runs a klaudiush command that changes policy",
		).AddFinding(commands...)
	}

	return validator.FailWithRef(
		validator.RefProtectedFile,
		"Command would change protected policy files: "+strings.Join(names, ", "),
	).AddFinding(append(files, commands...)...)
}

func fileFinding(m protection.Match, actual string) validator.Finding {
	return validator.Finding{
		Reference: validator.RefProtectedFile,
		Location:  m.Path,
		Message:   "Protected " + m.Reason,
		Actual:    strings.TrimSpace(actual),
		Repair:    protectedRepair,
	}
}

// validateTool checks the files a write, edit, patch or other tool call
// names. Tools other than file writers may also delete or move what they
// name, so directories holding protected files count for them.
func (*ProtectionValidator) validateTool(
	set *protection.Set,
	hookCtx *hook.Context,
) *validator.Result {
	tree := hookCtx.ToolFamily == hook.ToolFamilyUnknown

	var (
		findings []validator.Finding
		names    []string
	)

	seen := make(map[string]bool)

	for _, target := range protection.ToolTargets(hookCtx) {
		path := set.Resolve(target)
		if seen[path] {
			continue
		}

		seen[path] = true

		check := set.Check
		if tree {
			check = set.CheckTree
		}

		m, ok := check(path)
		if !ok {
			continue
		}

		names = append(names, m.Path)
		findings = append(findings, fileFinding(m, hookCtx.RawToolName+" "+target))
	}

	if len(findings) == 0 {
		return validator.Pass()
	}

	return validator.FailWithRef(
		validator.RefProtectedFile,
		hookCtx.RawToolName+" would change protected policy files: "+strings.Join(names, ", "),
	).AddFinding(findings...)
}

// validateChanged reports protected files a shell command changed, as the
// provider listed them after the command ran. Pre-tool checks stop what the
// parser sees; this catches what only the result shows.
func (*ProtectionValidator) validateChanged(
	set *protection.Set,
	hookCtx *hook.Context,
) *validator.Result {
	var (
		findings []validator.Finding
		names    []string
	)

	for _, changed := range hookCtx.ChangedFiles {
		m, ok := set.Check(set.Resolve(changed))
		if !ok {
			continue
		}

		names = append(names, m.Path)
		finding := fileFinding(m, "changed by the command")
		finding.Repair = "Restore the file to its previous content and tell the user it was changed"
		findings = append(findings, finding)
	}

	if len(findings) == 0 {
		return validator.Pass()
	}

	return validator.FailWithRef(
		validator.RefProtectedFile,
		"Command changed protected policy files: "+strings.Join(names, ", "),
	).AddFinding(findings...)
}

// shellDirKeys are the shell tool arguments that set the directory a
// command runs in: Gemini run_shell_command dir_path (directory in older
// releases) and Codex exec_command workdir.
var shellDirKeys = []string{"dir_path", "directory", "workdir", "cwd"}

// shellDir returns the directory a shell tool call runs its command in when
// the call names one, resolved against workDir.
func shellDir(hookCtx *hook.Context, workDir string) string {
	if !hookCtx.IsBashTool() {
		return ""
	}

	for _, key := range shellDirKeys {
		raw, ok := hookCtx.ToolInput.Additional[key]
		if !ok {
			continue
		}

		var dir string
		if json.Unmarshal(raw, &dir) != nil || dir == "" {
			continue
		}

		if !filepath.IsAbs(dir) {
			dir = filepath.Join(workDir, dir)
		}

		return filepath.Clean(dir)
	}

	return ""
}

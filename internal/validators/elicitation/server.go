// Package elicitation provides validators for MCP elicitation events.
package elicitation

import (
	"context"
	"path"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// ServerValidator validates MCP server names against allow/deny lists.
type ServerValidator struct {
	*validator.BaseValidator
	config *config.ElicitationServerConfig
}

// NewServerValidator creates a new ServerValidator.
func NewServerValidator(
	log logger.Logger,
	cfg *config.ElicitationServerConfig,
	ruleAdapter validator.RuleChecker,
) *ServerValidator {
	return &ServerValidator{
		BaseValidator: validator.NewBaseValidatorWithRules("mcp-server", log, ruleAdapter),
		config:        cfg,
	}
}

// Validate checks the MCP server name against allow/deny lists and mode restrictions.
func (v *ServerValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	serverName := hookCtx.GetMCPServerName()
	if serverName == "" {
		return validator.Pass()
	}

	if v.config == nil {
		return validator.Pass()
	}

	if len(v.config.DeniedServers) > 0 && matchesAny(serverName, v.config.DeniedServers) {
		return validator.FailWithRef(
			validator.RefMCPServerBlocked,
			"MCP server "+serverName+" is on the deny list",
		)
	}

	if len(v.config.AllowedServers) > 0 && !matchesAny(serverName, v.config.AllowedServers) {
		return validator.FailWithRef(
			validator.RefMCPServerNotAllowed,
			"MCP server "+serverName+" is not on the allow list",
		)
	}

	if v.config.IsBlockURLMode() && hookCtx.Elicitation != nil &&
		strings.EqualFold(hookCtx.Elicitation.Mode, "url") {
		return validator.FailWithRef(
			validator.RefMCPURLModeBlocked,
			"URL mode is blocked for MCP elicitation from "+serverName,
		)
	}

	return validator.Pass()
}

// Category returns the validator category.
func (*ServerValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}

func matchesAny(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, err := path.Match(pattern, name); err == nil && matched {
			return true
		}

		if strings.EqualFold(pattern, name) {
			return true
		}
	}

	return false
}

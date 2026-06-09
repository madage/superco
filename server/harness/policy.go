package harness

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AgentContext represents the context of an agent making a tool call.
type AgentContext struct {
	AgentProfileID string
	AgentName      string
	UserID         string
	WorkflowID     *string
	TaskID         *string
	Depth          int
	MaxDepth       int
	LoopCount      int
	MaxReviewLoops int
	TokensUsed     int64
	TokenBudget    int64
	Permissions    []string          // granted permissions
	Capabilities   map[string]bool   // declared tools (capabilities.tools)
}

// PolicyEngine checks whether a tool call is allowed.
type PolicyEngine struct {
}

// NewPolicyEngine creates a new policy engine.
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{}
}

// CheckResult describes the result of a policy check.
type CheckResult struct {
	Allowed      bool
	Reason       string
	ErrorCode    string
	Message      string
	Suggestion   string
}

// Check verifies a tool call against the agent's context.
func (pe *PolicyEngine) Check(ctx *AgentContext, tc *ToolCall) *CheckResult {
	// 1. Tool existence check
	defs := AllTools()
	def, exists := defs[tc.Tool]
	if !exists {
		return deny(ErrToolNotFound, fmt.Sprintf("tool '%s' not found", tc.Tool), "")
	}

	// 2. Schema validation
	if err := ValidateParams(def, tc.Params); err != nil {
		return deny(ErrSchemaInvalid, err.Error(), "check the required parameters and try again")
	}

	// 3. Capability check: agent must have declared this tool in capabilities.tools
	if ctx.Capabilities != nil {
		if !ctx.Capabilities[tc.Tool] {
			return deny(ErrPermissionDenied,
				fmt.Sprintf("agent '%s' has not declared capability for tool '%s'", ctx.AgentName, tc.Tool),
				"add the tool to capabilities.tools in the agent profile")
		}
	}

	// 4. Permission check
	// If no permissions are configured, skip the check (capability-only mode).
	// The UI exposes capabilities, not fine-grained permissions, so empty
	// permissions should not block tool calls.
	if len(ctx.Permissions) > 0 {
		hasPerm := false
		for _, p := range ctx.Permissions {
			if p == def.RequiredPerm || p == "task.admin" {
				hasPerm = true
				break
			}
		}
		if !hasPerm {
			return deny(ErrPermissionDenied,
				fmt.Sprintf("agent '%s' lacks required permission '%s' for tool '%s'", ctx.AgentName, def.RequiredPerm, tc.Tool),
				"request the required permission from the workspace admin")
		}
	}

	// 5. Depth check (only for create_sub_task)
	if tc.Tool == ToolCreateSubTask {
		if ctx.Depth >= ctx.MaxDepth {
			return deny(ErrDepthExceeded,
				fmt.Sprintf("当前深度为 %d，已达到上限（max_depth=%d），无法继续拆分", ctx.Depth, ctx.MaxDepth),
				"合并部分子任务或让人工介入")
		}
	}

	// 6. Loop check (for review/rework scenarios)
	if tc.Tool == ToolReviewTask {
		if ctx.LoopCount >= ctx.MaxReviewLoops {
			return deny(ErrLoopExceeded,
				fmt.Sprintf("审核已打回 %d 次（上限 %d 次），已暂停自动处理", ctx.LoopCount, ctx.MaxReviewLoops),
				"人工判断：批准当前版本，或重新指派")
		}
	}

	// 7. Token budget check
	if ctx.TokenBudget > 0 && ctx.TokensUsed >= ctx.TokenBudget {
		return deny(ErrBudgetExceeded,
			fmt.Sprintf("Token 预算已用完（%d/%d）", ctx.TokensUsed, ctx.TokenBudget),
			"联系管理员追加预算")
	}

	return &CheckResult{Allowed: true}
}

func deny(code, message, suggestion string) *CheckResult {
	return &CheckResult{
		Allowed:    false,
		Reason:     code,
		ErrorCode:  code,
		Message:    message,
		Suggestion: suggestion,
	}
}

// HasCapability checks whether an agent's declared capabilities include a specific tool.
func HasCapability(capabilitiesJSON json.RawMessage, tool string) bool {
	if len(capabilitiesJSON) == 0 {
		return false
	}
	var caps struct {
		Tools []string `json:"tools"`
	}
	if err := json.Unmarshal(capabilitiesJSON, &caps); err != nil {
		return false
	}
	for _, t := range caps.Tools {
		if t == tool {
			return true
		}
	}
	return false
}

// ParsePermissions parses a JSON permissions array into a string slice.
func ParsePermissions(permJSON json.RawMessage) []string {
	if len(permJSON) == 0 {
		return nil
	}
	var perms []string
	if err := json.Unmarshal(permJSON, &perms); err != nil {
		return nil
	}
	return perms
}

// ParseCapabilitiesMap parses capabilities JSON into a tool→bool map.
func ParseCapabilitiesMap(capJSON json.RawMessage) map[string]bool {
	result := make(map[string]bool)
	if len(capJSON) == 0 {
		return result
	}
	var caps struct {
		Tools []string `json:"tools"`
	}
	if err := json.Unmarshal(capJSON, &caps); err != nil {
		return result
	}
	for _, t := range caps.Tools {
		result[strings.TrimSpace(t)] = true
	}
	return result
}

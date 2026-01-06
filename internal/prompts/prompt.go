package prompts

import (
	"embed"

	"github.com/soyokaze83/invictus/internal/utils"
)

//go:embed SYSTEM_PROMPT.md
var promptFs embed.FS

var AgentPrompt = utils.MustLoadPrompt(promptFs, "SYSTEM_PROMPT.md")

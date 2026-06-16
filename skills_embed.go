package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"hr-cli/cmd"
)

// skillsEmbedFS embeds each skill's SKILL.md so the CLI serves content matching
// the binary version. Whitelist-only — adding new resource types (references/,
// scripts/, etc.) means extending the patterns below.
//
//go:embed skills/*/SKILL.md
var skillsEmbedFS embed.FS

func init() {
	sub, err := fs.Sub(skillsEmbedFS, "skills")
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: skills embed assembly failed, skills commands disabled:", err)
		return
	}
	cmd.SetEmbeddedSkillContent(sub)
}

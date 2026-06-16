package cmd

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"hr-cli/internal/errs"
)

var embeddedSkillContent fs.FS

// SetEmbeddedSkillContent wires the binary-embedded skill tree from main.
// Called from skills_embed.go's init(). Keeping this as a setter (rather than
// importing the embed FS directly) lets `go build ./cmd/...` compile without
// the embed file, which matters for tests and tooling.
func SetEmbeddedSkillContent(content fs.FS) {
	embeddedSkillContent = content
}

func newSkillsCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "skills",
		Short: "Read embedded skill content (list / read) for AI agents",
		Long: "Read agent-readable skill content (SKILL.md files) embedded in the CLI " +
			"binary at build time, so it stays in sync with the CLI version. " +
			"AI agents should run `hr skills list` to discover available skills, " +
			"then `hr skills read <name>` to pull the SKILL.md as context.",
	}
	root.AddCommand(newSkillsListCommand(), newSkillsReadCommand())
	return root
}

type skillsReadEnvelope struct {
	Skill   string `json:"skill"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

func newSkillsListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List embedded skills (name, description, version)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if embeddedSkillContent == nil {
				return errs.New("internal", "skills_not_embedded", "skill content not embedded in this build", 5)
			}
			skills, err := listSkills()
			if err != nil {
				return err
			}
			return emit(cmd, map[string]any{"items": skills, "count": len(skills)})
		},
	}
}

func newSkillsReadCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "read <name>",
		Short: "Print a skill's SKILL.md as raw markdown (use --format json for envelope)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if embeddedSkillContent == nil {
				return errs.New("internal", "skills_not_embedded", "skill content not embedded in this build", 5)
			}
			name := strings.TrimSpace(args[0])
			if name == "" || strings.ContainsAny(name, "/\\") {
				e := errs.Validation("invalid_skill_name", "skill name must not contain path separators")
				e.Param = "name"
				return e
			}
			content, err := readSkillFile(name, "SKILL.md")
			if err != nil {
				return err
			}
			if format == "json" {
				return emit(cmd, skillsReadEnvelope{
					Skill:   name,
					Path:    "SKILL.md",
					Content: string(content),
				})
			}
			fmt.Fprint(cmd.OutOrStdout(), string(content))
			return nil
		},
	}
}

type skillInfo = map[string]any

func listSkills() ([]skillInfo, *errs.Error) {
	entries, err := fs.ReadDir(embeddedSkillContent, ".")
	if err != nil {
		return nil, errs.New("internal", "skills_read_failed", err.Error(), 5)
	}
	out := make([]skillInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info := skillInfo{"name": e.Name()}
		content, readErr := fs.ReadFile(embeddedSkillContent, path.Join(e.Name(), "SKILL.md"))
		if readErr == nil {
			desc, ver := parseSkillFrontmatter(content)
			info["description"] = desc
			info["version"] = ver
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprint(out[i]["name"]) < fmt.Sprint(out[j]["name"])
	})
	return out, nil
}

func readSkillFile(name, relpath string) ([]byte, *errs.Error) {
	full := path.Join(name, relpath)
	content, err := fs.ReadFile(embeddedSkillContent, full)
	if err != nil {
		e := errs.Validation("skill_not_found", fmt.Sprintf("skill %q not found or has no %s", name, relpath))
		e.Param = "name"
		return nil, e
	}
	return content, nil
}

// parseSkillFrontmatter extracts description and version from the YAML
// frontmatter block at the top of SKILL.md. Best-effort: returns "" if the
// frontmatter is missing or malformed; never returns an error.
func parseSkillFrontmatter(content []byte) (description, version string) {
	text := string(content)
	if !strings.HasPrefix(text, "---") {
		return "", ""
	}
	rest := text[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", ""
	}
	for _, line := range strings.Split(rest[:end], "\n") {
		line = strings.TrimSpace(line)
		if v, ok := stripPrefix(line, "description:"); ok {
			description = strings.Trim(strings.TrimSpace(v), `"'`)
		}
		if v, ok := stripPrefix(line, "version:"); ok {
			version = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return description, version
}

func stripPrefix(s, prefix string) (string, bool) {
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):], true
	}
	return "", false
}

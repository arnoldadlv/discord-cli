package cli_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

func TestHelpSkillPrintsEmbeddedSkill(t *testing.T) {
	r := clitest.NewRunner(t)
	res := r.Run("help", "--skill")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.HasPrefix(res.Stdout, "---\nname: discord-cli\ndescription:") {
		t.Errorf("no frontmatter: %q", res.Stdout[:min(80, len(res.Stdout))])
	}
	for _, want := range []string{"--json", "auth status", "exit code", "default-guild", "dm export", "export search", "~/.config/discord-cli", "~/.discord-cli/exports", "DiscordChatExporter"} {
		if !strings.Contains(strings.ToLower(res.Stdout), strings.ToLower(want)) {
			t.Errorf("skill missing %q", want)
		}
	}
	for _, real := range []string{"Cooey", "cmmc", "kyle"} {
		if strings.Contains(strings.ToLower(res.Stdout), strings.ToLower(real)) {
			t.Errorf("skill names something real: %q", real)
		}
	}
	if res.Stderr != "" {
		t.Errorf("stderr: %q", res.Stderr)
	}
	// Plain help still works, and --skill takes no command.
	if res := r.Run("help"); res.ExitCode != 0 || !strings.Contains(res.Stdout, "Usage:") {
		t.Errorf("help: %d %q", res.ExitCode, res.Stdout)
	}
	if res := r.Run("help", "guild", "list"); res.ExitCode != 0 || !strings.Contains(res.Stdout, "discord guild list") {
		t.Errorf("help guild list: %d %q", res.ExitCode, res.Stdout)
	}
	if res := r.Run("help", "--skill", "guild"); res.ExitCode != 2 {
		t.Errorf("--skill with a command should be a usage error, got %d", res.ExitCode)
	}
}

var skillCommand = regexp.MustCompile(`(?m)^discord ([a-z]+) ([a-z]+)(.*)$`)
var skillFlag = regexp.MustCompile(`--[a-z-]+`)

func TestEmbeddedSkillCommandsExist(t *testing.T) {
	r := clitest.NewRunner(t)
	skill := r.Run("help", "--skill").Stdout
	matches := skillCommand.FindAllStringSubmatch(skill, -1)
	if len(matches) < 15 {
		t.Fatalf("only %d command lines found in the skill", len(matches))
	}
	for _, m := range matches {
		noun, verb, rest := m[1], m[2], m[3]
		res := r.Run(noun, verb, "--help")
		if res.ExitCode != 0 {
			t.Errorf("discord %s %s does not exist: %s", noun, verb, res.Stderr)
			continue
		}
		for _, f := range skillFlag.FindAllString(rest, -1) {
			if !strings.Contains(res.Stdout, f) {
				t.Errorf("discord %s %s: flag %s not in its help", noun, verb, f)
			}
		}
	}
}

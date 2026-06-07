package cli

import (
	"fmt"

	"github.com/Dieg0Code/arc/internal/skill"
	"github.com/spf13/cobra"
)

// newSkillCmd agrupa los subcomandos del agent skill de arc.
func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage the arc agent skill (teaches your agent to use arc)",
	}
	cmd.AddCommand(newSkillInstallCmd())
	return cmd
}

// newSkillInstallCmd crea `arc skill install`: (re)instala el SKILL.md en los
// agentes presentes. Idempotente.
func newSkillInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install the arc agent skill into Claude Code and/or Codex",
		RunE: func(cmd *cobra.Command, args []string) error {
			return installSkill(cmd)
		},
	}
}

// installSkill instala el skill y reporta el resultado. Lo comparten
// `arc init` y `arc skill install`.
func installSkill(cmd *cobra.Command) error {
	inst, err := skill.New()
	if err != nil {
		return err
	}
	report, err := inst.Install()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(report.Installed) == 0 {
		fmt.Fprintln(out, "no Claude Code or Codex detected; agent skill not installed")
		return nil
	}
	for _, in := range report.Installed {
		fmt.Fprintf(out, "agent skill installed for %s: %s\n", in.Agent, in.Path)
	}
	for _, s := range report.Skipped {
		fmt.Fprintf(out, "  (%s not detected, skipped)\n", s)
	}
	return nil
}

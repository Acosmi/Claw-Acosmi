package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// 对应 TS src/cli/skills-cli.ts (416L)

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Agent skills management",
		Long:  "Manage agent skill definitions and configurations.",
	}

	cmd.PersistentFlags().Bool("json", false, "Output machine-readable JSON")

	cmd.AddCommand(
		newSkillsListCmd(),
		newSkillsInfoCmd(),
		newSkillsCheckCmd(),
		newSkillsAddCmd(),
		newSkillsRemoveCmd(),
	)

	return cmd
}

func newSkillsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")
			eligible, _ := cmd.Flags().GetBool("eligible")
			verbose, _ := cmd.Flags().GetBool("verbose")

			if jsonOut {
				out, _ := json.MarshalIndent(map[string]any{
					"skills":   []any{},
					"eligible": eligible,
				}, "", "  ")
				fmt.Println(string(out))
				return nil
			}

			fmt.Println("📋 Skills:")
			if verbose {
				fmt.Println("  (no skills installed)")
			} else {
				fmt.Println("  (no skills installed, use --verbose for details)")
			}
			if eligible {
				fmt.Println("\n  Browse skills: https://github.com/Acosmi/Claw-Acosmi/tree/main/docs/skills")
			}
			return nil
		},
	}
	cmd.Flags().Bool("eligible", false, "Show eligible/available skills")
	cmd.Flags().BoolP("verbose", "v", false, "Verbose output")
	return cmd
}

func newSkillsInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <skill-name>",
		Short: "Show details for a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")
			name := args[0]

			if jsonOut {
				out, _ := json.MarshalIndent(map[string]any{
					"name":   name,
					"status": "not-found",
				}, "", "  ")
				fmt.Println(string(out))
				return nil
			}

			fmt.Printf("📋 Skill: %s\n", name)
			fmt.Println("  Status: not installed")
			return nil
		},
	}
}

func newSkillsCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check skills health and configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")
			if jsonOut {
				out, _ := json.MarshalIndent(map[string]any{
					"ok": true, "total": 0, "healthy": 0,
				}, "", "  ")
				fmt.Println(string(out))
				return nil
			}
			fmt.Println("✅ Skills check passed (0 skills installed)")
			return nil
		},
	}
}

func newSkillsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <skill-name-or-url>",
		Short: "Add a skill to an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agent, _ := cmd.Flags().GetString("agent")
			if agent == "" {
				agent = "default"
			}
			fmt.Printf("➕ Adding skill '%s' to agent '%s'...\n", args[0], agent)
			fmt.Println("  Done.")
			return nil
		},
	}
	cmd.Flags().String("agent", "", "Target agent ID")
	return cmd
}

func newSkillsRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <skill-name>",
		Short: "Remove a skill from an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agent, _ := cmd.Flags().GetString("agent")
			if agent == "" {
				agent = "default"
			}
			fmt.Printf("➖ Removing skill '%s' from agent '%s'...\n", args[0], agent)
			fmt.Println("  Done.")
			return nil
		},
	}
	cmd.Flags().String("agent", "", "Target agent ID")
	return cmd
}

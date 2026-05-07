// Package cli wires every subcommand of `sqlitedeploy`.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Defaults referenced by multiple subcommands.
const (
	flagProjectDir = "project-dir"
)

// projectDir reads --project-dir or falls back to the current working directory.
func projectDir(cmd *cobra.Command) (string, error) {
	dir, err := cmd.Flags().GetString(flagProjectDir)
	if err != nil {
		return "", err
	}
	if dir == "" {
		dir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// addProjectDirFlag attaches --project-dir / -C to a cobra command.
func addProjectDirFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP(flagProjectDir, "C", "", "project directory (default: current working directory)")
}

// envOrDefault returns the env var value, or fallback if unset/empty.
func envOrDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// promptString prompts the user on stderr and reads one line from stdin.
// Returns fallback if the user just presses enter.
func promptString(label, fallback string) (string, error) {
	if fallback != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, fallback)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return fallback, nil
	}
	return line, nil
}

// promptSecret prompts on stderr and reads a line from stdin without echo
// hiding (we keep this simple — users running interactively can paste, and CI
// flows should pass values via env vars / flags rather than the prompt).
func promptSecret(label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gdaddon/internal/addon"

	"github.com/spf13/cobra"
)

var (
	reposDir   string
	reposRaw   bool
	reposDirty bool
	reposDepth int
)

var reposCmd = &cobra.Command{
	Use:   "repos [flags] -- <command...>",
	Short: "Run a shell command in every git repo nested under a directory",
	Long: `Walk a directory tree, find every nested git repo (the top-level repo is
excluded), and run a shell command inside each one.

The command is joined and run via "sh -c", so pipes, &&, and redirects work — quote
them as a single argument so your own shell doesn't consume them first:

  gdaddon repos -- git status -s
  gdaddon repos --dirty -- git fetch
  gdaddon repos -- "git log --oneline | head -1"

By default output is captured and a header is printed only for repos that produced
output; use --raw to live-stream output instead.`,
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: false,
	RunE:          runRepos,
}

func init() {
	// Stop flag parsing at the first non-flag token so the command's own -flags are
	// collected as args; "--" remains supported but optional.
	reposCmd.Flags().SetInterspersed(false)
	reposCmd.Flags().StringVarP(&reposDir, "dir", "C", "", "directory to scan (default: current directory)")
	reposCmd.Flags().BoolVar(&reposRaw, "raw", false, "live-stream each repo's output instead of capturing it")
	reposCmd.Flags().BoolVar(&reposDirty, "dirty", false, "only repos with uncommitted changes")
	reposCmd.Flags().IntVar(&reposDepth, "depth", 5, "max directory depth to search")
	rootCmd.AddCommand(reposCmd)
}

func runRepos(cmd *cobra.Command, args []string) error {
	base := reposDir
	if base == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not get current working directory: %w", err)
		}
		base = cwd
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return err
	}

	repos, err := addon.FindGitRepos(base, reposDepth)
	if err != nil {
		return err
	}
	if reposDirty {
		dirty := repos[:0:0]
		for _, rel := range repos {
			if addon.HasUncommittedChanges(filepath.Join(base, rel)) {
				dirty = append(dirty, rel)
			}
		}
		repos = dirty
	}

	// No command: list mode — print the matching repo paths, one per line.
	if len(args) == 0 {
		for _, rel := range repos {
			fmt.Println(rel)
		}
		return nil
	}

	cmdStr := strings.Join(args, " ")
	prefix := filepath.Base(base)

	for _, rel := range repos {
		full := filepath.Join(base, rel)
		display := filepath.Join(prefix, rel)
		c := exec.Command("sh", "-c", cmdStr)
		c.Dir = full

		if reposRaw {
			reposHeader(display)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "error in %s: %v\n", display, err)
			}
			continue
		}

		var stdout, stderr bytes.Buffer
		c.Stdout = &stdout
		c.Stderr = &stderr
		runErr := c.Run()

		out := strings.TrimSpace(stdout.String())
		errOut := strings.TrimSpace(stderr.String())
		if out != "" || errOut != "" {
			reposHeader(display)
			if out != "" {
				fmt.Println(out)
			}
			if errOut != "" {
				fmt.Fprintln(os.Stderr, errOut)
			}
		}
		if runErr != nil {
			fmt.Fprintf(os.Stderr, "error in %s: %v\n", display, runErr)
		}
	}
	return nil
}

func reposHeader(text string) {
	line := strings.Repeat("-", 50)
	fmt.Printf("\n%s\n📁 %s\n%s\n", line, text, line)
}

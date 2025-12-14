package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var composeFile string

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rootCmd := newRootCommand()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "vaultdrop: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vaultdrop",
		Short: "VaultDrop development CLI",
		Long: `VaultDrop CLI orchestrates common development workflows such as building the Docker stack,
starting or stopping services, running tests, and launching the binaries directly.`,
		SilenceUsage: true,
	}
	cmd.PersistentFlags().StringVarP(&composeFile, "compose-file", "f", "docker-compose.yml", "Compose file to use for stack commands")
	cmd.AddCommand(
		newBuildCmd(),
		newUpCmd(),
		newDownCmd(),
		newLogsCmd(),
		newTestCmd(),
		newRunCmd(),
	)
	return cmd
}

func newBuildCmd() *cobra.Command {
	var noCache bool
	cmd := &cobra.Command{
		Use:   "build [service...]",
		Short: "Build Docker images via docker compose",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			composeArgs := []string{"compose", "-f", composeFile, "build"}
			if noCache {
				composeArgs = append(composeArgs, "--no-cache")
			}
			composeArgs = append(composeArgs, args...)
			return runCommand(ctx, "docker", composeArgs...)
		},
	}
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Disable Docker build cache")
	return cmd
}

func newUpCmd() *cobra.Command {
	var detach bool
	var skipBuild bool
	cmd := &cobra.Command{
		Use:   "up [service...]",
		Short: "Start the full docker-compose stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			composeArgs := []string{"compose", "-f", composeFile, "up"}
			if !skipBuild {
				composeArgs = append(composeArgs, "--build")
			}
			if detach {
				composeArgs = append(composeArgs, "-d")
			}
			composeArgs = append(composeArgs, args...)
			return runCommand(ctx, "docker", composeArgs...)
		},
	}
	cmd.Flags().BoolVarP(&detach, "detached", "d", true, "Run docker compose in detached mode")
	cmd.Flags().BoolVar(&skipBuild, "skip-build", false, "Skip rebuilding images before starting")
	return cmd
}

func newDownCmd() *cobra.Command {
	var removeVolumes bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop docker-compose stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			composeArgs := []string{"compose", "-f", composeFile, "down"}
			if removeVolumes {
				composeArgs = append(composeArgs, "-v")
			}
			return runCommand(ctx, "docker", composeArgs...)
		},
	}
	cmd.Flags().BoolVarP(&removeVolumes, "volumes", "v", false, "Remove stack volumes")
	return cmd
}

func newLogsCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs [service...]",
		Short: "Tail logs from docker-compose services",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			composeArgs := []string{"compose", "-f", composeFile, "logs"}
			if follow {
				composeArgs = append(composeArgs, "-f")
			}
			composeArgs = append(composeArgs, args...)
			return runCommand(ctx, "docker", composeArgs...)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream logs continuously")
	return cmd
}

func newTestCmd() *cobra.Command {
	var race bool
	var cover bool
	cmd := &cobra.Command{
		Use:   "test [packages]",
		Short: "Run Go tests (defaults to ./...)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			pkgs := args
			if len(pkgs) == 0 {
				pkgs = []string{"./..."}
			}
			goArgs := []string{"test"}
			if race {
				goArgs = append(goArgs, "-race")
			}
			if cover {
				goArgs = append(goArgs, "-cover")
			}
			goArgs = append(goArgs, pkgs...)
			return runCommand(ctx, "go", goArgs...)
		},
	}
	cmd.Flags().BoolVar(&race, "race", false, "Enable Go race detector")
	cmd.Flags().BoolVar(&cover, "cover", false, "Collect coverage data")
	return cmd
}

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run individual Go binaries directly",
	}
	cmd.AddCommand(
		newServiceRunner("api", "./cmd/api"),
		newServiceRunner("worker", "./cmd/worker"),
		newServiceRunner("server", "./cmd/server"),
	)
	return cmd
}

func newServiceRunner(name, path string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("go run %s", path),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			goArgs := []string{"run", path}
			goArgs = append(goArgs, args...)
			return runCommand(ctx, "go", goArgs...)
		},
	}
}

func runCommand(ctx context.Context, name string, args ...string) error {
	execCmd := exec.CommandContext(ctx, name, args...)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Stdin = os.Stdin
	return execCmd.Run()
}

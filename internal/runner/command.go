package runner

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

func runOne(opts Options, cmdDir string, c config.Command) evidence.Command {
	started := time.Now().UTC()
	rec := evidence.Command{
		ID:         c.ID,
		Run:        c.Run,
		StartedAt:  started,
		StdoutPath: filepath.Join("commands", c.ID+".stdout.txt"),
		StderrPath: filepath.Join("commands", c.ID+".stderr.txt"),
	}

	stdoutFile, err := os.Create(filepath.Join(cmdDir, c.ID+".stdout.txt"))
	if err != nil {
		return finishRecord(rec, err)
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(filepath.Join(cmdDir, c.ID+".stderr.txt"))
	if err != nil {
		return finishRecord(rec, err)
	}
	defer stderrFile.Close()

	return finishRecord(rec, execute(opts, filepath.Dir(cmdDir), c.Run, stdoutFile, stderrFile))
}

// execute gives profile commands and contract checks identical shell, working
// directory, environment, streaming, and sequential execution behavior.
func execute(opts Options, runDir, command string, stdout, stderr io.Writer) error {
	cmd := shellCommand(command)
	cmd.Dir = opts.RepoDir
	cmd.Env = append(gitutil.CommandEnv(), "LUNARFORGE_RUN_DIR="+runDir)
	if opts.Stream != nil {
		cmd.Stdout = io.MultiWriter(stdout, opts.Stream)
		cmd.Stderr = io.MultiWriter(stderr, opts.Stream)
	} else {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
	}
	return cmd.Run()
}

func finishRecord(rec evidence.Command, err error) evidence.Command {
	rec.FinishedAt = time.Now().UTC()
	rec.DurationMs = rec.FinishedAt.Sub(rec.StartedAt).Milliseconds()
	rec.ExitCode = exitCode(err)
	if err == nil {
		rec.Result = evidence.ResultPassed
	} else {
		rec.Result = evidence.ResultFailed
	}
	return rec
}

// shellCommand wraps a command string in the platform shell so that constructs
// like "npm run lint" or "./scripts/verify.sh" work as written in config.
func shellCommand(run string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", run)
	}
	return exec.Command("sh", "-c", run)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

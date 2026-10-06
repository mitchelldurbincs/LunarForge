package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

const contractTailBytes = 4096

// tailBuffer bounds memory as output arrives, including for very noisy tools.
type tailBuffer struct{ data []byte }

func (b *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if n >= contractTailBytes {
		b.data = append(b.data[:0], p[n-contractTailBytes:]...)
	} else {
		if excess := len(b.data) + n - contractTailBytes; excess > 0 {
			b.data = b.data[excess:]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

func runContract(opts Options, runDir string, row config.Contract) evidence.Contract {
	var stdout, stderr tailBuffer
	err := execute(opts, runDir, row.Check, &stdout, &stderr)
	rec := evidence.Contract{ID: row.ID, Command: row.Check, ExitCode: exitCode(err), StdoutTail: string(stdout.data), StderrTail: string(stderr.data)}
	switch rec.ExitCode {
	case 0:
		rec.Status = evidence.ResultPassed
		rec.Reason = "check passed (exit 0)"
	case 1:
		rec.Status = evidence.ResultFailed
		rec.Reason = "check failed (exit 1)"
	default:
		rec.Status = evidence.ResultError
		rec.Reason = fmt.Sprintf("check error (exit %d): %v", rec.ExitCode, err)
		if rec.ExitCode == -1 {
			rec.Reason = fmt.Sprintf("check could not execute or was terminated: %v", err)
		}
	}
	if row.Reason != "" {
		rec.Reason += "; " + row.Reason
	}
	if err != nil {
		for _, tail := range []string{rec.StderrTail, rec.StdoutTail} {
			if text := strings.TrimSpace(tail); text != "" {
				rec.Reason += "; " + text
			}
		}
	}
	return rec
}

func saveContract(runDir string, rec evidence.Contract) error {
	dir := filepath.Join(runDir, "contracts", rec.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	for name, data := range map[string][]byte{"evidence.json": data, "stdout.txt": []byte(rec.StdoutTail), "stderr.txt": []byte(rec.StderrTail)} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// reusableChecks selects only exact subject evidence. Profile commands still
// execute on every verify invocation; checks reuse their bounded saved results.
func reusableChecks(cfg *config.Config, opts Options, current *evidence.Evidence) *evidence.Evidence {
	if !cfg.HasChecks() {
		return nil
	}
	var previous *evidence.Evidence
	var err error
	if current.Identity != nil {
		previous, _, err = evidence.SelectCommit(opts.EvidenceDir, *current.Identity, false)
		if err != nil || !evidence.EvaluateCommit(previous, *current.Identity, false).Fresh {
			return nil
		}
	} else {
		previous, _, err = evidence.LoadLatest(opts.EvidenceDir)
		if err != nil || previous.Mode == "commit" || previous.DiffHash != current.DiffHash || previous.Git.Head != current.Git.Head {
			return nil
		}
	}
	if !evidence.CheckConfigMatches(cfg, previous) {
		return nil
	}
	return previous
}

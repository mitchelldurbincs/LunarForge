package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

// Identity is the applicability key of an isolated commit execution.
type Identity struct {
	Repository string          `json:"repository"`
	Subject    gitutil.Subject `json:"subject"`
	Contract   string          `json:"contract"`
	Platform   Platform        `json:"platform"`
}

// Platform records the runner and declared tool versions without dumping secrets.
type Platform struct {
	OS     string            `json:"os"`
	Arch   string            `json:"arch"`
	Runner string            `json:"runner"`
	Go     string            `json:"go"`
	Tools  map[string]string `json:"tools"`
}

// Digest hashes a JSON-serializable value with stable map ordering.
func Digest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// CaptureIdentity resolves the effective config, declared external input contents,
// and tool versions. Version commands must be read-only and deterministic.
func CaptureIdentity(cfg *config.Config, repo string) (*Identity, error) {
	evidenceDir := cfg.EvidenceDir()
	if !filepath.IsAbs(evidenceDir) {
		evidenceDir = filepath.Join(repo, evidenceDir)
	}
	subject, err := gitutil.ReadSubject(repo, ArtifactExcludes(repo, evidenceDir)...)
	if err != nil {
		return nil, err
	}
	inputs := map[string]string{}
	for _, path := range cfg.Verify.Inputs {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("verify.inputs requires absolute paths: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("contract input: %w", err)
		}
		inputs[path] = Digest(data)
	}
	platform, err := capturePlatform(cfg, repo)
	if err != nil {
		return nil, err
	}
	repository := cfg.Project.RepositoryID
	if repository == "" {
		repository, err = filepath.EvalSymlinks(repo)
		if err != nil {
			return nil, err
		}
	}
	return &Identity{Repository: repository, Subject: subject, Contract: Digest(struct {
		Config *config.Config
		Inputs map[string]string
	}{cfg, inputs}), Platform: platform}, nil
}

// SameInputs compares content, contract, repository, and platform, excluding commit
// metadata. Callers must separately require clean subjects and opt in to reuse.
func (i Identity) SameInputs(other Identity) bool {
	return i.Repository == other.Repository && i.Subject.Tree == other.Subject.Tree && i.Contract == other.Contract && Digest(i.Platform) == Digest(other.Platform)
}

func capturePlatform(cfg *config.Config, repo string) (Platform, error) {
	platform := Platform{OS: runtime.GOOS, Arch: runtime.GOARCH, Runner: "commit-evidence-v1", Go: runtime.Version(), Tools: map[string]string{}}
	versions := map[string]string{"git": "git --version"}
	for name, command := range cfg.Verify.ToolVersions {
		versions[name] = command
	}
	for name, command := range versions {
		cmd := exec.Command("sh", "-c", command)
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", command)
		}
		cmd.Dir = repo
		out, err := cmd.Output()
		if err != nil {
			return Platform{}, fmt.Errorf("tool version %s: %w", name, err)
		}
		platform.Tools[name] = strings.TrimSpace(string(out))
	}
	shell := "sh"
	if runtime.GOOS == "windows" {
		shell = "cmd"
	}
	shellPath, err := exec.LookPath(shell)
	if err != nil {
		return Platform{}, err
	}
	shellData, err := os.ReadFile(shellPath)
	if err != nil {
		return Platform{}, err
	}
	platform.Tools["shell"] = Digest(shellData)
	return platform, nil
}

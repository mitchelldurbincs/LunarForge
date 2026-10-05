package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

func TestInitVerifyLoopStatusReady(t *testing.T) {
	// Arrange: real stock init layout with no root .gitignore.
	repo := newTestRepo(t, passingConfig)
	if err := os.Remove(filepath.Join(repo, ".lunarforge.yml")); err != nil {
		t.Fatal(err)
	}
	if err := cmdInit(nil); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(repo, "scripts", "verify.sh"), "#!/bin/sh\ntrue\n", 0755)
	for _, args := range [][]string{{"add", ".lunarforge.yml", "scripts"}, {"commit", "-m", "configure verify"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	// Act.
	if err := cmdVerify([]string{"--quiet"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runLoop(t, repo, "--no-explain"); err != nil {
		t.Fatal(err)
	}
	out, err := inDir(t, repo, func() error { return cmdStatus([]string{"--json", "--require-fresh-passing"}) })
	// Assert.
	if err != nil {
		t.Fatalf("status: %v %s", err, out)
	}
	var status struct {
		Ready  bool   `json:"ready"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Ready || status.Reason == "dirty_subject" {
		t.Fatalf("not ready: %s", out)
	}
	loopDir(t, repo)
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 {
		t.Fatalf("stock layout is dirty: %v %s", err, out)
	}
}

func TestStatusAcceptsLegacyArtifactPorcelain(t *testing.T) {
	// Arrange: simulate an old record made before LF artifacts were ignored.
	repo := newTestRepo(t, passingConfig)
	if err := cmdVerify([]string{"--quiet"}); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(repo, ".lf", "runs")
	ev, runDir, err := evidence.LoadLatest(state)
	if err != nil {
		t.Fatal(err)
	}
	ev.Git.StatusPorcelain = "?? .lf/\n"
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "evidence.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	// Act / Assert.
	if err := cmdStatus([]string{"--require-fresh-passing"}); err != nil {
		t.Fatalf("legacy artifacts blocked readiness: %v", err)
	}
}

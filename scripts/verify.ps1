# LunarForge's own verify ritual (Windows / PowerShell) — this repo dogfooding
# itself. Mirrors scripts/verify.sh. Point verify.commands at this script in
# .lunarforge.yml when developing LunarForge on Windows.
$ErrorActionPreference = "Stop"

# $ErrorActionPreference does not make native commands throw, so every `go`
# invocation checks $LASTEXITCODE explicitly.
function Assert-ExitCode {
    param([string]$Step)
    if ($LASTEXITCODE -ne 0) {
        Write-Error "verify: $Step failed with exit code $LASTEXITCODE"
        exit $LASTEXITCODE
    }
}

Write-Host "==> gofmt"
$unformatted = & gofmt -l .
Assert-ExitCode "gofmt"
if ($unformatted) {
    Write-Host "these files are not gofmt-clean:"
    $unformatted | ForEach-Object { Write-Host "  $_" }
    Write-Error "verify: gofmt found unformatted files (fix with: gofmt -w .)"
    exit 1
}

Write-Host "==> go vet"
& go vet ./...
Assert-ExitCode "go vet"

Write-Host "==> go build"
& go build ./...
Assert-ExitCode "go build"

Write-Host "==> go test"
& go test ./...
Assert-ExitCode "go test"

Write-Host "verify.ps1: all checks passed"

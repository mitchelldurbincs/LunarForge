# Example repo verify ritual for Windows (PowerShell).
# This is the "real repo ritual" LunarForge enforces. Edit it to run the
# lint/build/test commands that actually matter for your project. Any failing
# command should stop the script with a non-zero exit code.
$ErrorActionPreference = "Stop"

Write-Host "==> lint"
# npm run lint

Write-Host "==> typecheck"
# npm run typecheck

Write-Host "==> test"
# npm test

Write-Host "==> build"
# npm run build

Write-Host "verify.ps1: all checks passed"

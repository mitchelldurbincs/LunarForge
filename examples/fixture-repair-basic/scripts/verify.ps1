# Dependency-free verify ritual for the repair fixture (PowerShell variant).
# Asserts that src/hello.txt contains the expected marker text.
$ErrorActionPreference = "Stop"

$expected = "hello lunarforge"
$file = "src/hello.txt"

if (-not (Test-Path $file)) {
    Write-Error "verify: $file is missing"
    exit 1
}

$content = Get-Content $file -Raw
if ($content -match [regex]::Escape($expected)) {
    Write-Output "verify: $file contains expected text: `"$expected`""
    exit 0
}

Write-Error "verify: $file does not contain expected text: `"$expected`""
exit 1

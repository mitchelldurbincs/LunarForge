# Dependency-free verify ritual for the fixture (Windows / PowerShell).
# Asserts that src/hello.txt still contains the expected marker text.
$ErrorActionPreference = "Stop"

$expected = "hello lunarforge"
$file = "src/hello.txt"

if (-not (Test-Path $file)) {
    Write-Error "verify: $file is missing"
    exit 1
}

if (Select-String -Path $file -SimpleMatch -Pattern $expected -Quiet) {
    Write-Host "verify: $file contains expected text: `"$expected`""
    exit 0
}

Write-Error "verify: $file does not contain expected text: `"$expected`""
exit 1

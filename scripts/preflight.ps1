#!/usr/bin/env pwsh
# Local mirror of the CI release validate job
# (.github/workflows/release.yml -> jobs.validate).
#
# Run this BEFORE creating a release tag. If this script exits 0,
# the same checks will pass on CI. If you add or change a check in
# release.yml, update this script in the same PR.
#
#   pwsh ./scripts/preflight.ps1
#
# History note: v1.4.0 shipped a release that failed validate because
# `gofmt -l .` flagged graph/graph.go locally was unformatted. The
# original RELEASE-CHECKLIST.md Phase 1 listed `go vet` + `go test`
# + `go build` but not `gofmt`. This script exists to make that class
# of mistake impossible by being THE canonical source of truth for
# what "ready to tag" means.

$ErrorActionPreference = 'Stop'
$failures = @()

function Step($label, $cmd) {
    Write-Host ""
    Write-Host "==> $label" -ForegroundColor Cyan
    & $cmd
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL: $label" -ForegroundColor Red
        $script:failures += $label
    } else {
        Write-Host "OK"   -ForegroundColor Green
    }
}

Write-Host ""
Write-Host "==> gofmt -l ." -ForegroundColor Cyan
$dirty = gofmt -l .
if ($dirty) {
    Write-Host "FAIL: gofmt found unformatted files:" -ForegroundColor Red
    $dirty | ForEach-Object { Write-Host "  $_" -ForegroundColor Red }
    Write-Host "  Fix with: gofmt -w ." -ForegroundColor Yellow
    $failures += "gofmt -l ."
} else {
    Write-Host "OK" -ForegroundColor Green
}

Step "go vet ./..."        { go vet ./... }
Step "go test ./... -count=1" { go test ./... -count=1 }
Step "go build -o architex.exe ./cmd/architex" { go build -o architex.exe ./cmd/architex }

Write-Host ""
if ($failures.Count -gt 0) {
    Write-Host ("PREFLIGHT FAILED ({0} check(s)): {1}" -f $failures.Count, ($failures -join ', ')) -ForegroundColor Red
    Write-Host "DO NOT tag a release until every check above passes." -ForegroundColor Red
    exit 1
}
Write-Host "ALL PREFLIGHT CHECKS PASSED -- safe to tag." -ForegroundColor Green
exit 0

# scripts/validate-real-world.ps1
#
# Phase 7 final pre-promotion gate: run ArchiteX against real, in-the-wild
# Terraform repositories and record what fails (panics, parse errors,
# unsupported_resource warnings, surprising risk scores). The output is a
# CSV + a per-repo log directory you can grep through.
#
# This script DOES touch the network (it shallow-clones each repo) and
# DOES write to disk (`./.validation-runs/`); both effects are isolated
# under a workspace-local folder so nothing else in the repo is touched.
#
# Usage:
#   pwsh ./scripts/validate-real-world.ps1                       # run the built-in 10-repo smoke set
#   pwsh ./scripts/validate-real-world.ps1 -RepoList repos.txt   # custom list (one URL per line)
#   pwsh ./scripts/validate-real-world.ps1 -OutDir ./out         # change output dir
#   pwsh ./scripts/validate-real-world.ps1 -SkipClone            # reuse an existing checkout dir
#
# Output:
#   .validation-runs/
#     <timestamp>/
#       summary.csv               # one row per repo: status, parse_warns, score, severity
#       repos/<owner>/<name>/...  # the cloned source (so you can re-grep)
#       logs/<owner>__<name>.log  # full architex output per repo
#
# Exit code:
#   0 if all repos completed (warnings are tracked but do NOT fail the run --
#     the whole point is to discover them).
#   1 if architex panicked or returned a non-zero exit on any repo.
#
# IMPORTANT: this is a development helper, not a CI gate. The list of
# repos and the bar for "good enough to promote" are policy decisions the
# maintainer makes by reading summary.csv and the logs.

param(
  [string] $RepoList = "",
  [string] $OutDir = ".validation-runs",
  [switch] $SkipClone
)

$ErrorActionPreference = "Continue"
# Architex emits informational lines on stderr (`[architex] WARN ...`) and a
# strict ErrorActionPreference would cause PowerShell to throw on every one.
# We rely on $LASTEXITCODE for hard failures and on per-line parsing for
# warnings, so "Continue" is the correct setting here.

# Built-in smoke set: a representative cross-section of real-world AWS
# Terraform usage. Mix of: official terraform-aws-modules monorepos,
# AWS-curated examples, popular community module collections. NOT meant
# to be exhaustive; the user can pass -RepoList for the full top-50 sweep.
$DefaultRepos = @(
  "https://github.com/terraform-aws-modules/terraform-aws-vpc",
  "https://github.com/terraform-aws-modules/terraform-aws-eks",
  "https://github.com/terraform-aws-modules/terraform-aws-rds",
  "https://github.com/terraform-aws-modules/terraform-aws-s3-bucket",
  "https://github.com/terraform-aws-modules/terraform-aws-iam",
  "https://github.com/terraform-aws-modules/terraform-aws-lambda",
  "https://github.com/terraform-aws-modules/terraform-aws-security-group",
  "https://github.com/terraform-aws-modules/terraform-aws-alb",
  "https://github.com/terraform-aws-modules/terraform-aws-ecs",
  "https://github.com/terraform-aws-modules/terraform-aws-cloudfront"
)

# Resolve repos to scan.
$repos = @()
if ($RepoList -ne "") {
  if (-not (Test-Path $RepoList)) { throw "RepoList file not found: $RepoList" }
  $repos = Get-Content $RepoList | Where-Object { $_ -and -not $_.StartsWith("#") }
} else {
  $repos = $DefaultRepos
}
Write-Host "[validate] $($repos.Count) repos to scan."

# Build architex.exe once (Windows) or `architex` (POSIX). Bail early on a
# build failure -- there's no point cloning anything if the binary is broken.
Write-Host "[validate] building architex CLI..."
$bin = if ($IsWindows -or $env:OS -like "Windows*") { "architex.exe" } else { "architex" }
& go build -o $bin .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }
$binAbs = (Resolve-Path $bin).Path

# Lay out output dirs.
$stamp  = Get-Date -Format "yyyyMMdd-HHmmss"
$runDir = Join-Path $OutDir $stamp
$null   = New-Item -ItemType Directory -Path $runDir -Force
$null   = New-Item -ItemType Directory -Path (Join-Path $runDir "repos") -Force
$null   = New-Item -ItemType Directory -Path (Join-Path $runDir "logs")  -Force

$csv = Join-Path $runDir "summary.csv"
"repo,status,exit_code,duration_sec,nodes,edges,score,severity,unsupported_warnings,first_unsupported,parse_errors" | Out-File $csv -Encoding utf8

$anyHardFailure = $false

foreach ($url in $repos) {
  $owner = ($url -replace "https?://github.com/","" -split "/")[0]
  $name  = ($url -replace "https?://github.com/","" -split "/")[1] -replace "\.git$",""
  $repoDir = Join-Path $runDir "repos/$owner/$name"
  $log     = Join-Path $runDir "logs/$($owner)__$($name).log"

  Write-Host ""
  Write-Host "[validate] === $owner/$name ==="

  if (-not $SkipClone) {
    if (Test-Path $repoDir) { Remove-Item $repoDir -Recurse -Force }
    $null = New-Item -ItemType Directory -Path (Split-Path $repoDir) -Force
    $cloneOut = & git clone --depth 1 --quiet $url $repoDir 2>&1
    $cloneOut | Out-File $log -Append -Encoding utf8
    if ($LASTEXITCODE -ne 0) {
      Write-Warning "clone failed for $url"
      "$url,clone_failed,$LASTEXITCODE,0,0,0,0,unknown,0,," | Out-File $csv -Append -Encoding utf8
      $anyHardFailure = $true
      continue
    }
  } elseif (-not (Test-Path $repoDir)) {
    Write-Warning "$repoDir not present; skipping (run without -SkipClone first)"
    continue
  }

  # Run architex graph on the repo root. We use `graph` (not `score`)
  # because there's no base/head pair here -- we just want to know if
  # the parser walks the codebase cleanly. Capture stdout/stderr
  # separately so JSON parsing only sees stdout and warnings only get
  # counted once.
  $start = Get-Date
  $stdoutFile = Join-Path $runDir "logs/$($owner)__$($name).stdout"
  $stderrFile = Join-Path $runDir "logs/$($owner)__$($name).stderr"
  $proc = Start-Process -FilePath $binAbs -ArgumentList @("graph", $repoDir) `
    -NoNewWindow -Wait -PassThru `
    -RedirectStandardOutput $stdoutFile -RedirectStandardError $stderrFile
  $exit = $proc.ExitCode
  $dur = [int]((Get-Date) - $start).TotalSeconds
  $stdout = if (Test-Path $stdoutFile) { Get-Content $stdoutFile -Raw } else { "" }
  $stderr = if (Test-Path $stderrFile) { Get-Content $stderrFile -Raw } else { "" }
  "----- STDERR -----" | Out-File $log -Append -Encoding utf8
  $stderr | Out-File $log -Append -Encoding utf8
  "----- STDOUT (truncated) -----" | Out-File $log -Append -Encoding utf8
  ($stdout.Substring(0, [Math]::Min(2000, $stdout.Length))) | Out-File $log -Append -Encoding utf8

  # Detect a panic in stderr (Go's runtime stacks always start with this).
  $panicked = $stderr -match "(?m)^panic: "
  if ($panicked) {
    Write-Warning "PANIC detected for $owner/$name (see $log)"
    $anyHardFailure = $true
  }

  # Parse the JSON `graph` output (stdout is pure JSON; stderr has
  # `[architex] ...` warnings). Count nodes/edges and pull warning info.
  $nodes = 0; $edges = 0; $warns = 0; $firstWarn = ""; $parseErrors = 0
  try {
    $g = $stdout | ConvertFrom-Json -ErrorAction Stop
    $nodes = if ($g.nodes) { @($g.nodes).Count } else { 0 }
    $edges = if ($g.edges) { @($g.edges).Count } else { 0 }
    # Warnings live under confidence.warnings (matches Graph.Confidence.Warnings).
    $warnList = $null
    if ($g.confidence -and $g.confidence.warnings) {
      $warnList = @($g.confidence.warnings)
    } elseif ($g.warnings) {
      $warnList = @($g.warnings)
    }
    if ($warnList -and $warnList.Count -gt 0) {
      $warns = $warnList.Count
      $firstWarn = $warnList[0].message
    }
  } catch {
    if ($exit -eq 0) { $parseErrors = 1 }
  }

  # `graph` doesn't compute a score (no base), so we only record the
  # rest. Severity stays "n/a" for this kind of run; switch to `score`
  # against an empty fixture to compute a "what does this repo look
  # like as one giant addition?" number once the smoke-set passes.
  $status = if ($panicked) { "panic" } elseif ($exit -ne 0) { "exit_nonzero" } else { "ok" }
  if ($status -ne "ok") { $anyHardFailure = $true }

  $row = "$url,$status,$exit,$dur,$nodes,$edges,n/a,n/a,$warns,""$($firstWarn -replace '"','""')"",$parseErrors"
  $row | Out-File $csv -Append -Encoding utf8

  Write-Host "  status=$status  exit=$exit  dur=${dur}s  nodes=$nodes  edges=$edges  warnings=$warns"
}

Write-Host ""
Write-Host "[validate] DONE. Summary:    $csv"
Write-Host "[validate]       Per-repo logs: $(Join-Path $runDir 'logs')"

if ($anyHardFailure) {
  Write-Warning "[validate] At least one repo panicked or returned non-zero. Triage required before promotion."
  exit 1
}
exit 0

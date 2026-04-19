# Mermaid + summary.md size stress test.
#
# Purpose: produce empirical numbers for "how big a delta does ArchiteX
# emit?" so we can verify the renderer's density cap and the comment
# poster's body safety net (interpreter/mermaid.go + main.go runComment)
# behave correctly on large deltas. Useful both as a one-off probe and
# as a regression check after touching the renderer or the formatter.
#
# It generates a synthetic Terraform pair where `head` adds N new
# (SecurityGroup-with-open-ingress, LB, EC2) triplets vs `base`, runs
# `architex report`, and prints a table of summary.md / Mermaid sizes
# plus node/edge counts.
#
# Reference cliffs (measured 2026-04-19, with citations):
#   - Visual spaghetti                : ~30 nodes (subjective; no doc, observation)
#   - mermaid-js maxTextSize default  : 50,000 chars in the Mermaid block
#       https://github.com/mermaid-js/mermaid-cli/issues/113
#       (over this, GitHub shows "Maximum text size in diagram exceeded")
#   - GitHub comment body mediumblob  : 262,144 bytes
#       https://github.community/t/maximum-length-for-the-comment-body-in-issues-and-pr/148867
#       (the often-cited "65,536 chars" is the 4-byte-per-char worst case)
#
# ArchiteX defends against the latter two with:
#   - interpreter.MermaidBudget   = 45,000 (5 KB margin under 50,000)
#   - main.commentBodyBudget      = 240,000 (22 KB margin under 262,144)
# Both are deterministic; truncation is announced visibly in the output.
#
# Run from repo root (after `go build -o architex.exe .`):
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/stress-mermaid.ps1
#
# Optional: pass `-Sizes 5,25,50` etc. to override the default sweep.

[CmdletBinding()]
param(
    [int[]]$Sizes = @(5, 25, 50, 100, 200)
)

# Note: native binaries that write to stderr trigger PS error records under
# 'Stop'. We use 'Continue' here and check $LASTEXITCODE explicitly.
$ErrorActionPreference = 'Continue'

$root  = Resolve-Path "$PSScriptRoot\.."
$exe   = Join-Path $root 'architex.exe'
$work  = Join-Path $env:TEMP 'architex-stress'

if (-not (Test-Path $exe)) {
    throw "architex.exe not found at $exe -- run 'go build -o architex.exe .' first"
}

function New-Base {
    param([string]$dir)
    Remove-Item -Recurse -Force $dir -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    @'
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
resource "aws_subnet" "main" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.1.0/24"
}
'@ | Set-Content -Path (Join-Path $dir 'main.tf') -Encoding utf8
}

function New-Head {
    param([string]$dir, [int]$n)
    New-Base $dir
    $sb = New-Object System.Text.StringBuilder
    for ($i = 1; $i -le $n; $i++) {
        # Each iteration: 1 SG (open ingress -> high risk), 1 LB (entry_point), 1 EC2.
        $null = $sb.AppendLine(@"
resource "aws_security_group" "sg_$i" {
  vpc_id = aws_vpc.main.id
  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
resource "aws_lb" "lb_$i" {
  internal = false
  subnets  = [aws_subnet.main.id]
}
resource "aws_instance" "ec2_$i" {
  subnet_id              = aws_subnet.main.id
  vpc_security_group_ids = [aws_security_group.sg_$i.id]
  ami                    = "ami-deadbeef"
  instance_type          = "t3.micro"
}
"@)
    }
    Add-Content -Path (Join-Path $dir 'main.tf') -Value $sb.ToString() -Encoding utf8
}

function Measure-Run {
    param([int]$n)
    $base = Join-Path $work "base-$n"
    $head = Join-Path $work "head-$n"
    $sumPath = Join-Path $work "summary-$n.md"

    New-Base $base
    New-Head $head $n

    # Capture stdout (the full Markdown report) and discard stderr (the
    # human-readable progress lines). Exit codes 0 and 1 are both expected
    # since these high-risk deltas legitimately produce risk.Status="fail".
    $summary = & $exe report $base $head 2>$null
    if ($LASTEXITCODE -gt 1) {
        Write-Warning "architex exited with $LASTEXITCODE for n=$n"
    }
    $summaryText = $summary -join "`n"
    Set-Content -Path $sumPath -Value $summaryText -Encoding utf8

    # Extract the mermaid block (between ```mermaid and ```).
    $inMermaid = $false
    $mermaidLines = @()
    foreach ($line in $summary) {
        if ($line -match '^```mermaid') { $inMermaid = $true; continue }
        if ($inMermaid -and $line -match '^```')   { $inMermaid = $false; continue }
        if ($inMermaid) { $mermaidLines += $line }
    }
    $mmd = $mermaidLines -join "`n"
    $nodes = ($mermaidLines | Where-Object { $_ -match '^\s+\w+\["' }).Count
    $edges = ($mermaidLines | Where-Object { $_ -match '-->|-\.->' -and $_ -notmatch 'classDef' }).Count

    [pscustomobject]@{
        N            = $n
        SummaryBytes = [System.Text.Encoding]::UTF8.GetByteCount($summaryText)
        MermaidBytes = [System.Text.Encoding]::UTF8.GetByteCount($mmd)
        MermaidNodes = $nodes
        MermaidEdges = $edges
        # Cap engaged when the renderer emitted the truncation placeholder.
        # Detection is exact: the placeholder ID is unique to budget mode.
        DiagramCapped = ($mmd -match '_architex_truncated')
        # GitHub stores comments in a 262,144-byte mediumblob; we cap at 240,000.
        OverBodyCap   = ([System.Text.Encoding]::UTF8.GetByteCount($summaryText) -gt 240000)
    }
}

$results = @()
foreach ($n in $Sizes) {
    Write-Host ">>> n=$n" -ForegroundColor Cyan
    $results += Measure-Run $n
}

Write-Host ""
Write-Host "Results (limits: mermaid block 45,000 chars budget; comment body 240,000 bytes budget):" -ForegroundColor Yellow
$results | Format-Table -AutoSize

Write-Host "Per-N summaries written to: $work\summary-<N>.md" -ForegroundColor DarkGray

param(
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location -LiteralPath $root

if (-not $Force) {
    Write-Host "This operation rewrites every local branch and tag to remove .env from Git history."
    Write-Host "Rotate every exposed secret first, coordinate with collaborators, then run:"
    Write-Host "  .\scripts\purge-env-history.ps1 -Force"
    exit 2
}

if (-not (Get-Command git-filter-repo -ErrorAction SilentlyContinue)) {
    throw "git-filter-repo is required. Install it from https://github.com/newren/git-filter-repo"
}

$status = git status --porcelain
if ($LASTEXITCODE -ne 0) {
    throw "Unable to inspect the worktree"
}
if ($status) {
    throw "Commit or stash all changes before rewriting history"
}

git filter-repo --path .env --invert-paths --force
if ($LASTEXITCODE -ne 0) {
    throw "git-filter-repo failed"
}

Write-Host "Local history was rewritten. Verify it, then force-push every branch and tag using --force-with-lease."
Write-Host "Every collaborator must clone the repository again. Rotation is still required even after deletion."

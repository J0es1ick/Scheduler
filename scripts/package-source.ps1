param(
    [string]$Output = "scheduler-source.zip"
)

$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$target = [System.IO.Path]::GetFullPath((Join-Path $root $Output))
if (-not $target.StartsWith($root, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Output must stay inside the repository"
}
if (Test-Path -LiteralPath $target) {
    Remove-Item -LiteralPath $target -Force
}

Push-Location $root
try {
    git archive --format=zip --output=$target HEAD
    if ($LASTEXITCODE -ne 0) {
        throw "git archive failed"
    }
} finally {
    Pop-Location
}

Write-Host "Created a tracked-files-only archive: $target"

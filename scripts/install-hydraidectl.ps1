$Version = Invoke-RestMethod https://api.github.com/repos/hydraide/hydraide/releases/latest | Select-Object -ExpandProperty tag_name
$Url = "https://github.com/hydraide/hydraide/releases/download/$Version/hydraidectl-windows-amd64.exe"
$TargetPath = "$env:ProgramFiles\Hydraide"
$ExePath = Join-Path $TargetPath "hydraidectl.exe"

Write-Host "🔽 Downloading $Url..."
New-Item -ItemType Directory -Force -Path $TargetPath | Out-Null
Invoke-WebRequest -Uri $Url -OutFile $ExePath

# Add to PATH if not already
$CurrentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
if ($CurrentPath -notlike "*$TargetPath*") {
  [Environment]::SetEnvironmentVariable(
    "Path",
    "$CurrentPath;$TargetPath",
    [EnvironmentVariableTarget]::Machine
  )
  Write-Host "🔧 Added $TargetPath to system PATH."
}

Write-Host "✅ hydraidectl installed to $ExePath"

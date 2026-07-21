# claude-teleport installer for Windows (PowerShell).
#
#   irm https://raw.githubusercontent.com/gowtham-sai-yadav/claude-teleport/main/install.ps1 | iex
#
# Downloads the prebuilt Windows binary from the latest GitHub release, verifies
# its SHA-256 checksum, installs it under %LOCALAPPDATA%, and adds it to your
# user PATH.
#
# Override the version with:  $env:VERSION = "v0.3.0"

$ErrorActionPreference = "Stop"

$repo    = "gowtham-sai-yadav/claude-teleport"
$binary  = "claude-teleport"
$version = if ($env:VERSION) { $env:VERSION } else { "latest" }
$asset   = "$binary-windows-amd64.exe"   # amd64 build runs on Windows arm64 via emulation

$base = if ($version -eq "latest") {
    "https://github.com/$repo/releases/latest/download"
} else {
    "https://github.com/$repo/releases/download/$version"
}

$installDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\claude-teleport" }
New-Item -ItemType Directory -Force -Path $installDir | Out-Null

$tmp = New-TemporaryFile
Write-Host "Downloading $asset ($version)..."
Invoke-WebRequest -Uri "$base/$asset" -OutFile $tmp

# Verify the checksum if the sums file is available.
try {
    $sumsPath = New-TemporaryFile
    Invoke-WebRequest -Uri "$base/SHA256SUMS.txt" -OutFile $sumsPath
    $line = Select-String -Path $sumsPath -Pattern " $asset$" | Select-Object -First 1
    if ($line) {
        $want = ($line.Line -split '\s+')[0]
        $got  = (Get-FileHash -Algorithm SHA256 -Path $tmp).Hash.ToLower()
        if ($want.ToLower() -ne $got) {
            throw "checksum mismatch for $asset (expected $want, got $got)"
        }
        Write-Host "Checksum verified."
    }
} catch {
    if ($_.Exception.Message -like "*checksum mismatch*") { throw }
    Write-Host "Note: could not verify checksum, continuing."
}

$dest = Join-Path $installDir "$binary.exe"
Move-Item -Force -Path $tmp -Destination $dest
Write-Host "Installed $binary to $dest"

# Add the install dir to the user PATH if it is not there already.
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not ($userPath -split ';' | Where-Object { $_ -eq $installDir })) {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    $env:Path = "$env:Path;$installDir"
    Write-Host "Added $installDir to your PATH. Open a new terminal for it to take effect."
}

& $dest version
Write-Host "Done. Run '$binary' to get started."

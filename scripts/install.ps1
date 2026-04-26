param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]] $TldArgs
)

$ErrorActionPreference = "Stop"

$Binary = "tld.exe"
$Repo = "Mertcikla/tld"

function Get-TldArchitecture {
    $architecture = $env:PROCESSOR_ARCHITEW6432
    if ([string]::IsNullOrWhiteSpace($architecture)) {
        $architecture = $env:PROCESSOR_ARCHITECTURE
    }

    switch ($architecture.ToUpperInvariant()) {
        "AMD64" { return "x86_64" }
        "ARM64" { return "arm64" }
        default {
            throw "Unsupported architecture: $architecture"
        }
    }
}

function Add-TldPath {
    param([string] $InstallDir)

    $isInProcessPath = ($env:Path -split ";") | Where-Object { $_ -and ($_.TrimEnd("\") -ieq $InstallDir.TrimEnd("\")) }
    if (-not $isInProcessPath) {
        $env:Path = "$env:Path;$InstallDir"
    }

    $pathParts = [Environment]::GetEnvironmentVariable("Path", "User") -split ";"
    $isInUserPath = $pathParts | Where-Object { $_ -and ($_.TrimEnd("\") -ieq $InstallDir.TrimEnd("\")) }
    if ($isInUserPath) {
        return
    }

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ([string]::IsNullOrWhiteSpace($userPath)) {
        [Environment]::SetEnvironmentVariable("Path", $InstallDir, "User")
    } else {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
    }

    Write-Host "Added $InstallDir to your user PATH."
    Write-Host "Open a new terminal before running tld from another shell."
}

try {
    if ([Net.ServicePointManager]::SecurityProtocol -notmatch "Tls12") {
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    }
} catch {
    # Older PowerShell hosts may not expose this setting; Invoke-WebRequest will use the host default.
}

$Arch = Get-TldArchitecture

if ([string]::IsNullOrWhiteSpace($env:INSTALL_DIR)) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "tld\bin"
} else {
    $InstallDir = $env:INSTALL_DIR
}

$LatestReleaseUrl = "https://api.github.com/repos/$Repo/releases/latest"
Write-Host "Finding latest tld release..."
$Release = Invoke-RestMethod -Uri $LatestReleaseUrl -Headers @{ "User-Agent" = "tld-installer" }
$Version = $Release.tag_name

if ([string]::IsNullOrWhiteSpace($Version)) {
    throw "Could not find latest version for $Repo"
}

$Filename = "tld_Windows_$Arch.zip"
$Url = "https://github.com/mertcikla/tld/releases/download/$Version/$Filename"
$TempDir = Join-Path ([IO.Path]::GetTempPath()) ("tld-install-" + [Guid]::NewGuid().ToString("N"))
$ZipPath = Join-Path $TempDir $Filename

Write-Host "Downloading tld $Version for Windows/$Arch..."
New-Item -ItemType Directory -Path $TempDir -Force | Out-Null

try {
    Invoke-WebRequest -Uri $Url -OutFile $ZipPath -Headers @{ "User-Agent" = "tld-installer" }

    Write-Host "Extracting $Filename..."
    Expand-Archive -Path $ZipPath -DestinationPath $TempDir -Force

    $ExtractedBinary = Get-ChildItem -Path $TempDir -Recurse -Filter $Binary | Select-Object -First 1
    if (-not $ExtractedBinary) {
        throw "Could not find $Binary in downloaded archive"
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $Destination = Join-Path $InstallDir $Binary

    Write-Host "Installing to $Destination..."
    Copy-Item -Path $ExtractedBinary.FullName -Destination $Destination -Force

    Add-TldPath -InstallDir $InstallDir

    & $Destination --help *> $null
    Write-Host "Successfully installed! Run 'tld --help' to get started."

    if ($TldArgs.Count -gt 0) {
        Write-Host "--------------------------------------------------"
        Write-Host "Executing: tld $($TldArgs -join ' ')"
        & $Destination @TldArgs
        exit $LASTEXITCODE
    }
} finally {
    if (Test-Path $TempDir) {
        Remove-Item -Path $TempDir -Recurse -Force
    }
}

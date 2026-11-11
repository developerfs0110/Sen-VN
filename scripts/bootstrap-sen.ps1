<#
.SYNOPSIS
    Trình khởi tạo tự động (Cold-Start Bootstrapper) cho `bin/sen.cmd` trên Windows.
.DESCRIPTION
    Tự động tải tệp nhị phân dựng sẵn từ GitHub Releases kèm kiểm chứng chữ ký SHA-256,
    gỡ cờ Mark-of-the-Web bằng `Unblock-File`, hoặc biên dịch dự phòng một lần bằng Go nếu ngoại tuyến.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$RepoRoot
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

# Ưu tiên lưu tệp nhị phân vào thư mục dữ liệu bền vững của plugin ($env:PLUGIN_DATA) hoặc .cache\bin.
$cacheDir = if (-not [string]::IsNullOrWhiteSpace($env:PLUGIN_DATA)) {
    Join-Path $env:PLUGIN_DATA "bin"
}
else {
    Join-Path $RepoRoot ".cache\bin"
}
if (-not (Test-Path -LiteralPath $cacheDir)) {
    New-Item -ItemType Directory -Path $cacheDir -Force | Out-Null
}
$targetExe = Join-Path $cacheDir "sen.exe"
if (Test-Path -LiteralPath $targetExe) {
    exit 0
}

# Xác định định danh kho GitHub từ biến môi trường, git remote origin hoặc mặc định developerfs0110/Sen-VN.
$slug = $env:SEN_GITHUB_REPO
if ([string]::IsNullOrWhiteSpace($slug)) {
    $gitCmd = Get-Command git -ErrorAction SilentlyContinue
    if ($null -ne $gitCmd) {
        $originUrl = (& $gitCmd.Source -C $RepoRoot remote get-url origin 2>$null | Out-String).Trim()
        if ($originUrl -match 'github\.com[:/]([^/\s]+/[^/\s\.]+?)(?:\.git)?$') {
            $slug = $Matches[1]
        }
    }
}
if ([string]::IsNullOrWhiteSpace($slug)) {
    $slug = "developerfs0110/Sen-VN"
}

$arch = if ($env:PROCESSOR_ARCHITECTURE -match 'ARM64') { "arm64" } else { "amd64" }
$assetName = "sen-windows-$arch.exe"
$baseReleaseUrl = "https://github.com/$slug/releases/latest/download"
$downloaded = $false
$tmpSum = Join-Path ([System.IO.Path]::GetTempPath()) ("sen-sum-" + [guid]::NewGuid().ToString("N") + ".txt")
$tmpBin = Join-Path ([System.IO.Path]::GetTempPath()) ("sen-bin-" + [guid]::NewGuid().ToString("N") + ".exe")

# Bước 2: Tải checksums.txt + sen-windows-<arch>.exe qua HTTPS và đối chiếu SHA-256 nghiêm ngặt.
try {
    Invoke-WebRequest -Uri "$baseReleaseUrl/checksums.txt" -OutFile $tmpSum -UseBasicParsing -TimeoutSec 15
    Invoke-WebRequest -Uri "$baseReleaseUrl/$assetName" -OutFile $tmpBin -UseBasicParsing -TimeoutSec 30
    $wantHash = ""
    foreach ($line in (Get-Content -LiteralPath $tmpSum)) {
        if ($line -match "^([0-9a-fA-F]{64})\s+\*?$([regex]::Escape($assetName))$") {
            $wantHash = $Matches[1].ToLowerInvariant()
            break
        }
    }
    if ([string]::IsNullOrWhiteSpace($wantHash)) {
        throw "Asset $assetName missing from checksums.txt"
    }
    $gotHash = (Get-FileHash -LiteralPath $tmpBin -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($gotHash -ne $wantHash) {
        throw "SHA-256 mismatch for $assetName (expected $wantHash, got $gotHash)"
    }
    Move-Item -LiteralPath $tmpBin -Destination $targetExe -Force
    Unblock-File -LiteralPath $targetExe -ErrorAction SilentlyContinue
    $downloaded = $true
}
catch {
    [Console]::Error.WriteLine("[sen bootstrap] Release download skipped ($($_.Exception.Message)); trying local Go build...")
}
finally {
    if (Test-Path -LiteralPath $tmpSum) { Remove-Item -LiteralPath $tmpSum -Force -ErrorAction SilentlyContinue }
    if (Test-Path -LiteralPath $tmpBin) { Remove-Item -LiteralPath $tmpBin -Force -ErrorAction SilentlyContinue }
}

if ($downloaded) {
    exit 0
}

# Bước 3: Nếu chưa có bản Release hoặc đang ngoại tuyến, biên dịch dự phòng một lần bằng Go cục bộ.
$goExe = $null
$goCmd = Get-Command go -ErrorAction SilentlyContinue
if ($null -ne $goCmd) {
    $goExe = $goCmd.Source
}
elseif (Test-Path -LiteralPath "C:\Program Files\Go\bin\go.exe") {
    $goExe = "C:\Program Files\Go\bin\go.exe"
}

if ([string]::IsNullOrWhiteSpace($goExe)) {
    [Console]::Error.WriteLine("sen: neither prebuilt GitHub Release ($baseReleaseUrl/$assetName) nor Go toolchain was found.")
    exit 1
}

Push-Location $RepoRoot
try {
    $env:CGO_ENABLED = "0"
    $buildOut = & $goExe build -trimpath -ldflags="-s -w" -o $targetExe ./cmd/sen 2>&1 | Out-String
    if (-not [string]::IsNullOrWhiteSpace($buildOut)) {
        [Console]::Error.WriteLine($buildOut.TrimEnd())
    }
    if ($LASTEXITCODE -ne 0) {
        throw "go build exited with code $LASTEXITCODE"
    }
    Unblock-File -LiteralPath $targetExe -ErrorAction SilentlyContinue
}
finally {
    Pop-Location
}

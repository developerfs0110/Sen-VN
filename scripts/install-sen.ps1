<#
.SYNOPSIS
    Trình cài đặt và cập nhật `sen.exe` v1.1.0 an toàn cho Windows (không cần quyền Quản trị viên / UAC).
.DESCRIPTION
    Cài đặt hoặc cập nhật (`-Update`) `sen.exe` vào `%LOCALAPPDATA%\Programs\sen\bin`, xác thực chữ ký SHA-256,
    chặn giao thức HTTP không mã hóa, cập nhật User PATH và tùy chọn khởi tạo ngay không gian làm việc (`-Workspace`)
    kèm bộ kỹ năng đồng hành mặc định và các gói mở rộng (`-Packs`).
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $false)]
    [Alias("Source")]
    [string]$BinarySource = "",

    [Parameter(Mandatory = $false)]
    [string]$Repo = "developerfs0110/Sen-VN",

    [Parameter(Mandatory = $false)]
    [string]$ExpectedSha256 = "",

    [Parameter(Mandatory = $false)]
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "Programs\sen\bin"),

    [Parameter(Mandatory = $false)]
    [string]$Workspace = "",

    [Parameter(Mandatory = $false)]
    [string]$Packs = "",

    [Parameter(Mandatory = $false)]
    [switch]$SkipCompanions,

    [Parameter(Mandatory = $false)]
    [switch]$Update,

    [Parameter(Mandatory = $false)]
    [switch]$SkipUserPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

# Nếu chạy từ bản clone cục bộ đã biên dịch sẵn `sen.exe` (không truyền `-Repo` và không bật `-Update`), ưu tiên dùng tệp nội bộ.
$explicitSource = $PSBoundParameters.ContainsKey("BinarySource") -and (-not [string]::IsNullOrWhiteSpace($BinarySource))
if (-not $explicitSource -and (-not $Update)) {
    $defaultLocal = Join-Path $PSScriptRoot "..\sen.exe"
    if (-not $PSBoundParameters.ContainsKey("Repo") -and (Test-Path -LiteralPath $defaultLocal)) {
        $BinarySource = $defaultLocal
        $explicitSource = $true
    }
}

# Chặn tuyệt đối kết nối HTTP thuần và đường dẫn HTTPS trực tiếp thiếu mã băm đối chứng.
if ($explicitSource -and $BinarySource -match '^http://') {
    throw "Insecure HTTP URL is forbidden for BinarySource ($BinarySource); use https:// with -ExpectedSha256."
}
if ($explicitSource -and $BinarySource -match '^https://' -and [string]::IsNullOrWhiteSpace($ExpectedSha256)) {
    throw "-ExpectedSha256 is required when downloading sen.exe from a remote HTTPS URL ($BinarySource)."
}

if (-not (Test-Path -Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$targetExe = Join-Path $InstallDir "sen.exe"
$tempDownload = $null
$tempChecksums = $null

try {
    if (-not $explicitSource) {
        # Tải tự động checksums.txt và gói thực thi kiến trúc tương ứng từ GitHub Releases.
        $arch = if ($env:PROCESSOR_ARCHITECTURE -match 'ARM64') { "arm64" } else { "amd64" }
        $assetName = "sen-windows-$arch.exe"
        $baseUrl = "https://github.com/$Repo/releases/latest/download"
        $tempChecksums = Join-Path ([System.IO.Path]::GetTempPath()) ("sen-checksums-" + [guid]::NewGuid().ToString("N") + ".txt")
        $tempDownload = Join-Path ([System.IO.Path]::GetTempPath()) ("sen-download-" + [guid]::NewGuid().ToString("N") + ".exe")
        Write-Host "Fetching $baseUrl/checksums.txt and $assetName ..."
        Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile $tempChecksums -UseBasicParsing
        Invoke-WebRequest -Uri "$baseUrl/$assetName" -OutFile $tempDownload -UseBasicParsing
        foreach ($line in (Get-Content -LiteralPath $tempChecksums)) {
            if ($line -match "^([0-9a-fA-F]{64})\s+\*?$([regex]::Escape($assetName))$") {
                $ExpectedSha256 = $Matches[1]
                break
            }
        }
        if ([string]::IsNullOrWhiteSpace($ExpectedSha256)) {
            throw "Could not find SHA-256 entry for $assetName in checksums.txt."
        }
        $sourceFile = $tempDownload
    } elseif ($BinarySource -match '^https://') {
        $tempDownload = Join-Path ([System.IO.Path]::GetTempPath()) ("sen-download-" + [guid]::NewGuid().ToString("N") + ".exe")
        Write-Host "Downloading sen.exe from $BinarySource ..."
        Invoke-WebRequest -Uri $BinarySource -OutFile $tempDownload -UseBasicParsing
        $sourceFile = $tempDownload
    } else {
        $sourceFile = (Resolve-Path -Path $BinarySource -ErrorAction Stop).Path
        if ([string]::IsNullOrWhiteSpace($ExpectedSha256) -and (Test-Path -LiteralPath "$sourceFile.sha256")) {
            $ExpectedSha256 = (((Get-Content -LiteralPath "$sourceFile.sha256" -Raw).Trim()) -split '\s+')[0]
        }
    }

    # Kiểm chứng mã băm SHA-256 trước khi sao chép tệp vào thư mục đích.
    $actualHash = (Get-FileHash -LiteralPath $sourceFile -Algorithm SHA256).Hash.ToLowerInvariant()
    if (-not [string]::IsNullOrWhiteSpace($ExpectedSha256)) {
        $wantHash = $ExpectedSha256.Trim().ToLowerInvariant()
        if ($actualHash -ne $wantHash) {
            throw "SHA-256 verification FAILED! Expected $wantHash, got $actualHash."
        }
        Write-Host "SHA-256 verified: $actualHash"
    } else {
        Write-Host "Computed SHA-256: $actualHash"
    }

    Copy-Item -LiteralPath $sourceFile -Destination $targetExe -Force
    Unblock-File -LiteralPath $targetExe -ErrorAction SilentlyContinue

    # Bổ sung thư mục cài đặt vào biến PATH cấp độ User nếu chưa tồn tại.
    if (-not $SkipUserPath) {
        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        $entries = @()
        if (-not [string]::IsNullOrWhiteSpace($userPath)) {
            $entries = $userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
        }
        $present = $false
        foreach ($e in $entries) {
            if ($e.TrimEnd('\') -ieq $InstallDir.TrimEnd('\')) { $present = $true; break }
        }
        if (-not $present) {
            [Environment]::SetEnvironmentVariable("Path", (($entries + $InstallDir) -join ';'), "User")
            Write-Host "Added $InstallDir to User PATH."
        }
    }
    Write-Host "Installed sen.exe successfully at: $targetExe"

    # Khởi tạo hoặc cập nhật ngay không gian làm việc và gói kỹ năng nếu truyền tham số -Workspace.
    if (-not [string]::IsNullOrWhiteSpace($Workspace)) {
        if (-not (Test-Path -LiteralPath $Workspace)) {
            New-Item -ItemType Directory -Path $Workspace -Force | Out-Null
        }
        $wsPath = (Resolve-Path -LiteralPath $Workspace).Path
        $initArgs = @("init")
        if ($Update -and (Test-Path -LiteralPath (Join-Path $wsPath ".sen\state.db")) -and [string]::IsNullOrWhiteSpace($Packs)) {
            $initArgs += "--update"
        } else {
            if (-not $SkipCompanions) { $initArgs += "--install-companions" }
            if (-not [string]::IsNullOrWhiteSpace($Packs)) { $initArgs += @("--packs", $Packs) }
            if ($Update) { $initArgs += "--update" }
        }
        Push-Location -LiteralPath $wsPath
        try {
            & $targetExe @initArgs
            if ($LASTEXITCODE -ne 0) {
                throw "Workspace command 'sen $($initArgs -join ' ')' failed with exit code $LASTEXITCODE."
            }
        } finally {
            Pop-Location
        }
    }
} finally {
    if ($null -ne $tempDownload -and (Test-Path -LiteralPath $tempDownload)) { Remove-Item -LiteralPath $tempDownload -Force -ErrorAction SilentlyContinue }
    if ($null -ne $tempChecksums -and (Test-Path -LiteralPath $tempChecksums)) { Remove-Item -LiteralPath $tempChecksums -Force -ErrorAction SilentlyContinue }
}

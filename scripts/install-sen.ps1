<#
.SYNOPSIS
    Trình cài đặt `sen.exe` an toàn cho Windows (không cần quyền Quản trị viên / UAC).
.DESCRIPTION
    Cài đặt `sen.exe` vào `%LOCALAPPDATA%\Programs\sen\bin`, tự động tải và xác thực SHA-256 từ GitHub Releases (`-Repo`),
    từ chối giao thức HTTP không mã hóa, bắt buộc kiểm tra SHA-256 khi tải trực tiếp qua HTTPS,
    gỡ cảnh báo SmartScreen bằng `Unblock-File` và cập nhật biến môi trường PATH của người dùng.
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
    [switch]$SkipUserPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

# Nếu chạy từ bản clone cục bộ đã biên dịch sẵn `sen.exe` và không truyền `-Repo`, ưu tiên dùng tệp nội bộ.
$explicitSource = $PSBoundParameters.ContainsKey("BinarySource") -and (-not [string]::IsNullOrWhiteSpace($BinarySource))
if (-not $explicitSource) {
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
} finally {
    if ($null -ne $tempDownload -and (Test-Path -LiteralPath $tempDownload)) { Remove-Item -LiteralPath $tempDownload -Force -ErrorAction SilentlyContinue }
    if ($null -ne $tempChecksums -and (Test-Path -LiteralPath $tempChecksums)) { Remove-Item -LiteralPath $tempChecksums -Force -ErrorAction SilentlyContinue }
}

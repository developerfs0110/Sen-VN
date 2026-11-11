@echo off
REM Zero-Build Windows wrapper for Sen MVP (ASCII-only for cmd.exe OEM codepage safety).
REM Detailed Vietnamese documentation is in scripts\bootstrap-sen.ps1.
setlocal
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..") do set "REPO_ROOT=%%~fI"

REM Step 1: Fast-path (<2ms) - execute cached binary in PLUGIN_DATA, .cache\bin, or repo root.
if defined PLUGIN_DATA (
    if exist "%PLUGIN_DATA%\bin\sen.exe" (
        "%PLUGIN_DATA%\bin\sen.exe" %*
        exit /b %ERRORLEVEL%
    )
)
if exist "%REPO_ROOT%\.cache\bin\sen.exe" (
    "%REPO_ROOT%\.cache\bin\sen.exe" %*
    exit /b %ERRORLEVEL%
)
if exist "%REPO_ROOT%\sen.exe" (
    "%REPO_ROOT%\sen.exe" %*
    exit /b %ERRORLEVEL%
)

REM Step 2 & 3: Cold-start bootstrap (HTTPS SHA-256 GitHub Release download or one-time go build).
REM Redirect stdin from nul and stdout to stderr so piped Antigravity hook JSON stays intact.
powershell -NoProfile -ExecutionPolicy Bypass -File "%REPO_ROOT%\scripts\bootstrap-sen.ps1" -RepoRoot "%REPO_ROOT%" < nul 1>&2
if errorlevel 1 exit /b %ERRORLEVEL%

if defined PLUGIN_DATA (
    if exist "%PLUGIN_DATA%\bin\sen.exe" (
        "%PLUGIN_DATA%\bin\sen.exe" %*
        exit /b %ERRORLEVEL%
    )
)
if exist "%REPO_ROOT%\.cache\bin\sen.exe" (
    "%REPO_ROOT%\.cache\bin\sen.exe" %*
    exit /b %ERRORLEVEL%
)
exit /b 1

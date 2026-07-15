# voxRobota installer for Windows (Windows PowerShell 5.1+).
#   - installs Go, Python, and a C compiler (mingw-w64) via winget/choco
#   - creates a Python venv + downloads the Kokoro models
#   - builds voxrobota-bin.exe
#   - installs a `voxrobota` launcher on your PATH (auto-starts the sidecar)
#
# Run from the project folder:  powershell -ExecutionPolicy Bypass -File .\install.ps1
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"   # PS 5.1 downloads are ~50x faster without the progress bar
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch {}

$Here       = Split-Path -Parent $MyInvocation.MyCommand.Path
$AppDir     = Join-Path $env:LOCALAPPDATA "voxRobota"
$BinDir     = Join-Path $AppDir "bin"
$KokoroBase = "https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0"

function Log($m)  { Write-Host "[voxRobota] $m" -ForegroundColor Cyan }
function Warn($m) { Write-Host "[voxRobota] $m" -ForegroundColor Yellow }
function Die($m)  { Write-Host "[voxRobota] $m" -ForegroundColor Red; exit 1 }

function Have($cmd) { return [bool](Get-Command $cmd -ErrorAction SilentlyContinue) }

function RefreshPath {
	$machine = [Environment]::GetEnvironmentVariable("Path", "Machine")
	$user    = [Environment]::GetEnvironmentVariable("Path", "User")
	$env:Path = "$env:Path;$machine;$user"
}

# NB: must not be named 'WinGet' - a function by that name shadows winget.exe
# and the inner call would recurse forever.
function Install-WinGetPkg($id) {
	if (-not (Have winget.exe)) { return $false }
	winget.exe install -e --id $id --accept-source-agreements --accept-package-agreements --silent
	RefreshPath
	return $true
}

# Prepend a dir to the user PATH (persistent) and the current session PATH.
function Add-UserPath($dir) {
	$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
	if ($userPath -notlike "*$dir*") {
		[Environment]::SetEnvironmentVariable("Path", "$dir;$userPath", "User")
	}
	if ($env:Path -notlike "*$dir*") { $env:Path = "$dir;$env:Path" }
}

# The Microsoft Store plants a fake python.exe on PATH that just opens the
# Store. Resolve a REAL interpreter: py launcher first, then python/python3,
# skipping the WindowsApps stub and verifying it actually runs.
function Find-Python {
	foreach ($c in @("py", "python", "python3")) {
		$cmd = Get-Command $c -ErrorAction SilentlyContinue
		if (-not $cmd -or $cmd.Source -like "*\Microsoft\WindowsApps\*") { continue }
		$argv = @("-c", "import sys; print(sys.executable)")
		if ($c -eq "py") { $argv = @("-3") + $argv }
		$exe = & $cmd.Source @argv
		if ($LASTEXITCODE -eq 0 -and $exe) { return "$exe".Trim() }
	}
	return $null
}

function Find-Go {
	$cmd = Get-Command go -ErrorAction SilentlyContinue
	if ($cmd) { return $cmd.Source }
	$default = Join-Path $env:ProgramFiles "Go\bin\go.exe"
	if (Test-Path $default) { return $default }
	return $null
}

# Locate a gcc.exe dir: PATH first, then the usual mingw install spots.
function Find-GccDir {
	$cmd = Get-Command gcc -ErrorAction SilentlyContinue
	if ($cmd) { return (Split-Path -Parent $cmd.Source) }
	foreach ($d in @("C:\msys64\ucrt64\bin", "C:\msys64\mingw64\bin",
	                 "C:\ProgramData\chocolatey\bin", "C:\TDM-GCC-64\bin", "C:\mingw64\bin")) {
		if (Test-Path (Join-Path $d "gcc.exe")) { return $d }
	}
	return $null
}

function Ensure-Deps {
	Log "checking dependencies..."
	if (-not (Have winget) -and -not (Have choco)) {
		Warn "neither winget nor choco found - anything missing below must be installed by hand."
	}
	if (-not (Have claude)) {
		Warn "'claude' not found on PATH - voxRobota drives Claude Code; install it separately."
	}

	if (-not (Find-Go))     { Log "installing Go...";     Install-WinGetPkg "GoLang.Go" | Out-Null }
	if (-not (Find-Python)) { Log "installing Python..."; Install-WinGetPkg "Python.Python.3.12" | Out-Null }

	# C compiler for cgo (miniaudio). This is the trickiest Windows piece:
	# installing MSYS2 alone is NOT enough - gcc must be installed inside it
	# and its bin dir put on PATH.
	if (-not (Find-GccDir)) {
		Log "installing a C compiler (mingw-w64)..."
		if (Have choco) {
			choco install -y mingw
		} elseif (Install-WinGetPkg "MSYS2.MSYS2") {
			$bash = "C:\msys64\usr\bin\bash.exe"
			if (Test-Path $bash) {
				Log "installing gcc inside MSYS2 (pacman)..."
				& $bash -lc "pacman -S --noconfirm --needed mingw-w64-ucrt-x86_64-gcc"
			}
		} else {
			Warn "Could not auto-install a C compiler."
			Warn "Install one manually, then re-run: 'winget install MSYS2.MSYS2' or 'choco install mingw'"
		}
		RefreshPath
	}

	$script:GoExe = Find-Go
	$script:PyExe = Find-Python
	if (-not $script:GoExe) { Die "Go not found after install. Open a new PowerShell and re-run install.ps1." }
	if (-not $script:PyExe) { Die "Python not found after install. Open a new PowerShell and re-run install.ps1." }

	$gccDir = Find-GccDir
	if (-not $gccDir) { Die "gcc still not found - install mingw-w64 (see INSTALL.md troubleshooting), open a new PowerShell, and re-run install.ps1." }
	Add-UserPath $gccDir   # cgo invokes plain 'gcc', so it must be on PATH

	Log "go:     $script:GoExe"
	Log "python: $script:PyExe"
	Log "gcc:    $(Join-Path $gccDir 'gcc.exe')"
}

function Setup-AppDir {
	Log "installing into $AppDir"
	New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
	if (Test-Path (Join-Path $AppDir "sidecar")) { Remove-Item -Recurse -Force (Join-Path $AppDir "sidecar") }
	Copy-Item -Recurse (Join-Path $Here "sidecar") (Join-Path $AppDir "sidecar")

	Log "creating python venv + installing STT/TTS deps..."
	& $script:PyExe -m venv (Join-Path $AppDir ".venv")
	if ($LASTEXITCODE -ne 0) { Die "creating the Python venv failed - see output above." }
	$py = Join-Path $AppDir ".venv\Scripts\python.exe"
	& $py -m pip install --upgrade pip | Out-Null
	& $py -m pip install -r (Join-Path $AppDir "sidecar\requirements.txt")
	if ($LASTEXITCODE -ne 0) { Die "pip install failed - see output above." }

	Log "fetching Kokoro models (~340 MB, one time)..."
	foreach ($f in @("kokoro-v1.0.onnx", "voices-v1.0.bin")) {
		$dst = Join-Path $AppDir $f
		if (Test-Path $dst) { continue }
		$src = Join-Path $Here $f
		if (Test-Path $src) { Copy-Item $src $dst }
		else { Log "downloading $f..."; Invoke-WebRequest -UseBasicParsing "$KokoroBase/$f" -OutFile $dst }
	}

	Log "building voxrobota (cgo)..."
	Push-Location $Here
	$env:CGO_ENABLED = "1"
	& $script:GoExe build -o (Join-Path $BinDir "voxrobota-bin.exe") .
	$built = ($LASTEXITCODE -eq 0)
	Pop-Location
	if (-not $built) { Die "go build failed - see output above (is gcc on PATH?)." }
}

function Write-Launcher {
	$launcher = Join-Path $BinDir "voxrobota.cmd"
	# Built as an array of lines (NOT a here-string: PS 5.1 mis-parses
	# here-strings in files with unix line endings). Set-Content joins the
	# lines with CRLF, which is what cmd.exe requires.
	$startSidecar = 'powershell -NoProfile -Command "try { Invoke-WebRequest -UseBasicParsing -TimeoutSec 1 http://127.0.0.1:8123/health | Out-Null } catch { Start-Process -WindowStyle Hidden -WorkingDirectory ''%APPDIR%'' -FilePath ''%APPDIR%\.venv\Scripts\python.exe'' -ArgumentList ''%APPDIR%\sidecar\server.py'' }"'
	$lines = @(
		'@echo off',
		"set ""APPDIR=$AppDir""",
		'set "VOX_KOKORO_MODEL=%APPDIR%\kokoro-v1.0.onnx"',
		'set "VOX_KOKORO_VOICES=%APPDIR%\voices-v1.0.bin"',
		$startSidecar,
		'"%APPDIR%\bin\voxrobota-bin.exe" %*'
	)
	Set-Content -Path $launcher -Value $lines -Encoding ASCII
	Log "launcher installed: $launcher"
}

function Ensure-Path {
	$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
	if ($userPath -notlike "*$BinDir*") {
		[Environment]::SetEnvironmentVariable("Path", "$userPath;$BinDir", "User")
		Warn "added $BinDir to your PATH - open a NEW terminal for it to take effect."
	}
}

Ensure-Deps
Setup-AppDir
Write-Launcher
Ensure-Path
Log "done."
Log "open a NEW terminal (Windows Terminal recommended) in any folder and run:  voxrobota"
Warn "first launch downloads the Whisper model (~150MB) and takes ~20s before speech works."

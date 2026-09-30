# Installs the test runner for YOUR FORK on Windows.
#
#   powershell -ExecutionPolicy Bypass -File runner\install.ps1
#
# Needs Docker Desktop (WSL 2 backend). Asks for your fork name and a
# fine-grained personal token, writes them to runner\.env (never committed)
# and starts the runner. It comes back after a reboot on its own as long as
# Docker Desktop starts with Windows.
$ErrorActionPreference = "Stop"
Set-Location -Path $PSScriptRoot

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host "Docker Desktop is not installed: https://www.docker.com/products/docker-desktop/"
    Write-Host "Install it (WSL 2 backend), enable 'Start Docker Desktop when you sign in', then run this script again."
    exit 1
}
docker compose version | Out-Null
if ($LASTEXITCODE -ne 0) { throw "docker compose is not available" }

$repo = Read-Host "Your fork on GitHub, for example your-name/mindstrata"
if ($repo -notmatch '^[^/\s]+/[^/\s]+$') { throw "expected owner/repo" }

Write-Host ""
Write-Host "Create a fine-grained token: GitHub -> Settings -> Developer settings -> Fine-grained tokens."
Write-Host "Repository access: ONLY your fork. Permission: Administration -> Read and write."
Write-Host "The token stays in runner\.env on this computer and is never committed."
$secure = Read-Host "Token" -AsSecureString
$pat = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure))
if ([string]::IsNullOrWhiteSpace($pat)) { throw "empty token" }

$cpus = if ($env:RUNNER_CPUS) { $env:RUNNER_CPUS } else { "2" }
$mem = if ($env:RUNNER_MEMORY) { $env:RUNNER_MEMORY } else { "6g" }
# Plain ASCII without BOM: docker compose reads .env byte by byte.
$content = "GITHUB_REPO=$repo`nGITHUB_PAT=$pat`nRUNNER_CPUS=$cpus`nRUNNER_MEMORY=$mem`n"
[IO.File]::WriteAllText((Join-Path $PSScriptRoot ".env"), $content, (New-Object Text.UTF8Encoding($false)))
Remove-Variable pat, secure
# Only the current user may read the token.
icacls (Join-Path $PSScriptRoot ".env") /inheritance:r /grant:r "$($env:USERNAME):(R,W)" | Out-Null

docker compose up -d --build
if ($LASTEXITCODE -ne 0) { throw "docker compose up failed" }

Write-Host ""
Write-Host "The runner is starting. One last step: in your fork open"
Write-Host "Settings -> Secrets and variables -> Actions -> Variables and add MINDSTRATA_SELF_HOSTED = true"
Write-Host "Without it CI in your fork keeps running on GitHub's machines."
Write-Host "Check: docker compose -f runner\docker-compose.yml logs -f runner"

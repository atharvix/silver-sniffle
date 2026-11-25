# Runs the whole Kinjo backend on this PC and connects every phone plugged in by USB.
# Each service opens in its own window (close the window to stop it); anything already running is left alone.
# Tools live in %USERPROFILE%\dev (see README.md). Usage:  powershell -File scripts\dev-up.ps1
$ErrorActionPreference = 'Stop'
$dev = "$env:USERPROFILE\dev"
$repo = Split-Path $PSScriptRoot -Parent

function Test-Port($port) { (Test-NetConnection 127.0.0.1 -Port $port -WarningAction SilentlyContinue).TcpTestSucceeded }

function Start-Service-Window($title, $port, $command) {
    if (Test-Port $port) { Write-Host "$title already running on $port"; return }
    Start-Process powershell -ArgumentList '-NoExit', '-Command', "`$host.UI.RawUI.WindowTitle='$title'; $command"
    foreach ($i in 1..30) { if (Test-Port $port) { Write-Host "$title up on $port"; return }; Start-Sleep 1 }
    throw "$title did not start on $port"
}

Start-Service-Window 'Kinjo Postgres' 5433 "& '$dev\tools\pgsql\bin\postgres.exe' -D '$dev\pgdata' -p 5433 -c listen_addresses=localhost"
$hasDb = & "$dev\tools\pgsql\bin\psql.exe" -h localhost -p 5433 -U kinjo -d postgres -tAc "select 1 from pg_database where datname = 'kinjo'"
if ($hasDb -ne '1') { & "$dev\tools\pgsql\bin\createdb.exe" -h localhost -p 5433 -U kinjo kinjo }

# No snapshots: live positions must never reach disk.
Start-Service-Window 'Kinjo Redis' 6379 "& '$dev\tools\redis\redis-server.exe' --port 6379 --bind 127.0.0.1 --save '' --appendonly no"
Start-Service-Window 'Kinjo NATS' 4222 "& '$dev\tools\nats-server.exe' -a 127.0.0.1 -p 4222 -js -sd '$dev\natsdata'"
# SeaweedFS stands in for Cloudflare R2 (both speak S3). Quote -ip or PowerShell splits it at the dots.
Start-Service-Window 'Kinjo photos (SeaweedFS)' 8333 "& '$dev\tools\weed\weed.exe' server '-dir=$dev\weeddata' '-ip=127.0.0.1' '-ip.bind=127.0.0.1' -s3 '-s3.port=8333' '-volume.max=100'"
try { Invoke-WebRequest -Method Put http://127.0.0.1:8333/kinjo-photos -UseBasicParsing | Out-Null } # creates the bucket
catch { if ($_.Exception.Response.StatusCode.value__ -ne 409) { throw } } # 409: it already exists

$env:Path = "$dev\tools\go\bin;$env:Path"
Push-Location "$repo\server"
go build -o kinjo-server.exe .
Pop-Location
$serverEnv = "`$env:DATABASE_URL='postgres://kinjo@localhost:5433/kinjo'; `$env:ADDR='127.0.0.1:8001'; " +
    "`$env:REDIS_URL='redis://127.0.0.1:6379/0'; `$env:NATS_URL='nats://127.0.0.1:4222'; " +
    "`$env:S3_ENDPOINT='http://127.0.0.1:8333'; `$env:S3_REGION='us-east-1'; `$env:S3_BUCKET='kinjo-photos'; " +
    "`$env:S3_ACCESS_KEY='dev'; `$env:S3_SECRET_KEY='dev'; `$env:PHOTO_BASE_URL='http://localhost:8333/kinjo-photos'; "
Start-Service-Window 'Kinjo server' 8001 "$serverEnv & '$repo\server\kinjo-server.exe'"

# Every phone plugged in reaches the API (8001) and the photos (8333) through its USB cable.
$adb = "$dev\android-sdk\platform-tools\adb.exe"
$phones = & $adb devices | Select-String '\tdevice$' | ForEach-Object { ($_ -split "`t")[0] }
foreach ($p in $phones) {
    & $adb -s $p reverse tcp:8001 tcp:8001 | Out-Null
    & $adb -s $p reverse tcp:8333 tcp:8333 | Out-Null
    Write-Host "phone $p connected"
}
if (-not $phones) { Write-Host 'no phone plugged in: plug one in (USB debugging on) and run this again' }

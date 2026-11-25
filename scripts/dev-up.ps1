# Runs the whole Kinjo backend on this PC.
#   powershell -File scripts\dev-up.ps1        phones plugged in by USB reach it through the cable
#   powershell -File scripts\dev-up.ps1 -Lan   phones on this office network reach it directly, and a
#                                               test APK for them is built into dist\ (send it to anyone here)
# Each service opens in its own window (close the window to stop it); anything already running is left alone,
# except the server and photo store, which -Lan restarts so they listen on the network.
# Tools live in %USERPROFILE%\dev (see README.md).
param([switch]$Lan)
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

$bind = '127.0.0.1'
$publicHost = 'localhost'
if ($Lan) {
    $bind = '0.0.0.0'
    $publicHost = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.IPAddress -notmatch '^(127|169\.254)\.' } | Select-Object -First 1).IPAddress
    Get-Process kinjo-server, weed -ErrorAction SilentlyContinue | Stop-Process -Force
    Start-Sleep 2
    Write-Host "this PC is $publicHost on the network"
}

Start-Service-Window 'Kinjo Postgres' 5433 "& '$dev\tools\pgsql\bin\postgres.exe' -D '$dev\pgdata' -p 5433 -c listen_addresses=localhost"
$hasDb = & "$dev\tools\pgsql\bin\psql.exe" -h localhost -p 5433 -U kinjo -d postgres -tAc "select 1 from pg_database where datname = 'kinjo'"
if ($hasDb -ne '1') { & "$dev\tools\pgsql\bin\createdb.exe" -h localhost -p 5433 -U kinjo kinjo }

# No snapshots: live positions must never reach disk.
Start-Service-Window 'Kinjo Redis' 6379 "& '$dev\tools\redis\redis-server.exe' --port 6379 --bind 127.0.0.1 --save '' --appendonly no"
Start-Service-Window 'Kinjo NATS' 4222 "& '$dev\tools\nats-server.exe' -a 127.0.0.1 -p 4222 -js -sd '$dev\natsdata'"

# SeaweedFS stands in for Cloudflare R2 (both speak S3). Only the server may write (key kept outside the repo);
# anyone may read, like R2's public URL. Quote -ip or PowerShell splits it at the dots.
$s3File = "$dev\weed-s3.json"
if (-not (Test-Path $s3File)) {
    $identities = @(
        @{ name = 'kinjo'; credentials = @(@{ accessKey = [guid]::NewGuid().ToString('N'); secretKey = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N') }); actions = @('Admin', 'Read', 'Write', 'List', 'Tagging') },
        @{ name = 'anonymous'; actions = @('Read') }
    )
    @{ identities = $identities } | ConvertTo-Json -Depth 5 | Set-Content $s3File -Encoding ascii
}
$s3Key = (Get-Content $s3File -Raw | ConvertFrom-Json).identities[0].credentials[0]
Start-Service-Window 'Kinjo photos (SeaweedFS)' 8333 "& '$dev\tools\weed\weed.exe' server '-dir=$dev\weeddata' '-ip=127.0.0.1' '-ip.bind=$bind' -s3 '-s3.port=8333' '-s3.config=$s3File' '-volume.max=100'"
# Through cmd: a PowerShell pipe would prefix the text with a byte-order mark. Harmless if the bucket exists.
cmd /c "echo s3.bucket.create -name kinjo-photos| `"$dev\tools\weed\weed.exe`" shell -master=127.0.0.1:9333 >nul 2>&1"

$env:Path = "$dev\tools\go\bin;$dev\jdk17\bin;$env:Path"
Push-Location "$repo\server"
go build -o kinjo-server.exe .
Pop-Location
$serverEnv = "`$env:DATABASE_URL='postgres://kinjo@localhost:5433/kinjo'; `$env:ADDR='${bind}:8001'; " +
    "`$env:REDIS_URL='redis://127.0.0.1:6379/0'; `$env:NATS_URL='nats://127.0.0.1:4222'; " +
    "`$env:S3_ENDPOINT='http://127.0.0.1:8333'; `$env:S3_REGION='us-east-1'; `$env:S3_BUCKET='kinjo-photos'; " +
    "`$env:S3_ACCESS_KEY='$($s3Key.accessKey)'; `$env:S3_SECRET_KEY='$($s3Key.secretKey)'; " +
    "`$env:PHOTO_BASE_URL='http://${publicHost}:8333/kinjo-photos'; "
New-Item -ItemType Directory -Force "$dev\logs" | Out-Null
Start-Service-Window 'Kinjo server' 8001 "$serverEnv & '$repo\server\kinjo-server.exe' 2>&1 | Tee-Object -FilePath '$dev\logs\kinjo-server.log' -Append"

if ($Lan) {
    $env:JAVA_HOME = "$dev\jdk17"; $env:ANDROID_HOME = "$dev\android-sdk"
    Push-Location "$repo\android"
    .\gradlew.bat assembleDebug "-PkinjoApi=http://${publicHost}:8001" --no-daemon -q
    Pop-Location
    New-Item -ItemType Directory -Force "$repo\dist" | Out-Null
    $apk = "$repo\dist\kinjo-test-$($publicHost -replace '\.', '-').apk"
    Copy-Item "$repo\android\app\build\outputs\apk\debug\app-debug.apk" $apk -Force
    Write-Host "test APK for phones on this network: $apk"
    return
}

# Every phone plugged in reaches the API (8001) and the photos (8333) through its USB cable.
$adb = "$dev\android-sdk\platform-tools\adb.exe"
$phones = & $adb devices | Select-String '\tdevice$' | ForEach-Object { ($_ -split "`t")[0] }
foreach ($p in $phones) {
    & $adb -s $p reverse tcp:8001 tcp:8001 | Out-Null
    & $adb -s $p reverse tcp:8333 tcp:8333 | Out-Null
    Write-Host "phone $p connected"
}
if (-not $phones) { Write-Host 'no phone plugged in: plug one in (USB debugging on) and run this again' }

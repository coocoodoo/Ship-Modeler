[CmdletBinding()]
param(
    [ValidateSet('Connect','Launch','State','Capture','Commands','Tools','Job','Disconnect','Build')]
    [string]$Action = 'Connect',
    [string]$File,
    [string]$Json,
    [string]$Output,
    [string]$Id,
    [int]$ProcessId,
    [string]$ConfigDir,
    [switch]$Test
)
$ErrorActionPreference = 'Stop'
$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (!$ConfigDir) { $ConfigDir = $env:MODELER_CONFIG_DIR }
if (!$ConfigDir) { $ConfigDir = Join-Path ([Environment]::GetFolderPath('ApplicationData')) 'Modeler' }
$sessionDir = Join-Path $ConfigDir 'ai'

if ($Action -eq 'Build') {
    Push-Location -LiteralPath $projectRoot
    try {
        $env:PATH = 'C:\msys64\mingw64\bin;C:\Program Files\Go\bin;' + $env:PATH
        if ($Test) { & go test ./... -count=1; if ($LASTEXITCODE) { throw 'Tests failed.' } }
        & go build -ldflags '-s -w -H windowsgui -extldflags=-static' -o modeler.exe ./cmd/modeler
        if ($LASTEXITCODE) { throw 'Compilation failed. If the executable is locked, close Modeler after saving your work.' }
        Copy-Item -LiteralPath (Join-Path $projectRoot 'modeler.exe') -Destination (Join-Path $projectRoot 'modeler-release.exe')
        Get-FileHash -LiteralPath (Join-Path $projectRoot 'modeler.exe'),(Join-Path $projectRoot 'modeler-release.exe') | Select-Object Path,Hash | ConvertTo-Json
    } finally { Pop-Location }
    return
}

function Find-Sessions {
    $found = @()
    if (Test-Path -LiteralPath $sessionDir) {
        foreach ($entry in (Get-ChildItem -LiteralPath $sessionDir -Filter 'session-*.json' -File)) {
            try {
                $s = Get-Content -LiteralPath $entry.FullName -Raw | ConvertFrom-Json
                if ($ProcessId -and $s.pid -ne $ProcessId) { continue }
                $uri = [Uri]$s.url
                if ($uri.Scheme -ne 'http' -or $uri.Host -ne '127.0.0.1' -or !$s.token) { continue }
                $health = Invoke-RestMethod -Uri ($s.url + '/v1/health') -Headers @{Authorization=('Bearer ' + $s.token)} -TimeoutSec 2
                if ($health.pid -eq $s.pid -and $health.protocol -eq 1) { $found += $s }
            } catch { }
        }
    }
    return $found
}

if ($Action -eq 'Launch') {
    $exe = Join-Path $projectRoot 'modeler.exe'
    if (!(Test-Path -LiteralPath $exe)) { throw 'Build Modeler first.' }
    $previousConfig = $env:MODELER_CONFIG_DIR
    try {
        $env:MODELER_CONFIG_DIR = $ConfigDir
        $process = Start-Process -FilePath $exe -ArgumentList '-ai' -WorkingDirectory $projectRoot -WindowStyle Hidden -PassThru
        $ProcessId = $process.Id
    } finally { $env:MODELER_CONFIG_DIR = $previousConfig }
    for ($attempt = 0; $attempt -lt 100; $attempt++) {
        $sessions = @(Find-Sessions)
        if ($sessions.Count) { break }
        Start-Sleep -Milliseconds 100
    }
} else { $sessions = @(Find-Sessions) }
if (!$sessions.Count) { throw 'No enabled Modeler connection. In Modeler: Settings > AI connection: On. Or use -Action Launch to open a connected session.' }
if ($sessions.Count -gt 1) { throw ('Multiple Modeler sessions found. Choose -ProcessId: ' + (($sessions | ForEach-Object { $_.pid }) -join ', ')) }
$session = $sessions[0]
$headers = @{Authorization=('Bearer ' + $session.token)}
if ($Action -in @('Connect','Launch')) {
    @{connected=$true;pid=$session.pid;url=$session.url} | ConvertTo-Json
    return
}
if ($Action -eq 'Tools') {
    $result = Invoke-RestMethod -Uri ($session.url + '/v1/tools') -Headers $headers
} else {
    if ($Action -ne 'Job') {
        if (!$Id) { $Id = [Guid]::NewGuid().ToString('N') }
        $request = @{id=$Id;type=$Action.ToLowerInvariant()}
        if ($Action -eq 'Commands') {
            if ($File) { $Json = Get-Content -LiteralPath $File -Raw }
            if (!$Json) { throw 'Commands requires -File operations.json or -Json.' }
            $request.ops = @(ConvertFrom-Json -InputObject $Json)
        }
        $result = Invoke-RestMethod -Method Post -Uri ($session.url + '/v1/jobs') -Headers $headers -ContentType 'application/json' -Body ([Text.Encoding]::UTF8.GetBytes(($request | ConvertTo-Json -Depth 64 -Compress))) -TimeoutSec 10
    } elseif (!$Id) { throw 'Job requires -Id.' }
    $deadline = [DateTime]::UtcNow.AddMinutes(3)
    do {
        $result = Invoke-RestMethod -Uri ($session.url + '/v1/jobs/' + $Id) -Headers $headers -TimeoutSec 10
        if ($result.status -in @('done','failed','cancelled')) { break }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    if ($result.status -notin @('done','failed','cancelled')) { throw "Job $Id is still running. Inspect it with -Action Job -Id $Id; do not send the edits again with a new ID." }
}
if ($Output) {
    $target = [IO.Path]::GetFullPath($Output)
    [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target)) | Out-Null
    if ($Action -eq 'Capture' -and $result.status -eq 'done') {
        Copy-Item -LiteralPath $result.result.path -Destination $target
    } else { [IO.File]::WriteAllText($target, ($result | ConvertTo-Json -Depth 100), [Text.UTF8Encoding]::new($false)) }
    @{path=$target;job=$Id;status=$result.status} | ConvertTo-Json
} else { $result | ConvertTo-Json -Depth 100 }
if ($result.status -eq 'failed') { throw "Modeler job $Id failed: $($result.error). Review completed operations and checkpoint before continuing." }

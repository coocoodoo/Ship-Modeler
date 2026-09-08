[CmdletBinding()]
param(
    [string]$InstallDir = 'C:\Program Files\Modeler',
    [string]$LibrarySource = (Join-Path ([Environment]::GetFolderPath('ApplicationData')) 'Modeler'),
    [switch]$SkipBuild,
    [switch]$FilesOnly,
    [switch]$RegisterOnly
)
$ErrorActionPreference = 'Stop'
$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$InstallDir = [IO.Path]::GetFullPath($InstallDir)
$exe = Join-Path $InstallDir 'modeler.exe'
if (!$RegisterOnly) {
    if (!$SkipBuild) { & (Join-Path $PSScriptRoot 'modeler.ps1') -Action Build }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    foreach ($name in @('modeler.exe', 'modeler-release.exe')) {
        Copy-Item -LiteralPath (Join-Path $projectRoot $name) -Destination (Join-Path $InstallDir $name) -Force
    }
    Copy-Item -LiteralPath (Join-Path $projectRoot 'assets\modeler.ico') -Destination (Join-Path $InstallDir 'modeler.ico') -Force
    # The install carries a library snapshot. Editable user assets stay in
    # AppData, where ordinary (non-administrator) sessions can update them.
    foreach ($name in @('parts', 'tilesets')) {
        $source = Join-Path $LibrarySource $name
        if (Test-Path -LiteralPath $source) {
            $destination = Join-Path $InstallDir ('Library\' + $name)
            New-Item -ItemType Directory -Path $destination -Force | Out-Null
            foreach ($item in Get-ChildItem -LiteralPath $source) {
                Copy-Item -LiteralPath $item.FullName -Destination $destination -Recurse -Force
            }
        }
    }
    @'
Modeler

Double-click a .pxm project to view it. Use Edit in the top-right corner to
open the full workspace. Launch modeler.exe without a filename for the editor.
Viewer controls: right-drag orbit, middle-drag pan, mouse wheel zoom, F fit.

Library\parts and Library\tilesets contain the library copied at installation.
Your working library remains in %APPDATA%\Modeler so edits can be saved without
administrator privileges. Project textures, pins and PBR remain inside .pxm.
'@ | Set-Content -LiteralPath (Join-Path $InstallDir 'README.txt') -Encoding UTF8
}
if (!(Test-Path -LiteralPath $exe)) { throw "Modeler executable not found: $exe" }
if (!$FilesOnly) {
    $classes = 'HKCU:\Software\Classes'
    $progId = 'Modeler.PXM'
    $values = @{
        "$classes\.pxm" = $progId
        "$classes\$progId" = 'Modeler Project'
        "$classes\$progId\DefaultIcon" = ('"' + (Join-Path $InstallDir 'modeler.ico') + '",0')
        "$classes\$progId\shell" = 'open'
        "$classes\$progId\shell\open" = 'View in Modeler'
        "$classes\$progId\shell\open\command" = ('"' + $exe + '" -view "%1"')
        "$classes\$progId\shell\edit" = 'Edit in Modeler'
        "$classes\$progId\shell\edit\command" = ('"' + $exe + '" -edit "%1"')
    }
    foreach ($path in $values.Keys) {
        New-Item -Path $path -Force | Out-Null
        Set-Item -LiteralPath $path -Value $values[$path]
    }
    $openWith = "$classes\.pxm\OpenWithProgids"
    New-Item -Path $openWith -Force | Out-Null
    New-ItemProperty -LiteralPath $openWith -Name $progId -Value '' -PropertyType String -Force | Out-Null
    if (!('Modeler.AssociationNotify' -as [type])) {
        Add-Type 'namespace Modeler { public static class AssociationNotify { [System.Runtime.InteropServices.DllImport("shell32.dll")] public static extern void SHChangeNotify(uint e, uint f, System.IntPtr a, System.IntPtr b); } }'
    }
    [Modeler.AssociationNotify]::SHChangeNotify(0x08000000, 0, [IntPtr]::Zero, [IntPtr]::Zero)
}
Get-FileHash -LiteralPath $exe,(Join-Path $InstallDir 'modeler-release.exe') | Select-Object Path,Hash

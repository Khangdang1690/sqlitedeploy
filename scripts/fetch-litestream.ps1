# Windows-native equivalent of `make fetch-litestream`.
#
# Downloads upstream Litestream release artifacts for every platform we ship
# and copies the extracted binary into internal\litestream\bin so go:embed
# bundles it into the next sqlitedeploy build.
#
# Usage:
#   pwsh scripts\fetch-litestream.ps1
#   pwsh scripts\fetch-litestream.ps1 -Version 0.5.11
#   pwsh scripts\fetch-litestream.ps1 -Only windows-amd64       # one platform only

[CmdletBinding()]
param(
    [string]$Version = '0.5.11',
    [string]$Only = ''
)

$ErrorActionPreference = 'Stop'
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
$embedDir = Join-Path $repoRoot 'internal\litestream\bin'
New-Item -ItemType Directory -Force -Path $embedDir | Out-Null

# (go-os, go-arch, litestream-arch, archive-ext) — must match the table in Makefile.
$platforms = @(
    @{ Os='linux';   GoArch='amd64'; LsArch='x86_64'; Ext='tar.gz' },
    @{ Os='linux';   GoArch='arm64'; LsArch='arm64';  Ext='tar.gz' },
    @{ Os='darwin';  GoArch='amd64'; LsArch='x86_64'; Ext='tar.gz' },
    @{ Os='darwin';  GoArch='arm64'; LsArch='arm64';  Ext='tar.gz' },
    @{ Os='windows'; GoArch='amd64'; LsArch='x86_64'; Ext='zip'    },
    @{ Os='windows'; GoArch='arm64'; LsArch='arm64';  Ext='zip'    }
)

if ($Only -ne '') {
    $platforms = $platforms | Where-Object { "$($_.Os)-$($_.GoArch)" -eq $Only }
    if (-not $platforms) { throw "Unknown -Only target: $Only" }
}

# Windows includes tar.exe (Windows 10 17063+) which handles tar.gz natively.
$tar = Get-Command tar -ErrorAction SilentlyContinue
if (-not $tar) { throw "tar.exe not found on PATH (needed to extract .tar.gz archives)" }

foreach ($p in $platforms) {
    $url = "https://github.com/benbjohnson/litestream/releases/download/v$Version/litestream-$Version-$($p.Os)-$($p.LsArch).$($p.Ext)"
    $outName = "litestream-$($p.Os)-$($p.GoArch)"
    if ($p.Os -eq 'windows') { $outName += '.exe' }
    $outPath = Join-Path $embedDir $outName

    Write-Host "-> $url"
    $tmp = New-TemporaryFile; Remove-Item $tmp; New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        $archive = Join-Path $tmp "archive.$($p.Ext)"
        Invoke-WebRequest -Uri $url -OutFile $archive -UseBasicParsing

        if ($p.Ext -eq 'zip') {
            Expand-Archive -Path $archive -DestinationPath $tmp -Force
        } else {
            & tar.exe -xzf $archive -C $tmp
            if ($LASTEXITCODE -ne 0) { throw "tar extraction failed for $url" }
        }

        $binName = if ($p.Os -eq 'windows') { 'litestream.exe' } else { 'litestream' }
        $extracted = Get-ChildItem -Path $tmp -Recurse -Filter $binName | Select-Object -First 1
        if (-not $extracted) { throw "$binName not found in archive from $url" }
        Copy-Item -Path $extracted.FullName -Destination $outPath -Force

        $sizeMB = [math]::Round((Get-Item $outPath).Length / 1MB, 1)
        Write-Host ("  OK {0} ({1} MB)" -f $outPath, $sizeMB)
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
}

Write-Host "`nlitestream binaries cached in $embedDir"
Write-Host "Now run: go build -o dist\sqlitedeploy.exe .\cmd\sqlitedeploy"

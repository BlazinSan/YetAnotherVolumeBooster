$ErrorActionPreference = 'Stop'

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Dist = Join-Path $Root 'dist'
$Payload = Join-Path $Root 'setup\payload'
$ExpectedApoHash = '7403BE7427BBE1936A40DDED082829B6E217FC4F5990FEE5CBA501F0AE055AFA'
$ApoInstaller = Join-Path $Payload 'EqualizerAPO-x64-1.4.2.exe'

New-Item -ItemType Directory -Force -Path $Dist, $Payload | Out-Null

Push-Location $Root
try {
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'

    go build -trimpath -ldflags '-H windowsgui -s -w' -o (Join-Path $Dist 'YetAnotherVolumeBooster.exe') .\app
    Copy-Item (Join-Path $Dist 'YetAnotherVolumeBooster.exe') (Join-Path $Payload 'YetAnotherVolumeBooster.exe') -Force

    if ((Test-Path $ApoInstaller) -and ((Get-Item $ApoInstaller).Length -gt 5MB)) {
        $Hash = (Get-FileHash $ApoInstaller -Algorithm SHA256).Hash
        if ($Hash -ne $ExpectedApoHash) {
            throw "Equalizer APO payload checksum mismatch. Expected $ExpectedApoHash, got $Hash"
        }
        Write-Host 'Building fully offline setup with the verified Equalizer APO payload.'
    }
    else {
        Set-Content -Path $ApoInstaller -Value 'ONLINE_BUILD_PLACEHOLDER' -NoNewline
        Write-Host 'Building online setup. Equalizer APO will be downloaded and verified during installation.'
    }

    go build -trimpath -ldflags '-H windowsgui -s -w' -o (Join-Path $Dist 'VolumeBoostSetup.exe') .\setup
    Get-FileHash (Join-Path $Dist 'VolumeBoostSetup.exe') -Algorithm SHA256
}
finally {
    Pop-Location
}

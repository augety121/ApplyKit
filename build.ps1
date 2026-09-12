param([switch]$SkipTests, [switch]$WriteChecksums)
$ErrorActionPreference='Stop'
Push-Location $PSScriptRoot
try {
 if(-not (Get-Command go -ErrorAction SilentlyContinue)){throw 'Source builds require Go 1.23 or newer. Release users do not need Go.'}
 if(-not $SkipTests){
  & go test -count=1 ./...; if($LASTEXITCODE -ne 0){throw 'Unit tests failed.'}
  & go vet ./...; if($LASTEXITCODE -ne 0){throw 'go vet failed.'}
 }
 foreach($f in @('web/index.html','web/app.css','web/desktop.css','web/app.js','web/ui.js','web/editor.js','web/geometry.mjs','web/brand.svg','assets/render.ps1','resource_windows_amd64.syso')){if(-not (Test-Path $f)){throw ('Missing build resource: '+$f)}}
 $oldOS=$env:GOOS;$oldArch=$env:GOARCH;$oldCGO=$env:CGO_ENABLED
 try {
  $env:GOOS='windows';$env:GOARCH='amd64';$env:CGO_ENABLED='0'
  [void][IO.Directory]::CreateDirectory((Join-Path $PSScriptRoot 'dist'))
  & go build -trimpath -ldflags='-s -w -H=windowsgui' -o dist/ApplyKit.exe .
  if($LASTEXITCODE -ne 0){throw 'Windows build failed.'}
 } finally {$env:GOOS=$oldOS;$env:GOARCH=$oldArch;$env:CGO_ENABLED=$oldCGO}
 if($WriteChecksums){
 $hash=(Get-FileHash dist/ApplyKit.exe -Algorithm SHA256).Hash.ToLowerInvariant()
 [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'dist/SHA256SUMS.txt'),$hash+'  ApplyKit.exe'+[Environment]::NewLine,[Text.Encoding]::ASCII)
 }
 Write-Host ('Built: '+(Join-Path $PSScriptRoot 'dist/ApplyKit.exe'))

} finally {Pop-Location}

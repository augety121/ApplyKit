param([switch]$SkipTests)
$ErrorActionPreference='Stop'
Push-Location $PSScriptRoot
try {
 if(-not (Get-Command go -ErrorAction SilentlyContinue)){throw 'Source builds require Go 1.23 or newer. Running a release EXE does not require Go.'}
 if(-not $SkipTests){
  & go test -count=1 ./...
  if($LASTEXITCODE -ne 0){throw 'Unit tests failed.'}
  & go vet ./...
  if($LASTEXITCODE -ne 0){throw 'go vet failed.'}
 }
 $oldOS=$env:GOOS;$oldArch=$env:GOARCH;$oldCGO=$env:CGO_ENABLED
 try {
  $env:GOOS='windows';$env:GOARCH='amd64';$env:CGO_ENABLED='0'
  [void][IO.Directory]::CreateDirectory((Join-Path $PSScriptRoot 'dist'))
  & go build -trimpath -ldflags='-s -w -H=windowsgui' -o dist/ApplyKit.exe .
  if($LASTEXITCODE -ne 0){throw 'Windows build failed.'}
 } finally {$env:GOOS=$oldOS;$env:GOARCH=$oldArch;$env:CGO_ENABLED=$oldCGO}
 $hash=(Get-FileHash dist/ApplyKit.exe -Algorithm SHA256).Hash.ToLowerInvariant()
 [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'dist/SHA256SUMS.txt'),$hash+'  ApplyKit.exe'+[Environment]::NewLine,[Text.Encoding]::ASCII)
 Write-Host ('Built: '+(Join-Path $PSScriptRoot 'dist/ApplyKit.exe'))
} finally {Pop-Location}

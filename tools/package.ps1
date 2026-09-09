param([string]$OutputDirectory='dist')
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
$versionMatch=[regex]::Match([IO.File]::ReadAllText((Join-Path $root 'main.go')),'const version = "([^"]+)"')
if(-not $versionMatch.Success){throw 'Version was not found.'}
$version=$versionMatch.Groups[1].Value
if(-not [IO.Path]::IsPathRooted($OutputDirectory)){$OutputDirectory=Join-Path $root $OutputDirectory}
[void][IO.Directory]::CreateDirectory($OutputDirectory)
$stage=Join-Path $OutputDirectory 'package-stage'
if(Test-Path $stage){Remove-Item -LiteralPath $stage -Recurse -Force}
$folder=Join-Path $stage 'ApplyKit';[void][IO.Directory]::CreateDirectory($folder)
Copy-Item -LiteralPath (Join-Path $root 'dist/ApplyKit.exe') -Destination $folder
foreach($name in @('LICENSE','THIRD_PARTY_NOTICES.txt','CHANGELOG.md')){Copy-Item -LiteralPath (Join-Path $root $name) -Destination $folder}
Copy-Item -LiteralPath (Join-Path $root 'docs/USER_GUIDE.zh-CN.html') -Destination (Join-Path $folder '使用说明.html')
Copy-Item -LiteralPath (Join-Path $root 'docs/TESTING.md') -Destination (Join-Path $folder '测试范围.md')
Copy-Item -LiteralPath (Join-Path $root 'samples') -Destination (Join-Path $folder '测试样本') -Recurse
Copy-Item -LiteralPath (Join-Path $root 'tools/windows_smoke.ps1') -Destination (Join-Path $folder 'windows_smoke.ps1')
Copy-Item -LiteralPath (Join-Path $root 'tools/run_self_check.cmd') -Destination (Join-Path $folder '运行自检.cmd')
$hash=(Get-FileHash (Join-Path $folder 'ApplyKit.exe') -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText((Join-Path $folder 'SHA256SUMS.txt'),$hash+'  ApplyKit.exe'+[Environment]::NewLine,[Text.Encoding]::ASCII)
$archive=Join-Path $OutputDirectory ('ApplyKit_Windows_x64_v'+$version+'.zip')
Compress-Archive -Path $folder -DestinationPath $archive -CompressionLevel Optimal -Force
Remove-Item -LiteralPath $stage -Recurse -Force
Write-Host $archive

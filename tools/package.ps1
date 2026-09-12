param([string]$OutputDirectory='dist', [switch]$WriteChecksums)
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
$versionMatch=[regex]::Match([IO.File]::ReadAllText((Join-Path $root 'main.go')),'const version = "([^"]+)"')
if(-not $versionMatch.Success){throw 'Version was not found.'};$version=$versionMatch.Groups[1].Value
if(-not [IO.Path]::IsPathRooted($OutputDirectory)){$OutputDirectory=Join-Path $root $OutputDirectory};[void][IO.Directory]::CreateDirectory($OutputDirectory)
$stage=Join-Path $OutputDirectory ('package-stage-'+[Guid]::NewGuid().ToString('N'));$folder=Join-Path $stage 'ApplyKit';[void][IO.Directory]::CreateDirectory($folder)
Copy-Item (Join-Path $root 'dist/ApplyKit.exe') $folder
foreach($name in @('LICENSE','THIRD_PARTY_NOTICES.txt','CHANGELOG.md')){Copy-Item (Join-Path $root $name) $folder}
Copy-Item (Join-Path $root 'docs/USER_GUIDE.zh-CN.html') (Join-Path $folder '使用说明.html')
Copy-Item (Join-Path $root 'docs/windows-acceptance.json') (Join-Path $folder 'windows-acceptance.json')
Copy-Item (Join-Path $root 'docs/ACCEPTANCE.md') (Join-Path $folder '验收说明.md')
Copy-Item (Join-Path $root 'docs/app-icon-preview.png') (Join-Path $folder '新图标预览.png')
Copy-Item (Join-Path $root 'docs/ui-v22-desktop.png') (Join-Path $folder '界面布局验收.png')
Copy-Item (Join-Path $root 'samples') (Join-Path $folder '测试样本') -Recurse
Copy-Item (Join-Path $root 'tools/windows_smoke.ps1') (Join-Path $folder 'windows_smoke.ps1')
Copy-Item (Join-Path $root 'tools/run_self_check.cmd') (Join-Path $folder '运行自检.cmd')
if($WriteChecksums){$hash=(Get-FileHash (Join-Path $folder 'ApplyKit.exe') -Algorithm SHA256).Hash.ToLowerInvariant();[IO.File]::WriteAllText((Join-Path $folder 'SHA256SUMS.txt'),$hash+'  ApplyKit.exe'+[Environment]::NewLine,[Text.Encoding]::ASCII)}
$archive=Join-Path $OutputDirectory ('ApplyKit_Windows_x64_v'+$version+'.zip');Compress-Archive -Path $folder -DestinationPath $archive -CompressionLevel Optimal -Force;$resolvedStage=[IO.Path]::GetFullPath($stage);$resolvedOutput=[IO.Path]::GetFullPath($OutputDirectory).TrimEnd('\')+'\';if(-not $resolvedStage.StartsWith($resolvedOutput,[StringComparison]::OrdinalIgnoreCase)){throw 'Unsafe stage path'};Remove-Item -LiteralPath $resolvedStage -Recurse -Force;Write-Host $archive

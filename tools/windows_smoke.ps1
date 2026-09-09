param([string]$Engine='',[switch]$Portable)
$ErrorActionPreference='Stop'
if([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT){throw 'This test needs real Windows; Linux/mocked rendering does not qualify.'}
if([Threading.Thread]::CurrentThread.GetApartmentState() -ne 'STA'){throw 'Run in Windows PowerShell 5.1 with -STA.'}
if($Portable){
 $root=$PSScriptRoot
 if(-not $Engine){$Engine=Join-Path $root 'ApplyKit.exe'}
 $sample=Join-Path $root '测试样本/crop-demo.pdf'
 $base=[Environment]::GetFolderPath('Desktop');if(-not $base){$base=[IO.Path]::GetTempPath()}
 $out=Join-Path $base ('ApplyKit_自检_'+[Guid]::NewGuid().ToString('N').Substring(0,8))
}else{
 $root=Split-Path $PSScriptRoot -Parent
 if(-not $Engine){$Engine=Join-Path $root 'dist/ApplyKit.exe'}
 $sample=Join-Path $root 'samples/crop-demo.pdf'
 $out=Join-Path $root ('out/windows-smoke-'+[Guid]::NewGuid().ToString('N'))
}
$Engine=[IO.Path]::GetFullPath($Engine)
[void][IO.Directory]::CreateDirectory($out)
$checks=New-Object 'System.Collections.Generic.List[object]'
$utf8=New-Object Text.UTF8Encoding($false)
function Assert-True([bool]$condition,[string]$message){if(-not $condition){throw $message}}
function Run-Engine([string]$flag,[string]$path){
 $psi=New-Object Diagnostics.ProcessStartInfo;$psi.FileName=$Engine;$psi.Arguments=$flag+' "'+$path+'"';$psi.UseShellExecute=$false;$psi.CreateNoWindow=$true
 $p=New-Object Diagnostics.Process;$p.StartInfo=$psi
 try{[void]$p.Start();if(-not $p.WaitForExit(120000)){$p.Kill();throw 'Native worker timed out.'};if($p.ExitCode -ne 0){throw ('Native worker returned '+$p.ExitCode+'. Inspect '+$out)}}finally{$p.Dispose()}
}
function Read-Events([string]$path){
 $events=@();foreach($line in [IO.File]::ReadAllLines($path,[Text.Encoding]::UTF8)){if($line.Trim()){$events+=($line | ConvertFrom-Json)}};return $events
}
function Check-Step([string]$name,[scriptblock]$action){
 try{& $action;$checks.Add([pscustomobject]@{name=$name;status='passed'});Write-Host ('PASS '+$name)}catch{$checks.Add([pscustomobject]@{name=$name;status='failed';message=$_.Exception.GetBaseException().Message});throw}
}
function Run-Worker([string]$name,[hashtable]$extra){
 $dir=Join-Path $out $name;[void][IO.Directory]::CreateDirectory($dir)
 $progress=Join-Path $dir 'progress.jsonl'
 $job=@{mode='pdf-images';inputs=@($sample);output=$dir;format='jpg';limit=2000000;profile='balanced';dpi=200;pages='';paper='a4';strict=$false;makeZip=$false;consent=$false;progress=$progress;cancel=(Join-Path $dir 'cancel.flag');crop=@{mode='none'}}
 foreach($k in $extra.Keys){$job[$k]=$extra[$k]}
 $request=Join-Path $dir 'job.json';[IO.File]::WriteAllText($request,($job | ConvertTo-Json -Depth 16),$utf8)
 Run-Engine '--worker' $request
 $events=@(Read-Events $progress);$complete=@($events | Where-Object {$_.kind -eq 'complete'})
 Assert-True ($complete.Count -eq 1) ('No complete event for '+$name)
 Assert-True ([int]$complete[0].failed -eq 0) ('A conversion failed: '+$name+'; see '+$progress)
 return @($events | Where-Object {$_.kind -eq 'result'} | ForEach-Object {$_.record})
}
$success=$false
try{
 Assert-True ([IO.File]::Exists($Engine)) 'ApplyKit.exe is missing.'
 Assert-True ([IO.File]::Exists($sample)) 'The bundled crop-demo.pdf fixture is missing.'
 $sourceHash=(Get-FileHash -LiteralPath $sample -Algorithm SHA256).Hash.ToLowerInvariant()
 $infoPath=Join-Path $out 'assets-info.json';Run-Engine '--assets-info' $infoPath
 $assetInfo=[IO.File]::ReadAllText($infoPath,[Text.Encoding]::UTF8) | ConvertFrom-Json
 $assetPath=[string]$assetInfo.directory
 Check-Step 'Windows PowerShell parsing of embedded scripts' {
  foreach($file in (Get-ChildItem -LiteralPath $assetPath -Filter '*.ps1')){
   $tokens=$null;$parseErrors=$null;[void][System.Management.Automation.Language.Parser]::ParseFile($file.FullName,[ref]$tokens,[ref]$parseErrors)
   Assert-True ($parseErrors.Count -eq 0) ($file.Name+': '+($parseErrors | Out-String))
  }
 }
 Check-Step 'Real WPF loading and named controls' {
  Add-Type -AssemblyName PresentationFramework,PresentationCore,WindowsBase,System.Xaml,System.Windows.Forms
  foreach($name in @('ui.xaml','crop.xaml')){
   $xmlText=[IO.File]::ReadAllText((Join-Path $assetPath $name),[Text.Encoding]::UTF8)
   $xmlReader=New-Object Xml.XmlNodeReader ([xml]$xmlText);$w=[Windows.Markup.XamlReader]::Load($xmlReader)
   foreach($m in [regex]::Matches($xmlText,'x:Name="([^"]+)"')){Assert-True ($null -ne $w.FindName($m.Groups[1].Value)) ('Missing control '+$m.Groups[1].Value)}
   $w.Measure((New-Object Windows.Size(1260,880)));$w.Arrange((New-Object Windows.Rect(0,0,1260,880)));$w.UpdateLayout();$w.Close()
  }
 }
 Check-Step 'Windows.Data.Pdf to JPG, auto crop, blank preservation' {
  $script:autoResults=@(Run-Worker 'auto-crop' @{crop=@{mode='auto';threshold=250;marginMM=2}})
  Assert-True ($script:autoResults.Count -eq 3) 'Expected three pages, including a blank page.'
  Assert-True ([bool]$script:autoResults[0].crop.applied) 'Page one was not cropped.'
  Assert-True ([bool]$script:autoResults[2].crop.blank -and -not [bool]$script:autoResults[2].crop.applied) 'Blank page handling failed.'
  foreach($record in $script:autoResults){Assert-True ((Get-Item -LiteralPath $record.output).Length -lt 2000000) 'Image is over the upload limit.'}
 }
 Check-Step 'Uniform crop uses the same bounds on equal-size pages' {
  $records=@(Run-Worker 'uniform-crop' @{pages='1-2';crop=@{mode='uniform';threshold=250;marginMM=2}})
  Assert-True ($records.Count -eq 2) 'Page selection was not respected.'
  Assert-True ($records[0].crop.width -eq $records[1].crop.width -and $records[0].crop.height -eq $records[1].crop.height) 'Uniform bounds differ.'
 }
 Check-Step 'Manual normalized crop with source fingerprint' {
  $manual=@{};$manual[$sample]=@{sha256=$sourceHash;all=@{left=0.2;top=0.15;right=0.8;bottom=0.85};pages=@{}}
  $records=@(Run-Worker 'manual-crop' @{pages='1';crop=@{mode='manual';manual=$manual}})
  Assert-True ($records.Count -eq 1 -and [bool]$records[0].crop.applied) 'Manual cropping failed.'
 }
 Check-Step 'Native PDF preview and document fingerprint' {
  $dir=Join-Path $out 'crop-preview';[void][IO.Directory]::CreateDirectory($dir)
  $progress=Join-Path $dir 'progress.jsonl';$request=Join-Path $dir 'request.json'
  $job=@{input=$sample;output=(Join-Path $dir 'pages');progress=$progress;cancel=(Join-Path $dir 'cancel.flag');threshold=250;marginMM=2}
  [IO.File]::WriteAllText($request,($job | ConvertTo-Json -Depth 8),$utf8);Run-Engine '--crop-preview' $request
  $events=@(Read-Events $progress);$pages=@($events | Where-Object {$_.kind -eq 'page'});$meta=@($events | Where-Object {$_.kind -eq 'meta'})
  Assert-True ($pages.Count -eq 3) 'Preview lost pages.'
  Assert-True ($meta[0].sha256 -eq $sourceHash) 'Fingerprint mismatch.'
  Assert-True (@($events | Where-Object {$_.kind -eq 'complete'}).Count -eq 1) 'Preview did not complete.'
  foreach($page in $pages){Assert-True ([Math]::Max($page.width,$page.height) -le 1600) 'Preview exceeded the long-edge limit.'}
 }
 Check-Step 'Images to PDF preserves page count and byte limit' {
  $inputs=@($script:autoResults | ForEach-Object {$_.output})
  $records=@(Run-Worker 'merge' @{mode='images-pdf';inputs=$inputs;crop=@{mode='none'}})
  Assert-True ($records.Count -eq 1 -and $records[0].after -lt 2000000) 'Merged PDF failed its size check.'
  $merged=$records[0].output
  $verify=@(Run-Worker 'reopen-merged-pdf' @{inputs=@($merged);crop=@{mode='none'}})
  Assert-True ($verify.Count -eq 3) 'Windows renderer could not reopen all merged pages.'
 }
 Check-Step 'Original test PDF is byte-identical' {
  Assert-True ((Get-FileHash -LiteralPath $sample -Algorithm SHA256).Hash.ToLowerInvariant() -eq $sourceHash) 'Source PDF was modified.'
 }
 $success=$true
} catch {
 Write-Host ('FAIL '+$_.Exception.GetBaseException().Message) -ForegroundColor Red
} finally {
 $report=@{version='1.1.0';time=[DateTime]::UtcNow.ToString('o');windows=[Environment]::OSVersion.VersionString;powershell=$PSVersionTable.PSVersion.ToString();success=$success;checks=@($checks.ToArray());scope='Native engine and WPF loading. Mouse interactions and display appearance require a manual check.'}
 [IO.File]::WriteAllText((Join-Path $out 'self-check-report.json'),($report | ConvertTo-Json -Depth 10),$utf8)
 Write-Host ('Report: '+(Join-Path $out 'self-check-report.json'))
}
if(-not $success){exit 1}

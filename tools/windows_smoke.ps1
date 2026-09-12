param([string]$Engine='', [switch]$Portable)
$ErrorActionPreference='Stop'
if([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT){throw 'Run this check on Windows 10/11 x64.'}
$root=if($Portable){$PSScriptRoot}else{Split-Path $PSScriptRoot -Parent}
if(-not $Engine){$Engine=Join-Path $root $(if($Portable){'ApplyKit.exe'}else{'dist/ApplyKit.exe'})}
$sampleRoot=Join-Path $root $(if($Portable){'测试样本'}else{'samples'})
$base=if($Portable){[IO.Path]::GetTempPath()}else{Join-Path $root 'out'}
$out=Join-Path $base ('windows-smoke-'+[Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($out)
$Engine=[IO.Path]::GetFullPath($Engine)
$utf8=New-Object Text.UTF8Encoding($false)
$checks=New-Object 'System.Collections.Generic.List[object]'
$serverProc=$null;$success=$false;$readyInfo=$null
function Assert-True([bool]$condition,[string]$message){if(-not $condition){throw $message}}
function Check-Step([string]$name,[scriptblock]$action){
 try{& $action;$checks.Add([pscustomobject]@{name=$name;status='passed'});Write-Host ('PASS '+$name) -ForegroundColor Green}
 catch{$checks.Add([pscustomobject]@{name=$name;status='failed';message=$_.Exception.GetBaseException().Message});throw}
}
function Call-API([string]$path,[string]$method='GET',$body=$null){
 $args=@{Uri=($readyInfo.address+'/api/'+$path);Method=$method;Headers=@{Authorization=('Bearer '+$readyInfo.token)}}
 if($null -ne $body){$args.Body=$utf8.GetBytes(($body|ConvertTo-Json -Depth 20));$args.ContentType='application/json'}
 Invoke-RestMethod @args
}
function Upload-Sample([string]$path){
 $name=[Uri]::EscapeDataString([IO.Path]::GetFileName($path))
 $res=Invoke-RestMethod -Uri ($readyInfo.address+'/api/upload?name='+$name) -Method POST -Headers @{Authorization=('Bearer '+$readyInfo.token)} -ContentType 'application/octet-stream' -InFile $path
 return $res.file
}
function Run-Job([hashtable]$job){
 $job.output=Join-Path $out 'exports'
 $started=Call-API 'jobs' 'POST' $job
 $deadline=(Get-Date).AddSeconds(120)
 do{
   $data=Call-API ('jobs/'+$started.id)
   if($data.done){break}
   if((Get-Date) -gt $deadline){Call-API ('cancel/'+$started.id) 'POST' @{} | Out-Null;throw 'Job timeout'}
   Start-Sleep -Milliseconds 150
 }while($true)
 $fatal=@($data.events|Where-Object {$_.kind -eq 'fatal'})
 Assert-True ($fatal.Count -eq 0) ('Fatal: '+($fatal|ConvertTo-Json -Depth 8))
 $records=@($data.events|Where-Object {$_.kind -eq 'result'}|ForEach-Object {$_.record})
 Assert-True ($records.Count -gt 0) 'No output records.'
 foreach($rec in $records){Assert-True ($rec.status -ne 'failed') ('Job failed: '+$rec.message)}
 return $records
}
function Base-Job([string]$mode,[string[]]$inputs){
 return @{mode=$mode;inputs=$inputs;format='jpg';amount='2';unit='MB';profile='balanced';dpi=200;pages='';paper='a4';strict=$false;makeZip=$false;consent=$false;maxEdge=0;trimImages=$false;crop=@{mode='none';threshold=250;marginMM=2}}
}
try{
 Check-Step 'Executable icon and version resources' {
  Assert-True (Test-Path -LiteralPath $Engine) 'Executable missing.'
  Add-Type -AssemblyName System.Drawing
  $ico=[Drawing.Icon]::ExtractAssociatedIcon($Engine);Assert-True ($null -ne $ico) 'Embedded icon missing.';$ico.Dispose()
  $v=[Diagnostics.FileVersionInfo]::GetVersionInfo($Engine)
  Assert-True ($v.ProductVersion -eq '2.2.0') ('Wrong product version: '+$v.ProductVersion)
 }
 $ready=Join-Path $out 'ready.json';$dataPath=Join-Path $out 'data'
 Check-Step 'Start local server and load all embedded web assets' {
  $args='--serve --no-open --ready "'+$ready+'" --data "'+$dataPath+'"'
  $script:serverProc=Start-Process -FilePath $Engine -ArgumentList $args -WindowStyle Hidden -PassThru
  for($i=0;$i -lt 100 -and -not (Test-Path $ready);$i++){Start-Sleep -Milliseconds 100}
  Assert-True (Test-Path $ready) 'No server ready file.'
  $script:readyInfo=[IO.File]::ReadAllText($ready)|ConvertFrom-Json
  foreach($asset in @('','app.css','desktop.css','app.js','ui.js','editor.js','geometry.mjs','brand.svg')){
   $resp=Invoke-WebRequest -UseBasicParsing -Uri ($readyInfo.address+'/'+$asset)
   Assert-True ($resp.StatusCode -eq 200 -and $resp.RawContentLength -gt 100) ('Asset missing: '+$asset)
  }
  $config=Call-API 'config';Assert-True ($config.version -eq '2.2.0') 'Wrong API version.'
 }
 $imageSample=Join-Path $sampleRoot 'text-demo.png';$pdfSample=Join-Path $sampleRoot 'crop-demo.pdf'
 $imageOriginal=[IO.File]::ReadAllBytes($imageSample);$pdfOriginal=[IO.File]::ReadAllBytes($pdfSample)
 $imageItem=Upload-Sample $imageSample;$pdfItem=Upload-Sample $pdfSample
 Check-Step 'Native PDF preview and configurable automatic crop margins' {
  $preview=Call-API ('preview/'+$pdfItem.id) 'POST' @{page=1;password=''}
  Assert-True ($preview.total -eq 3) 'Native PDF preview page count is incorrect.'
  $a=Call-API 'autocrop' 'POST' @{id=$pdfItem.id;previewKey=$preview.key;edit=@{};marginMM=0}
  $b=Call-API 'autocrop' 'POST' @{id=$pdfItem.id;previewKey=$preview.key;edit=@{};marginMM=5}
  Assert-True ($b.rect.left -lt $a.rect.left -and $b.rect.right -gt $a.rect.right) '5 mm margin did not retain more area.'
 }
 Check-Step 'Image rotation, flip, manual crop, maximum edge and 200 KB limit' {
  $job=Base-Job 'image-compress' @($imageItem.id);$job.amount='200';$job.unit='KB';$job.maxEdge=1200
  $edit=@{sha256=$imageItem.sha256;rotation=90;flipH=$true;flipV=$false;rect=@{left=.04;top=.04;right=.96;bottom=.96};autoTrim=$false;threshold=250;padding=8}
  $job.edits=@{};$job.edits[$imageItem.id]=$edit
  $records=@(Run-Job $job);Assert-True ($records.Count -eq 1) 'Expected one image.'
  Assert-True ($records[0].after -lt 200000 -and $records[0].width -le 1200 -and $records[0].height -le 1200) 'Image size or edge limit failed.'
 }
 Check-Step 'PDF pages to JPG with automatic crop and blank-page protection' {
  $job=Base-Job 'pdf-images' @($pdfItem.id);$job.crop.mode='auto';$job.crop.marginMM=5
  $script:pdfRecords=@(Run-Job $job)
  Assert-True ($pdfRecords.Count -eq 3) 'Expected three page results.'
  Assert-True ($pdfRecords[0].crop.applied -and $pdfRecords[2].crop.blank) 'Auto crop or blank-page protection failed.'
  foreach($rec in $pdfRecords){Assert-True ($rec.after -lt 2000000) 'JPG exceeds size limit.'}
 }
 Check-Step 'Manual PDF crop and page-range export' {
  $job=Base-Job 'pdf-images' @($pdfItem.id);$job.pages='2';$job.crop.mode='manual'
  $job.crop.manual=@{};$job.crop.manual[$pdfItem.id]=@{sha256=$pdfItem.sha256;all=@{left=.1;top=.1;right=.9;bottom=.9};pages=@{}}
  $records=@(Run-Job $job);Assert-True ($records.Count -eq 1 -and $records[0].page -eq 2 -and $records[0].crop.applied) 'Manual crop or page selection failed.'
 }
 Check-Step 'Merge images to PDF, then compress the generated scan PDF' {
  $one=Upload-Sample $pdfRecords[0].output;$two=Upload-Sample $pdfRecords[1].output
  $job=Base-Job 'images-pdf' @($one.id,$two.id);$merged=@(Run-Job $job)
  Assert-True ($merged.Count -eq 1 -and $merged[0].after -lt 2000000) 'Merged PDF limit failed.'
  $mergedFile=Upload-Sample $merged[0].output
  $job=Base-Job 'pdf-compress' @($mergedFile.id);$job.consent=$true;$job.amount='500';$job.unit='KB'
  $compressed=@(Run-Job $job);Assert-True ($compressed[0].after -lt 500000 -and $compressed[0].after -le $merged[0].after) 'PDF compression increased size or exceeded limit.'
 }
 Check-Step 'Invalid size is rejected before starting a job' {
  $job=Base-Job 'pdf-images' @($pdfItem.id);$job.amount='0'
  $rejected=$false
  try{Call-API 'jobs' 'POST' $job | Out-Null}catch{$rejected=($_.Exception.Response.StatusCode.value__ -eq 400)}
  Assert-True $rejected 'Invalid size was not rejected.'
 }
 Check-Step 'Original samples remain byte-for-byte unchanged' {
  Assert-True ([Convert]::ToBase64String($imageOriginal) -ceq [Convert]::ToBase64String([IO.File]::ReadAllBytes($imageSample))) 'Image sample changed.'
  Assert-True ([Convert]::ToBase64String($pdfOriginal) -ceq [Convert]::ToBase64String([IO.File]::ReadAllBytes($pdfSample))) 'PDF sample changed.'
 }
 $success=$true
}finally{
 if($serverProc){Stop-Process -Id $serverProc.Id -Force -ErrorAction SilentlyContinue}
 # This check owns only its unique session directory; exported diagnostics stay available.
 if(Test-Path -LiteralPath (Join-Path $out 'ready.json')){Remove-Item -LiteralPath (Join-Path $out 'ready.json') -Force}
 $report=[pscustomobject]@{app='ApplyKit';version='2.2.0';time=(Get-Date).ToString('o');success=$success;windows=[Environment]::OSVersion.VersionString;checks=$checks}
 [IO.File]::WriteAllText((Join-Path $out 'acceptance.json'),($report|ConvertTo-Json -Depth 10),$utf8)
 Write-Host ('Acceptance report: '+(Join-Path $out 'acceptance.json'))
}
if(-not $success){exit 1}

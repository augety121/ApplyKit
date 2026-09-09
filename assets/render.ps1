param(
 [Parameter(Mandatory=$true)][string]$InputPath,
 [Parameter(Mandatory=$true)][string]$OutputDir,
 [int]$Dpi=200,
 [AllowEmptyString()][string]$Pages='',
 [AllowEmptyString()][string]$CancelPath='',
 [ValidateRange(400,12000)][int]$MaxLongEdge=12000
)
$ErrorActionPreference='Stop'
$script:WireWriter=[IO.StreamWriter]::new([Console]::OpenStandardOutput(),[Text.UTF8Encoding]::new($false))
$script:WireWriter.AutoFlush=$true
function Emit([hashtable]$Value) { $script:WireWriter.WriteLine(($Value | ConvertTo-Json -Compress -Depth 5)) }
function Check-Cancel {
 if ($CancelPath -and [IO.File]::Exists($CancelPath)) { throw '用户取消了任务。' }
}
try {
 Add-Type -AssemblyName System.Runtime.WindowsRuntime
 [void][Windows.Storage.StorageFile,Windows.Storage,ContentType=WindowsRuntime]
 [void][Windows.Data.Pdf.PdfDocument,Windows.Data.Pdf,ContentType=WindowsRuntime]
 [void][Windows.Data.Pdf.PdfPageRenderOptions,Windows.Data.Pdf,ContentType=WindowsRuntime]
 [void][Windows.Storage.Streams.InMemoryRandomAccessStream,Windows.Storage.Streams,ContentType=WindowsRuntime]
 [void][Windows.Storage.Streams.DataReader,Windows.Storage.Streams,ContentType=WindowsRuntime]
 $script:AsTaskGeneric = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
  $_.Name -eq 'AsTask' -and $_.IsGenericMethod -and $_.GetGenericArguments().Count -eq 1 -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
 } | Select-Object -First 1
 $script:AsTaskAction = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
  $_.Name -eq 'AsTask' -and -not $_.IsGenericMethod -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncAction'
 } | Select-Object -First 1
 if (-not $script:AsTaskGeneric -or -not $script:AsTaskAction) { throw '系统缺少可用的 Windows Runtime 任务适配器。' }
 function Wait-Task($Task) {
  $watch=[Diagnostics.Stopwatch]::StartNew()
  while (-not $Task.IsCompleted) {
   Check-Cancel
   if ($watch.Elapsed.TotalSeconds -gt 90) { throw '单页渲染或读取超过安全等待上限；请检查 PDF 是否损坏或过于复杂。' }
   Start-Sleep -Milliseconds 40
  }
  if ($Task.IsCanceled) { throw 'Windows PDF 任务已取消。' }
  if ($Task.IsFaulted) { throw $Task.Exception.GetBaseException() }
 }
 function Await-Result($Operation,[Type]$ResultType) {
  $method=$script:AsTaskGeneric.MakeGenericMethod($ResultType)
  $task=$method.Invoke($null,@($Operation))
  Wait-Task $task
  return $task.Result
 }
 function Await-Action($Operation) {
  $task=$script:AsTaskAction.Invoke($null,@($Operation))
  Wait-Task $task
 }
 Check-Cancel
 $file=Await-Result ([Windows.Storage.StorageFile]::GetFileFromPathAsync([IO.Path]::GetFullPath($InputPath))) ([Windows.Storage.StorageFile])
 $password=[Environment]::GetEnvironmentVariable('APPLYKIT_PDF_PASSWORD','Process')
 [Environment]::SetEnvironmentVariable('APPLYKIT_PDF_PASSWORD',$null,'Process')
 try {
  if ([string]::IsNullOrEmpty($password)) {
   $doc=Await-Result ([Windows.Data.Pdf.PdfDocument]::LoadFromFileAsync($file)) ([Windows.Data.Pdf.PdfDocument])
  } else {
   $doc=Await-Result ([Windows.Data.Pdf.PdfDocument]::LoadFromFileAsync($file,$password)) ([Windows.Data.Pdf.PdfDocument])
  }
 } catch { throw ('无法打开 PDF。请检查文件是否完整，以及是否需要在界面填写正确的打开密码。系统信息：'+$_.Exception.GetBaseException().Message) }
 finally { $password=$null }
 $total=[int]$doc.PageCount
 if ($total -lt 1 -or $total -gt 200) { throw '每个 PDF 支持 1～200 页；请先拆分超大材料。' }
 $selected=New-Object 'System.Collections.Generic.List[int]'
 $seen=New-Object 'System.Collections.Generic.HashSet[int]'
 $range=$Pages.Trim().Replace('，',',').Replace('－','-')
 if ([string]::IsNullOrWhiteSpace($range)) {
  for($i=1;$i -le $total;$i++){ $selected.Add($i) }
 } else {
  foreach($piece in $range.Split(',')) {
   if ($piece.Trim() -notmatch '^(\d+)(?:\s*-\s*(\d+))?$') { throw ('页码格式无效：'+$piece+'。示例：1,3-5') }
   $start=[int]$Matches[1];$end=$start
   if ($Matches[2]) { $end=[int]$Matches[2] }
   if($start -lt 1 -or $end -lt $start -or $end -gt $total){throw ('页码超出范围：'+$piece+'；此 PDF 共 '+$total+' 页。')}
   for($i=$start;$i -le $end;$i++){ if($seen.Add($i)){ $selected.Add($i) } }
  }
 }
 Emit @{kind='meta';total=$total;selected=$selected.Count}
 [void][IO.Directory]::CreateDirectory($OutputDir)
 foreach($number in $selected) {
  Check-Cancel
  $page=$null;$stream=$null;$reader=$null;$inputStream=$null
  try {
   $page=$doc.GetPage([uint32]($number-1))
   $pw=[double]$page.Size.Width;$ph=[double]$page.Size.Height
   if ($pw -le 0 -or $ph -le 0) { throw ('第 '+$number+' 页尺寸无效。') }
   # WinRT page.Size is in DIPs (96/in). PDF MediaBox uses points (72/in).
   $width=[Math]::Max(1,[Math]::Round($pw*$Dpi/96.0))
   $height=[Math]::Max(1,[Math]::Round($ph*$Dpi/96.0))
   if ($width*$height -gt 25000000) {
    $factor=[Math]::Sqrt(25000000.0/($width*$height))
    $width=[Math]::Max(1,[Math]::Floor($width*$factor));$height=[Math]::Max(1,[Math]::Floor($height*$factor))
   }
   if ([Math]::Max($width,$height) -gt $MaxLongEdge) {
    $factor=[double]$MaxLongEdge/[Math]::Max($width,$height)
    $width=[Math]::Max(1,[Math]::Floor($width*$factor));$height=[Math]::Max(1,[Math]::Floor($height*$factor))
   }
   $options=New-Object Windows.Data.Pdf.PdfPageRenderOptions
   $options.DestinationWidth=[uint32]$width;$options.DestinationHeight=[uint32]$height
   # Built-in renderer defaults to PNG. A white background avoids transparency surprises.
   [void][Windows.UI.Colors,Windows.UI,ContentType=WindowsRuntime]
   $options.BackgroundColor=[Windows.UI.Colors]::White
   $stream=New-Object Windows.Storage.Streams.InMemoryRandomAccessStream
   Await-Action ($page.RenderToStreamAsync($stream,$options))
   if ($stream.Size -gt 268435456) { throw '单页渲染结果超过安全上限。' }
   $inputStream=$stream.GetInputStreamAt(0)
   $reader=New-Object Windows.Storage.Streams.DataReader($inputStream)
   [void](Await-Result ($reader.LoadAsync([uint32]$stream.Size)) ([uint32]))
   $bytes=New-Object byte[] ([int]$stream.Size)
   $reader.ReadBytes($bytes)
   $path=[IO.Path]::Combine($OutputDir,('page-{0:D4}.png' -f $number))
   [IO.File]::WriteAllBytes($path,$bytes)
   $bytes=$null
   Emit @{kind='page';page=$number;path=$path;width=[int]$width;height=[int]$height;pageWidth=($pw*0.75);pageHeight=($ph*0.75)}
  } finally {
   if($reader){$reader.Dispose()};if($inputStream){$inputStream.Dispose()};if($stream){$stream.Dispose()};if($page){$page.Dispose()}
  }
 }
 $doc=$null
 Emit @{kind='done'}
 exit 0
} catch {
 Emit @{kind='error';message=$_.Exception.GetBaseException().Message}
 exit 1
}

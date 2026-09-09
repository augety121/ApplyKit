param([Parameter(Mandatory=$true)][string]$Engine,[Parameter(Mandatory=$true)][string]$Assets)
$ErrorActionPreference='Stop'
try {
 Add-Type -AssemblyName PresentationFramework,PresentationCore,WindowsBase,System.Xaml,System.Windows.Forms
 . ([IO.Path]::Combine($Assets,'crop-ui.ps1'))
 $script:XamlText=[IO.File]::ReadAllText([IO.Path]::Combine($Assets,'ui.xaml'),[Text.Encoding]::UTF8)
 $reader=New-Object System.Xml.XmlNodeReader ([xml]$script:XamlText)
 $script:Window=[Windows.Markup.XamlReader]::Load($reader)
 $script:C=@{}
 foreach($match in [regex]::Matches($script:XamlText,'x:Name="([^"]+)"')) { $name=$match.Groups[1].Value;$control=$Window.FindName($name);if($control){$script:C[$name]=$control} }
 $iconPath=[IO.Path]::Combine($Assets,'app.ico')
 if([IO.File]::Exists($iconPath)){try{$Window.Icon=[Windows.Media.Imaging.BitmapFrame]::Create([uri]$iconPath)}catch{}}
 $script:Queue=New-Object 'System.Collections.ObjectModel.ObservableCollection[object]'
 $script:Results=New-Object 'System.Collections.ObjectModel.ObservableCollection[object]'
 $C.InputList.ItemsSource=$Queue;$C.ResultList.ItemsSource=$Results
 $script:CropPlans=@{}
 $script:Mode='pdf-images';$script:Busy=$false;$script:LastOutput='';$script:Process=$null;$script:JobDir='';$script:JobDirs=New-Object 'System.Collections.Generic.List[string]'
 $script:Seen=0;$script:FinishedEvent=$false;$script:CloseAfter=$false;$script:ActiveMode='';$script:ProgressPath='';$script:CancelPath=''
 $desktop=[Environment]::GetFolderPath('Desktop');if(-not $desktop){$desktop=[Environment]::GetFolderPath('MyDocuments')}
 $C.OutputBox.Text=[IO.Path]::Combine($desktop,'投递材料输出')
 $work=[Windows.SystemParameters]::WorkArea
 $Window.Width=[Math]::Min($Window.Width,$work.Width-28);$Window.Height=[Math]::Min($Window.Height,$work.Height-28)
 $Window.MinWidth=[Math]::Min($Window.MinWidth,$Window.Width);$Window.MinHeight=[Math]::Min($Window.MinHeight,$Window.Height)
 function Update-CropSummary {
  $count=0;foreach($item in $Queue){if($script:CropPlans.ContainsKey($item.Path)){$count++}}
  if($count -gt 0){$C.CropSummary.Text='已保存 '+$count+' 份 PDF 的手动方案'}else{$C.CropSummary.Text='尚无手动裁剪方案'}
 }
 function Get-CropSettings {
  if($script:Mode -ne 'pdf-images'){return @{mode='none'}}
  [double]$margin=0
  if(-not [double]::TryParse($C.CropMarginBox.Text,[Globalization.NumberStyles]::Float,[Globalization.CultureInfo]::InvariantCulture,[ref]$margin) -or [double]::IsNaN($margin) -or [double]::IsInfinity($margin) -or $margin -lt 0 -or $margin -gt 20){throw '裁剪安全边距请输入 0～20 的数字（单位 mm）。'}
  $mode=[string]$C.CropModeBox.SelectedItem.Tag
  $manual=@{}
  if($mode -eq 'manual'){
   foreach($item in $Queue){
    if(-not $script:CropPlans.ContainsKey($item.Path)){throw ('尚未为 '+$item.Name+' 设置手动方案。请选中该 PDF，打开裁剪预览并保存；或改用自动去白边。')}
    $manual[$item.Path]=$script:CropPlans[$item.Path]
   }
  }
  return @{mode=$mode;threshold=[int]$C.CropThresholdBox.SelectedItem.Tag;marginMM=$margin;manual=$manual}
 }
 function Message([string]$text,[string]$title='投递材料助手') {[void][Windows.MessageBox]::Show($Window,$text,$title,'OK','Information')}
 function Show-Control($control,[bool]$visible) {if($visible){$control.Visibility='Visible'}else{$control.Visibility='Collapsed'}}
 function Size-Text([long]$size) {if($size -ge 1000000){return ('{0:N2} MB' -f ($size/1000000.0))};return ('{0:N1} KB' -f ($size/1000.0))}
 function Refresh-Queue {
  [long]$total=0
  for($i=0;$i -lt $Queue.Count;$i++){$Queue[$i].Index=$i+1;$total+=$Queue[$i].Size}
  $C.InputList.Items.Refresh();$C.QueueTitle.Text='待处理材料  ·  '+$Queue.Count+' 项';$C.TotalSize.Text='合计 '+(Size-Text $total)
  Show-Control $C.EmptyOverlay ($Queue.Count -eq 0)
  Update-CropSummary
 }
 function Is-Compatible([string]$path) {
  $ext=[IO.Path]::GetExtension($path).ToLowerInvariant()
  if($script:Mode -in @('pdf-images','pdf-compress')){return $ext -eq '.pdf'}
  return $ext -in @('.jpg','.jpeg','.png','.gif')
 }
 function Add-Files($paths) {
  if($script:Busy){return}
  $skipped=0
  foreach($p in $paths){
   try{
    if($Queue.Count -ge 200){$skipped++;continue}
    if(-not (Is-Compatible $p) -or -not [IO.File]::Exists($p)){$skipped++;continue}
    $file=New-Object IO.FileInfo($p)
    if(@($Queue | Where-Object {$_.Path -eq $file.FullName}).Count -gt 0){continue}
    $Queue.Add([pscustomobject]@{Index=0;Name=$file.Name;Path=$file.FullName;Size=[long]$file.Length;SizeText=(Size-Text $file.Length);Type=$file.Extension.TrimStart('.').ToUpperInvariant()})
   }catch{$skipped++}
  }
  Refresh-Queue;$C.WorkspaceTabs.SelectedIndex=0
  if($skipped -gt 0){$C.StatusText.Text='已跳过 '+$skipped+' 项：格式不匹配、路径不可用，或超过每批 200 项。'}else{$C.StatusText.Text='已添加 '+$Queue.Count+' 项。检查设置后即可开始处理。'}
 }
 function Update-Limit {
  $tag=[string]$C.LimitBox.SelectedItem.Tag
  Show-Control $C.CustomLimit ($tag -eq 'custom')
  if($tag -eq 'custom'){$C.LimitHint.Text='请输入整数 KB；1 KB = 1000 字节。导出目标为上限的 95%。';$C.LimitBadge.Text='自定义大小上限'}else{
   [long]$n=[long]$tag;$C.LimitHint.Text='严格小于 '+$n.ToString('N0')+' 字节；目标 '+(Size-Text ([long]($n*0.95)))+'。'
   $unit='每张';if($script:Mode -in @('images-pdf','pdf-compress')){$unit='每份 PDF'}
   $C.LimitBadge.Text=$unit+' < '+(Size-Text $n)
  }
 }
 function Switch-Mode([string]$newMode) {
  if($script:Busy){return}
  $script:Mode=$newMode
  $pdf=$newMode -in @('pdf-images','pdf-compress');$imagesOut=$newMode -in @('pdf-images','image-compress');$compression=$newMode -in @('image-compress','pdf-compress')
  Show-Control $C.CropPanel ($newMode -eq 'pdf-images')
  Show-Control $C.FormatPanel $imagesOut;Show-Control $C.DpiPanel $pdf;Show-Control $C.PagesPanel ($newMode -eq 'pdf-images');Show-Control $C.PasswordPanel $pdf;Show-Control $C.PaperPanel ($newMode -eq 'images-pdf')
  Show-Control $C.StrictBox (-not $compression);Show-Control $C.StrictHint (-not $compression);Show-Control $C.CompressionNotice $compression;Show-Control $C.PdfWarning ($newMode -eq 'pdf-compress')
  if($imagesOut){$C.LimitLabel.Text='每张图片大小上限'}else{$C.LimitLabel.Text='每份 PDF 的总大小上限'}
  switch($newMode){
   'pdf-images'{$C.ModeTitle.Text='PDF 转图片 · 可裁剪白边';$C.ModeDescription.Text='可先自动去白边或手动框选，再按大小限制逐页导出。';$C.DropHint.Text='支持批量添加 PDF 文件';$C.StrictHint.Text='可选：每份 PDF 导出图片的合计小于该 PDF。文字型 PDF 很小，启用后可能无法导出。'}
   'image-compress'{$C.ModeTitle.Text='图片压缩';$C.ModeDescription.Text='先保留尺寸，再逐步缩小；压缩没有收益时保留原件。';$C.DropHint.Text='支持 JPG、JPEG、PNG、静态 GIF'}
   'images-pdf'{$C.ModeTitle.Text='图片合并 PDF';$C.ModeDescription.Text='把多张材料按顺序整理成一份 PDF，并校验最终总大小。';$C.DropHint.Text='支持 JPG、JPEG、PNG、静态 GIF';$C.StrictHint.Text='可选：生成的 PDF 必须小于所有输入图片的合计；目标过小时会停止导出。'}
   'pdf-compress'{$C.ModeTitle.Text='扫描版 PDF 压缩';$C.ModeDescription.Text='适合扫描件；逐页重建更小的 PDF，务必阅读右侧风险提示。';$C.DropHint.Text='支持批量添加 PDF；保留全部页'}
  }
  $removed=0
  for($i=$Queue.Count-1;$i -ge 0;$i--){if(-not (Is-Compatible $Queue[$i].Path)){$Queue.RemoveAt($i);$removed++}}
  Refresh-Queue;Update-Limit;$C.WorkspaceTabs.SelectedIndex=0
  if($removed -gt 0){$C.StatusText.Text='已从队列移除 '+$removed+' 个不适用于当前模式的文件，原文件未更改。'}
 }
 function Set-Busy([bool]$value) {
  $script:Busy=$value
  foreach($n in @('AddButton','RemoveButton','UpButton','DownButton','ClearButton','StartButton','NavPdfImages','NavImageCompress','NavImagesPdf','NavPdfCompress','SettingsPanel')){$C[$n].IsEnabled=(-not $value)}
  $C.CancelButton.IsEnabled=$value
 }
 function Selected-Path {
  if($C.WorkspaceTabs.SelectedIndex -eq 1){if($C.ResultList.SelectedItem -and $C.ResultList.SelectedItem.Path){return [string]$C.ResultList.SelectedItem.Path};return ''}
  if($C.InputList.SelectedItem){return [string]$C.InputList.SelectedItem.Path};return ''
 }
 function Preview-Selected {
  $path=Selected-Path
  if(-not $path -or -not [IO.File]::Exists($path)){Message '请先选择一个有效的输入文件或已导出的结果。';return}
  if([IO.Path]::GetExtension($path).ToLowerInvariant() -eq '.pdf'){try{Start-Process -FilePath $path}catch{Message ('无法调用 PDF 阅读器：'+$_.Exception.Message)};return}
  try{
   [xml]$previewXaml=@'
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation" xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml" Width="1000" Height="780" MinWidth="620" MinHeight="440" WindowStartupLocation="CenterOwner" Background="#E9EDF2" FontFamily="Microsoft YaHei UI">
 <Grid><Grid.RowDefinitions><RowDefinition Height="54"/><RowDefinition Height="*"/></Grid.RowDefinitions><Border Background="White" Padding="16,10"><DockPanel><TextBlock Text="缩放" VerticalAlignment="Center" Margin="0,0,12,0"/><Slider x:Name="Zoom" Width="260" Minimum="10" Maximum="200" Value="50" VerticalAlignment="Center"/><TextBlock x:Name="ZoomLabel" Text="50%" VerticalAlignment="Center" Margin="12,0,0,0"/><TextBlock Text="请核对小字、证书编号和印章；透明区域为白底。" Foreground="#6F8196" VerticalAlignment="Center" Margin="24,0,0,0" FontSize="11"/></DockPanel></Border><ScrollViewer Grid.Row="1" HorizontalScrollBarVisibility="Auto" VerticalScrollBarVisibility="Auto" Padding="20"><Image x:Name="Picture" Stretch="None" HorizontalAlignment="Center" VerticalAlignment="Top"/></ScrollViewer></Grid>
</Window>
'@
   $pr=New-Object Xml.XmlNodeReader($previewXaml);$pw=[Windows.Markup.XamlReader]::Load($pr);$pw.Owner=$Window;$pw.Title='材料预览 · '+[IO.Path]::GetFileName($path)
   $pic=$pw.FindName('Picture');$zoom=$pw.FindName('Zoom');$label=$pw.FindName('ZoomLabel')
   $bmp=New-Object Windows.Media.Imaging.BitmapImage;$bmp.BeginInit();$bmp.CacheOption=[Windows.Media.Imaging.BitmapCacheOption]::OnLoad;$bmp.DecodePixelWidth=2600;$bmp.UriSource=New-Object Uri($path);$bmp.EndInit();$bmp.Freeze();$pic.Source=$bmp
   $transform=New-Object Windows.Media.ScaleTransform(0.5,0.5);$pic.LayoutTransform=$transform
   $zoom.Add_ValueChanged(({param($sender,$eventArgs) $transform.ScaleX=$sender.Value/100;$transform.ScaleY=$sender.Value/100;$label.Text=([int]$sender.Value).ToString()+'%'}).GetNewClosure())
   [void]$pw.ShowDialog()
  }catch{Message ('预览失败：'+$_.Exception.GetBaseException().Message)}
 }
 function Add-Result($rec) {
  $state='未导出';switch([string]$rec.status){'saved'{$state='已导出'}'unchanged'{$state='保留原件'}'failed'{$state='失败'}}
  $name=[IO.Path]::GetFileName([string]$rec.output);if(-not $name){$name=[IO.Path]::GetFileName([string]$rec.input)}
  [long]$before=$rec.before;[long]$after=$rec.after;$change='—'
  if($script:ActiveMode -eq 'pdf-images'){$change='格式转换'}elseif($before -gt 0 -and $after -gt 0){$pct=100.0*($after-$before)/$before;if($pct -le 0){$change=('{0:N1}%' -f $pct)}else{$change=('＋{0:N1}%' -f $pct)}}
  $bt='—';$at='—';if($before -gt 0){$bt=Size-Text $before};if($after -gt 0){$at=Size-Text $after}
  $Results.Add([pscustomobject]@{Name=$name;Path=[string]$rec.output;StatusText=$state;BeforeText=$bt;AfterText=$at;ChangeText=$change;Detail=[string]$rec.message;Bytes=$after;Width=$rec.width;Height=$rec.height})
  if($Results.Count -gt 0){$C.ResultList.SelectedIndex=$Results.Count-1;$C.ResultList.ScrollIntoView($C.ResultList.SelectedItem)}
  if($rec.page){$C.StatusText.Text='第 '+$rec.page+' 页：'+$state}
 }
 function Read-Events {
  if(-not $script:ProgressPath -or -not [IO.File]::Exists($script:ProgressPath)){return}
  $fs=$null;$sr=$null
  try{
   $fs=[IO.File]::Open($script:ProgressPath,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::ReadWrite)
   $sr=New-Object IO.StreamReader($fs,[Text.Encoding]::UTF8,$true);$text=$sr.ReadToEnd();$lines=$text.Split([char]10)
  }catch{return}finally{if($sr){$sr.Dispose()}elseif($fs){$fs.Dispose()}}
  while($script:Seen -lt ($lines.Length-1)){
   $line=$lines[$script:Seen].Trim();if(-not $line){$script:Seen++;continue}
   try{$ev=$line | ConvertFrom-Json}catch{break};$script:Seen++
   switch([string]$ev.kind){
    'status'{if($ev.message){$C.StatusText.Text=$ev.message};if($ev.output){$script:LastOutput=[string]$ev.output}}
    'progress'{if([int]$ev.total -gt 0){$C.ProgressBar.Value=[Math]::Min(98,100.0*[int]$ev.current/[int]$ev.total)};$C.StatusText.Text=[string]$ev.message}
    'result'{Add-Result $ev.record}
    'fatal'{$C.StatusText.Text='任务未完成：'+[string]$ev.message;$C.DetailText.Text=[string]$ev.message}
    'complete'{
     $script:FinishedEvent=$true;$script:LastOutput=[string]$ev.output;$C.ProgressBar.Value=100
     $text='已导出 '+[int]$ev.success+' 项 · 保留原件 '+[int]$ev.kept+' 项 · 失败 '+[int]$ev.failed+' 项'
     if($ev.canceled){$text='已停止，保留已完成结果。'+$text}
     $C.StatusText.Text=$text
    }
   }
  }
 }
 $script:Timer=New-Object Windows.Threading.DispatcherTimer;$Timer.Interval=[TimeSpan]::FromMilliseconds(300)
 $Timer.Add_Tick({
  try{
   Read-Events
   if($script:Process -and $script:Process.HasExited){
    Read-Events;$Timer.Stop();Set-Busy $false;$C.OpenOutputButton.IsEnabled=($script:LastOutput -and [IO.Directory]::Exists($script:LastOutput))
    if(-not $script:FinishedEvent){$C.StatusText.Text='任务异常结束；请检查结果说明和输出目录。';if($Results.Count -eq 0){$C.DetailText.Text='工作进程未正常完成。可能受到 Windows 安全策略限制，或无法访问输出目录。无需关闭安全软件；请查看随包说明及 startup.log。'}}
    $script:Process.Dispose();$script:Process=$null
    if($script:CloseAfter){$Window.Close()}
   }
  }catch{$Timer.Stop();Set-Busy $false;$C.StatusText.Text='界面读取任务状态时出错：'+$_.Exception.Message}
 })
 $C.AddButton.Add_Click({
  $dlg=New-Object Microsoft.Win32.OpenFileDialog;$dlg.Multiselect=$true
  if($script:Mode -in @('pdf-images','pdf-compress')){$dlg.Filter='PDF 文件 (*.pdf)|*.pdf'}else{$dlg.Filter='图片 (*.jpg;*.jpeg;*.png;*.gif)|*.jpg;*.jpeg;*.png;*.gif'}
  if($dlg.ShowDialog($Window)){Add-Files $dlg.FileNames}
 })
 $dropHandler={param($sender,$ev) if($ev.Data.GetDataPresent([Windows.DataFormats]::FileDrop)){$ev.Handled=$true;Add-Files ($ev.Data.GetData([Windows.DataFormats]::FileDrop))}}
 $C.DropZone.Add_Drop($dropHandler)
 $C.DropZone.Add_PreviewDragOver({param($sender,$ev) $ev.Handled=$true;if(-not $script:Busy -and $ev.Data.GetDataPresent([Windows.DataFormats]::FileDrop)){$ev.Effects=[Windows.DragDropEffects]::Copy}else{$ev.Effects=[Windows.DragDropEffects]::None}})
 foreach($name in @('NavPdfImages','NavImageCompress','NavImagesPdf','NavPdfCompress')){$C[$name].Add_Checked({param($sender,$ev) Switch-Mode ([string]$sender.Tag)})}
 $C.LimitBox.Add_SelectionChanged({Update-Limit})
 $C.RemoveButton.Add_Click({foreach($item in @($C.InputList.SelectedItems)){[void]$Queue.Remove($item)};Refresh-Queue})
 $C.ClearButton.Add_Click({$Queue.Clear();Refresh-Queue})
 $C.UpButton.Add_Click({$i=$C.InputList.SelectedIndex;if($i -gt 0){$Queue.Move($i,$i-1);$C.InputList.SelectedIndex=$i-1;Refresh-Queue}})
 $C.DownButton.Add_Click({$i=$C.InputList.SelectedIndex;if($i -ge 0 -and $i -lt $Queue.Count-1){$Queue.Move($i,$i+1);$C.InputList.SelectedIndex=$i+1;Refresh-Queue}})
 $C.BrowseButton.Add_Click({$dlg=New-Object Windows.Forms.FolderBrowserDialog;$dlg.Description='选择材料输出位置';$dlg.SelectedPath=$C.OutputBox.Text;if($dlg.ShowDialog() -eq [Windows.Forms.DialogResult]::OK){$C.OutputBox.Text=$dlg.SelectedPath};$dlg.Dispose()})
 $C.PreviewButton.Add_Click({Preview-Selected});$C.ResultList.Add_MouseDoubleClick({Preview-Selected});$C.InputList.Add_MouseDoubleClick({Preview-Selected})
 $C.ResultList.Add_SelectionChanged({
  $item=$C.ResultList.SelectedItem
  if($item){$txt=$item.Detail;if([long]$item.Bytes -gt 0){$txt='实际 '+([long]$item.Bytes).ToString('N0')+' 字节。'+$txt};$C.DetailText.Text=$txt}
 })
 $C.OpenOutputButton.Add_Click({if($script:LastOutput -and [IO.Directory]::Exists($script:LastOutput)){Start-Process -FilePath $script:LastOutput}})
 $C.CancelButton.Add_Click({if($script:Busy -and $script:CancelPath){[IO.File]::WriteAllText($script:CancelPath,'cancel');$C.CancelButton.IsEnabled=$false;$C.StatusText.Text='正在停止；已完成的文件会保留，不导出未完成文件。'}})
 $C.CropEditButton.Add_Click({
  if($script:Busy){return}
  try{
   $item=$C.InputList.SelectedItem
   if(-not $item -and $Queue.Count -eq 1){$item=$Queue[0];$C.InputList.SelectedIndex=0}
   if(-not $item){Message '请先在文件队列中选中一份 PDF。每份 PDF 的手动方案分别保存。';return}
   [double]$margin=0
   if(-not [double]::TryParse($C.CropMarginBox.Text,[Globalization.NumberStyles]::Float,[Globalization.CultureInfo]::InvariantCulture,[ref]$margin) -or [double]::IsNaN($margin) -or [double]::IsInfinity($margin) -or $margin -lt 0 -or $margin -gt 20){Message '安全边距请输入 0～20 mm。';return}
   $existing=$null;if($script:CropPlans.ContainsKey($item.Path)){$existing=$script:CropPlans[$item.Path]}
   $result=Show-AKCropEditor -Engine $Engine -Assets $Assets -InputPath $item.Path -Owner $Window -ExistingPlan $existing -Threshold ([int]$C.CropThresholdBox.SelectedItem.Tag) -MarginMM $margin -Password $C.PasswordBox.Password
   if($result){$script:CropPlans[$item.Path]=$result;$C.CropModeBox.SelectedIndex=3;Update-CropSummary;$C.StatusText.Text='已保存 '+$item.Name+' 的手动裁剪方案。开始处理时会重新渲染并校验大小。'}
  }catch{Message ('裁剪预览失败：'+$_.Exception.GetBaseException().Message)}
 })
 $C.CropClearButton.Add_Click({$script:CropPlans=@{};Update-CropSummary;if([string]$C.CropModeBox.SelectedItem.Tag -eq 'manual'){$C.CropModeBox.SelectedIndex=0};$C.StatusText.Text='已清除当前会话的手动裁剪方案；原始材料未修改。'})
 $C.HelpButton.Add_Click({
  Message @'
投递材料助手 ApplyKit 1.1.0

PDF 转图片：每页单独输出 JPG / JPEG / PNG / 静态 GIF。
新增裁剪：逐页自动去白边、同一 PDF 内容并集、手动拖动框选。
手动框可用于当前页或本 PDF 全部页；未单独设置的页保留整页。
裁剪仅作用于输出图片；空白页不删除，不修改原 PDF。
图片压缩：结果必须比原文件小；做不到且原件合规时，原字节保留。
图片合并 PDF：按队列顺序合并，并校验最终整份 PDF 的大小。
扫描版 PDF 压缩：保留全部页，但会丢失文本层、表单和数字签名。

默认上限 2,000,000 字节，安全目标 1,900,000 字节。
转换格式不保证天然变小；可选“转换结果也必须小于输入总量”。
清晰度底线只是质量与像素阈值，不是文字可读性的自动认证。
透明图片输出为白底。动态 GIF 不处理。请人工核对小字和印章。

本软件在本地处理，不包含上传、登录、统计或自动更新功能。
输入原件不会修改。输出报告仅保存在本机。
Windows 10 / 11 x64；需要系统自带 Windows PowerShell 5.1 和 WPF。
当前构建未做 Windows 真机端到端验收，具体测试范围见随包说明。

本程序未做商业代码签名。受管设备请遵守单位安全策略，无需关闭防护。
'@
 })
 $C.StartButton.Add_Click({
  try{
   if($script:Busy){return};if($Queue.Count -eq 0){Message '请先添加需要处理的材料。';return}
   $tag=[string]$C.LimitBox.SelectedItem.Tag
   [long]$limit=0
   if($tag -eq 'custom'){if($C.CustomLimit.Text -notmatch '^\d+$'){Message '自定义大小请输入整数 KB。';return};$limit=[long]$C.CustomLimit.Text*1000}else{$limit=[long]$tag}
   if($limit -lt 10000 -or $limit -gt 100000000){Message '大小上限应在 10KB～100MB 之间。';return}
   if($script:Mode -eq 'pdf-compress' -and -not $C.ConsentBox.IsChecked){Message '扫描版重建会丢失文本层和数字签名。请先阅读并勾选右侧确认。';return}
   $crop=Get-CropSettings
   $output=$C.OutputBox.Text.Trim();if(-not [IO.Path]::IsPathRooted($output)){Message '请选择完整的输出文件夹路径。';return}
   if($Queue.Count -gt 0 -and $script:Mode -eq 'images-pdf' -and $Queue.Count -gt 30){if([Windows.MessageBox]::Show($Window,'当前有 '+$Queue.Count+' 张图片，要压入一份小 PDF 可能触及清晰度底线。是否继续？','确认页数','YesNo','Question') -ne 'Yes'){return}}
   $script:JobDir=[IO.Path]::Combine([IO.Path]::GetTempPath(),'ApplyKit-session-'+[Guid]::NewGuid().ToString('N'));[void][IO.Directory]::CreateDirectory($script:JobDir);$script:JobDirs.Add($script:JobDir)
   $script:ProgressPath=[IO.Path]::Combine($script:JobDir,'progress.jsonl');$script:CancelPath=[IO.Path]::Combine($script:JobDir,'cancel.flag');$jobPath=[IO.Path]::Combine($script:JobDir,'job.json')
   $job=@{crop=$crop;mode=$script:Mode;inputs=@($Queue | ForEach-Object {$_.Path});output=$output;format=[string]$C.FormatBox.SelectedItem.Tag;limit=$limit;profile=[string]$C.ProfileBox.SelectedItem.Tag;dpi=[int]$C.DpiBox.SelectedItem.Tag;pages=$C.PagesBox.Text;paper=[string]$C.PaperBox.SelectedItem.Tag;strict=[bool]$C.StrictBox.IsChecked;makeZip=[bool]$C.ZipBox.IsChecked;consent=[bool]$C.ConsentBox.IsChecked;progress=$script:ProgressPath;cancel=$script:CancelPath}
   [IO.File]::WriteAllText($jobPath,($job | ConvertTo-Json -Depth 12),(New-Object Text.UTF8Encoding($false)))
   $psi=New-Object Diagnostics.ProcessStartInfo;$psi.FileName=$Engine;$psi.Arguments='--worker "'+$jobPath+'"';$psi.UseShellExecute=$false;$psi.CreateNoWindow=$true
   $psi.EnvironmentVariables['APPLYKIT_PDF_PASSWORD']=$C.PasswordBox.Password
   $C.PasswordBox.Clear()
   $script:Seen=0;$script:FinishedEvent=$false;$script:ActiveMode=$script:Mode;$script:LastOutput='';$Results.Clear();$C.ProgressBar.Value=0;$C.OpenOutputButton.IsEnabled=$false;$C.DetailText.Text='正在处理；选择结果可查看实际大小及提示。'
   $script:Process=New-Object Diagnostics.Process;$script:Process.StartInfo=$psi
   Set-Busy $true
   try{[void]$script:Process.Start()}finally{$psi.EnvironmentVariables.Remove('APPLYKIT_PDF_PASSWORD')}
   $C.WorkspaceTabs.SelectedIndex=1;$C.StatusText.Text='正在启动本地处理进程…';$Timer.Start()
  }catch{Set-Busy $false;Message ('无法开始处理：'+$_.Exception.GetBaseException().Message)}
 })
 $Window.Add_Closing({param($sender,$ev)
  if($script:Busy){
   $ev.Cancel=$true
   if([Windows.MessageBox]::Show($Window,'材料正在处理。是否停止任务，并在安全结束后退出？','退出确认','YesNo','Question') -eq 'Yes'){$script:CloseAfter=$true;[IO.File]::WriteAllText($script:CancelPath,'cancel');$C.CancelButton.IsEnabled=$false;$C.StatusText.Text='正在安全停止并退出…'}
  }
 })
 $Window.Add_Closed({$Timer.Stop();foreach($d in $script:JobDirs){try{[IO.Directory]::Delete($d,$true)}catch{}}})
 Switch-Mode 'pdf-images';Refresh-Queue
 [void]$Window.ShowDialog()
} catch {
 $msg=$_.Exception.GetBaseException().Message+"`r`n"+$_.ScriptStackTrace
 try{[IO.File]::WriteAllText([IO.Path]::Combine($Assets,'ui-error.log'),$msg,[Text.Encoding]::UTF8)}catch{}
 try{Add-Type -AssemblyName PresentationFramework;[void][Windows.MessageBox]::Show(('启动失败：'+$msg+"`r`n请查看随包说明。"),'ApplyKit','OK','Error')}catch{}
 exit 1
}

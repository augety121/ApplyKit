# PDF crop workbench. Windows PowerShell 5.1 / WPF. No WebView or network access.
# State is kept for one modal window and discarded on close. The caller only
# receives normalized rectangles and the source document's SHA-256 fingerprint.
function Copy-AKRect($r) {
 return @{left=[double]$r.left;top=[double]$r.top;right=[double]$r.right;bottom=[double]$r.bottom}
}
function Limit-AKValue([double]$v,[double]$lo,[double]$hi) { return [Math]::Max($lo,[Math]::Min($hi,$v)) }
function Show-AKMessage([string]$text) { [void][Windows.MessageBox]::Show($script:AKCrop.Window,$text,'裁剪工作台','OK','Information') }
function Set-AKShape($shape,[double]$x,[double]$y,[double]$w,[double]$h) {
 [Windows.Controls.Canvas]::SetLeft($shape,$x);[Windows.Controls.Canvas]::SetTop($shape,$y)
 $shape.Width=[Math]::Max(0,$w);$shape.Height=[Math]::Max(0,$h)
}
function Update-AKOverlay {
 $s=$script:AKCrop;if(-not $s.Current -or -not $s.Rect){return}
 $c=$s.Controls;[double]$w=$s.Current.Width;[double]$h=$s.Current.Height;$r=$s.Rect
 $x1=$r.left*$w;$y1=$r.top*$h;$x2=$r.right*$w;$y2=$r.bottom*$h
 Set-AKShape $c.ShadeTop 0 0 $w $y1
 Set-AKShape $c.ShadeBottom 0 $y2 $w ($h-$y2)
 Set-AKShape $c.ShadeLeft 0 $y1 $x1 ($y2-$y1)
 Set-AKShape $c.ShadeRight $x2 $y1 ($w-$x2) ($y2-$y1)
 Set-AKShape $c.KeepRect $x1 $y1 ($x2-$x1) ($y2-$y1)
 [double]$scale=[Math]::Min($c.PageView.ActualWidth/$w,$c.PageView.ActualHeight/$h)
 if($scale -le 0){$scale=0.4}
 [double]$handle=9/$scale;$c.KeepRect.StrokeThickness=1.8/$scale
 Set-AKShape $c.HandleTL ($x1-$handle/2) ($y1-$handle/2) $handle $handle
 Set-AKShape $c.HandleTR ($x2-$handle/2) ($y1-$handle/2) $handle $handle
 Set-AKShape $c.HandleBL ($x1-$handle/2) ($y2-$handle/2) $handle $handle
 Set-AKShape $c.HandleBR ($x2-$handle/2) ($y2-$handle/2) $handle $handle
 foreach($n in @('HandleTL','HandleTR','HandleBL','HandleBR')){$c[$n].StrokeThickness=1.4/$scale}
 $culture=[Globalization.CultureInfo]::InvariantCulture
 $c.LeftValue.Text=([double]($r.left*100)).ToString('0.##',$culture)
 $c.TopValue.Text=([double]($r.top*100)).ToString('0.##',$culture)
 $c.RightValue.Text=([double]($r.right*100)).ToString('0.##',$culture)
 $c.BottomValue.Text=([double]($r.bottom*100)).ToString('0.##',$culture)
 [int]$px=[Math]::Floor($x1);[int]$py=[Math]::Floor($y1)
 [int]$pw=[Math]::Min([int]$w,[int][Math]::Ceiling($x2))-$px
 [int]$ph=[Math]::Min([int]$h,[int][Math]::Ceiling($y2))-$py
 if($pw -gt 0 -and $ph -gt 0 -and $s.Bitmap){
  $rect=New-Object Windows.Int32Rect($px,$py,$pw,$ph)
  $cropped=New-Object Windows.Media.Imaging.CroppedBitmap($s.Bitmap,$rect);$cropped.Freeze();$c.ResultPreview.Source=$cropped
 }
 $c.SelectionText.Text=('第 {0} 页 · 预览 {1}×{2} → {3}×{4} px；正式导出按设置的 DPI 重新渲染。' -f $s.Current.Page,[int]$w,[int]$h,$pw,$ph)
}
function Update-AKPageLabels {
 $s=$script:AKCrop
 foreach($p in $s.Pages){$flag='';if($s.Plan.pages.ContainsKey([string]$p.Page)){$flag=' · 已设'}elseif($s.Plan.all){$flag=' · 统一'};$p.Label='第 '+$p.Page+' 页'+$flag}
 $s.Controls.PageList.Items.Refresh()
}
function Save-AKCurrentRect {
 $s=$script:AKCrop;if(-not $s.Current){return}
 $s.Plan.pages[[string]$s.Current.Page]=Copy-AKRect $s.Rect
 $s.Controls.ScopeText.Text='已暂存当前页的框选；点击底部“保存裁剪方案”后生效。'
 Update-AKPageLabels
}
function Set-AKRect($rect,[bool]$persist=$true) {
 $s=$script:AKCrop
 $r=Copy-AKRect $rect
 foreach($v in @($r.left,$r.top,$r.right,$r.bottom)){if([double]::IsNaN($v) -or [double]::IsInfinity($v) -or $v -lt 0 -or $v -gt 1){throw '边界必须位于 0%～100%。'}}
 if($r.right -le $r.left -or $r.bottom -le $r.top){throw '右边界必须大于左边界，下边界必须大于上边界。'}
 if($s.Current -and (($r.right-$r.left)*$s.Current.Width -lt 2 -or ($r.bottom-$r.top)*$s.Current.Height -lt 2)){throw '裁剪范围过小，请至少保留 2×2 个预览像素。'}
 $s.Rect=$r;Update-AKOverlay
 if($persist){Save-AKCurrentRect}
}
function Select-AKPage($page) {
 if(-not $page){return};$s=$script:AKCrop;$c=$s.Controls
 try{
  $s.Current=$page;$s.Drag=$null
  $bmp=New-Object Windows.Media.Imaging.BitmapImage;$bmp.BeginInit();$bmp.CacheOption=[Windows.Media.Imaging.BitmapCacheOption]::OnLoad;$bmp.UriSource=New-Object Uri([string]$page.Path);$bmp.EndInit();$bmp.Freeze()
  $s.Bitmap=$bmp;$c.PageImage.Source=$bmp
  $c.CropCanvas.Width=$page.Width;$c.CropCanvas.Height=$page.Height;$c.PageImage.Width=$page.Width;$c.PageImage.Height=$page.Height
  $rect=@{left=0.0;top=0.0;right=1.0;bottom=1.0};$text='此页未设置裁剪，将保留整页。'
  if($s.Plan.all){$rect=$s.Plan.all;$text='此页使用本 PDF 的统一手动框。'}
  if($s.Plan.pages.ContainsKey([string]$page.Page)){$rect=$s.Plan.pages[[string]$page.Page];$text='此页使用单独设置的手动框。'}
  Set-AKRect $rect $false;$c.ScopeText.Text=$text;$c.LoadingOverlay.Visibility='Collapsed';$c.EditorControls.IsEnabled=$true
 }catch{$c.StatusText.Text='页面预览失败：'+$_.Exception.GetBaseException().Message}
}
function Start-AKDrag($ev) {
 $s=$script:AKCrop;if(-not $s.Current){return};$canvas=$s.Controls.CropCanvas
 [void]$canvas.Focus();$point=$ev.GetPosition($canvas)
 $x=Limit-AKValue ($point.X/$s.Current.Width) 0 1;$y=Limit-AKValue ($point.Y/$s.Current.Height) 0 1
 $r=$s.Rect;$op='';$ex=10/[Math]::Max(100,$s.Controls.PageView.ActualWidth);$ey=10/[Math]::Max(100,$s.Controls.PageView.ActualHeight)
 if($y -ge $r.top-$ey -and $y -le $r.bottom+$ey){if([Math]::Abs($x-$r.left) -le $ex){$op+='l'}elseif([Math]::Abs($x-$r.right) -le $ex){$op+='r'}}
 if($x -ge $r.left-$ex -and $x -le $r.right+$ex){if([Math]::Abs($y-$r.top) -le $ey){$op+='t'}elseif([Math]::Abs($y-$r.bottom) -le $ey){$op+='b'}}
 if(-not $op){
  if($s.Controls.MoveMode.IsChecked -and $x -ge $r.left -and $x -le $r.right -and $y -ge $r.top -and $y -le $r.bottom){$op='move'}else{$op='draw'}
 }
 $s.Drag=@{X=$x;Y=$y;Original=(Copy-AKRect $r);Operation=$op;Changed=$false}
 [void]$canvas.CaptureMouse();$ev.Handled=$true
}
function Move-AKDrag($ev) {
 $s=$script:AKCrop;if(-not $s.Drag -or -not $s.Current){return}
 $point=$ev.GetPosition($s.Controls.CropCanvas)
 $x=Limit-AKValue ($point.X/$s.Current.Width) 0 1;$y=Limit-AKValue ($point.Y/$s.Current.Height) 0 1
 $d=$s.Drag;$r=Copy-AKRect $d.Original;$dx=$x-$d.X;$dy=$y-$d.Y;$minW=2.01/$s.Current.Width;$minH=2.01/$s.Current.Height
 switch($d.Operation){
  'draw'{
   $r.left=[Math]::Min($x,$d.X);$r.right=[Math]::Max($x,$d.X);$r.top=[Math]::Min($y,$d.Y);$r.bottom=[Math]::Max($y,$d.Y)
   if($r.right-$r.left -lt $minW -or $r.bottom-$r.top -lt $minH){return}
  }
  'move'{
   $w=$r.right-$r.left;$h=$r.bottom-$r.top;$r.left=Limit-AKValue ($r.left+$dx) 0 (1-$w);$r.top=Limit-AKValue ($r.top+$dy) 0 (1-$h);$r.right=$r.left+$w;$r.bottom=$r.top+$h
  }
  default{
   if($d.Operation.Contains('l')){$r.left=Limit-AKValue $x 0 ($r.right-$minW)}
   if($d.Operation.Contains('r')){$r.right=Limit-AKValue $x ($r.left+$minW) 1}
   if($d.Operation.Contains('t')){$r.top=Limit-AKValue $y 0 ($r.bottom-$minH)}
   if($d.Operation.Contains('b')){$r.bottom=Limit-AKValue $y ($r.top+$minH) 1}
  }
 }
 try{Set-AKRect $r $false;$d.Changed=$true}catch{}
 $ev.Handled=$true
}
function Finish-AKDrag {
 $s=$script:AKCrop;if(-not $s.Drag){return}
 $changed=$s.Drag.Changed;$s.Drag=$null
 if($changed){Save-AKCurrentRect}
 $s.Controls.CropCanvas.ReleaseMouseCapture()
}
function Read-AKPreviewEvents {
 $s=$script:AKCrop;if(-not [IO.File]::Exists($s.Progress)){return}
 $fs=$null;$reader=$null
 try{
  $fs=[IO.File]::Open($s.Progress,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::ReadWrite)
  $reader=New-Object IO.StreamReader($fs,[Text.Encoding]::UTF8,$true);$lines=$reader.ReadToEnd().Split([char]10)
 }catch{return}finally{if($reader){$reader.Dispose()}elseif($fs){$fs.Dispose()}}
 while($s.Seen -lt $lines.Length-1){
  $line=$lines[$s.Seen].Trim();if(-not $line){$s.Seen++;continue}
  try{$ev=$line | ConvertFrom-Json}catch{break};$s.Seen++
  switch([string]$ev.kind){
   'meta'{
    $s.Total=[int]$ev.total;$s.Hash=[string]$ev.sha256;$s.Controls.PageCountText.Text='共 '+$s.Total+' 页'
    if($s.Plan.sha256 -and $s.Plan.sha256 -ne $s.Hash){$s.Plan=@{sha256=$s.Hash;all=$null;pages=@{}};$s.Controls.StatusText.Text='源文件已更改，之前的裁剪方案已清除。'}
    $s.Plan.sha256=$s.Hash
   }
   'page'{
    $item=[pscustomobject]@{Page=[int]$ev.page;Path=[string]$ev.path;Width=[int]$ev.width;Height=[int]$ev.height;Suggested=(Copy-AKRect $ev.rect);Blank=[bool]$ev.blank;Note=[string]$ev.message;Label=('第 '+$ev.page+' 页')}
    $s.Pages.Add($item);Update-AKPageLabels
    if($s.Controls.PageList.SelectedIndex -lt 0){$s.Controls.PageList.SelectedIndex=0}
    $s.Controls.StatusText.Text='已生成 '+$s.Pages.Count+' / '+$s.Total+' 页预览；完整加载后可保存。'
   }
   'fatal'{$s.Failed=$true;$s.Controls.StatusText.Text='预览未完成：'+[string]$ev.message;$s.Controls.SaveButton.IsEnabled=$false}
   'complete'{$s.Done=$true;$s.Controls.StatusText.Text=[string]$ev.message}
  }
 }
}
function Show-AKCropEditor {
 param([string]$Engine,[string]$Assets,[string]$InputPath,$Owner,[hashtable]$ExistingPlan,[int]$Threshold=250,[double]$MarginMM=2,[string]$Password='')
 $text=[IO.File]::ReadAllText([IO.Path]::Combine($Assets,'crop.xaml'),[Text.Encoding]::UTF8)
 $reader=New-Object Xml.XmlNodeReader ([xml]$text);$dialog=[Windows.Markup.XamlReader]::Load($reader);$dialog.Owner=$Owner
 $controls=@{};foreach($m in [regex]::Matches($text,'x:Name="([^"]+)"')){$n=$m.Groups[1].Value;$controls[$n]=$dialog.FindName($n)}
 $work=[Windows.SystemParameters]::WorkArea;$dialog.Width=[Math]::Min($dialog.Width,$work.Width-32);$dialog.Height=[Math]::Min($dialog.Height,$work.Height-32)
 $dialog.MinWidth=[Math]::Min($dialog.MinWidth,$dialog.Width);$dialog.MinHeight=[Math]::Min($dialog.MinHeight,$dialog.Height)
 $plan=@{sha256='';all=$null;pages=@{}}
 if($ExistingPlan){
  $plan.sha256=[string]$ExistingPlan.sha256
  if($ExistingPlan.all){$plan.all=Copy-AKRect $ExistingPlan.all}
  if($ExistingPlan.pages){foreach($key in $ExistingPlan.pages.Keys){$plan.pages[[string]$key]=Copy-AKRect $ExistingPlan.pages[$key]}}
 }
 $dir=[IO.Path]::Combine([IO.Path]::GetTempPath(),'ApplyKit-crop-'+[Guid]::NewGuid().ToString('N'));[void][IO.Directory]::CreateDirectory($dir)
 $script:AKCrop=@{Window=$dialog;Controls=$controls;Plan=$plan;Pages=(New-Object 'System.Collections.ObjectModel.ObservableCollection[object]');Current=$null;Rect=$null;Bitmap=$null;Drag=$null;Directory=$dir;Progress=[IO.Path]::Combine($dir,'progress.jsonl');Cancel=[IO.Path]::Combine($dir,'cancel.flag');Process=$null;Seen=0;Total=0;Hash='';Done=$false;Failed=$false;ClosePending=$false;SavedPlan=$null}
 $controls.PageList.ItemsSource=$script:AKCrop.Pages;$controls.DocumentName.Text=[IO.Path]::GetFileName($InputPath)
 $controls.AutoSettings.Text='识别阈值 '+$Threshold+' · 安全边距 '+$MarginMM+' mm（在主界面调整）'
 $controls.PageList.Add_SelectionChanged({Select-AKPage $script:AKCrop.Controls.PageList.SelectedItem})
 $controls.CropCanvas.Add_MouseLeftButtonDown({param($sender,$ev) Start-AKDrag $ev})
 $controls.CropCanvas.Add_MouseMove({param($sender,$ev) Move-AKDrag $ev})
 $controls.CropCanvas.Add_MouseLeftButtonUp({param($sender,$ev) Finish-AKDrag;$ev.Handled=$true})
 $controls.CropCanvas.Add_LostMouseCapture({Finish-AKDrag})
 $controls.PageView.Add_SizeChanged({Update-AKOverlay})
 $controls.CropCanvas.Add_KeyDown({param($sender,$ev)
  $s=$script:AKCrop;if(-not $s.Current){return}
  if($ev.Key -eq [Windows.Input.Key]::Escape -and $s.Drag){$orig=$s.Drag.Original;$s.Drag=$null;Set-AKRect $orig $false;$s.Controls.CropCanvas.ReleaseMouseCapture();$ev.Handled=$true;return}
  $delta=1;if(([Windows.Input.Keyboard]::Modifiers -band [Windows.Input.ModifierKeys]::Shift) -ne 0){$delta=10}
  $dx=0.0;$dy=0.0
  switch($ev.Key.ToString()){'Left'{$dx=-$delta/$s.Current.Width}'Right'{$dx=$delta/$s.Current.Width}'Up'{$dy=-$delta/$s.Current.Height}'Down'{$dy=$delta/$s.Current.Height}default{return}}
  $r=Copy-AKRect $s.Rect;$w=$r.right-$r.left;$h=$r.bottom-$r.top;$r.left=Limit-AKValue ($r.left+$dx) 0 (1-$w);$r.top=Limit-AKValue ($r.top+$dy) 0 (1-$h);$r.right=$r.left+$w;$r.bottom=$r.top+$h
  Set-AKRect $r;$ev.Handled=$true
 })
 $controls.AutoButton.Add_Click({try{$s=$script:AKCrop;if($s.Current){Set-AKRect $s.Current.Suggested;$s.Controls.StatusText.Text=$s.Current.Note}}catch{Show-AKMessage $_.Exception.GetBaseException().Message}})
 $controls.UpdateValuesButton.Add_Click({
  try{
   $c=$script:AKCrop.Controls;$values=@()
   foreach($n in @('LeftValue','TopValue','RightValue','BottomValue')){
    [double]$v=0
    if(-not [double]::TryParse($c[$n].Text,[Globalization.NumberStyles]::Float,[Globalization.CultureInfo]::InvariantCulture,[ref]$v)){throw '请输入 0～100 的数字，小数点用英文句点。'}
    $values+=($v/100.0)
   }
   Set-AKRect @{left=$values[0];top=$values[1];right=$values[2];bottom=$values[3]}
  }catch{Show-AKMessage $_.Exception.GetBaseException().Message}
 })
 $controls.ApplyAllButton.Add_Click({
  $s=$script:AKCrop;if(-not $s.Current -or -not $s.Done){return}
  $answer=[Windows.MessageBox]::Show($s.Window,'这会清除本 PDF 各页的单独框选，并把当前框按页面比例应用到全部页。横竖版或内容位置不同的页面可能需要单独调整。请逐页核对。是否应用？','统一手动裁剪','YesNo','Question')
  if($answer -eq 'Yes'){$s.Plan.all=Copy-AKRect $s.Rect;$s.Plan.pages=@{};Update-AKPageLabels;$s.Controls.ScopeText.Text='已将当前框暂存为本 PDF 的统一框；仍可单独调整某页。'}
 })
 $controls.ResetPageButton.Add_Click({Set-AKRect @{left=0.0;top=0.0;right=1.0;bottom=1.0}})
 $controls.ResetAllButton.Add_Click({$s=$script:AKCrop;$s.Plan.all=$null;$s.Plan.pages=@{};Set-AKRect @{left=0.0;top=0.0;right=1.0;bottom=1.0} $false;Update-AKPageLabels;$s.Controls.ScopeText.Text='本 PDF 的裁剪已清空；保存后所有页保留整页。'})
 $controls.CloseButton.Add_Click({$script:AKCrop.Window.Close()})
 $controls.SaveButton.Add_Click({
  $s=$script:AKCrop;if(-not $s.Done -or $s.Failed -or -not $s.Hash){Show-AKMessage '请等完整预览加载成功后再保存。';return}
  Finish-AKDrag;$s.Plan.sha256=$s.Hash;$s.SavedPlan=$s.Plan;$s.Window.Close()
 })
 $timer=New-Object Windows.Threading.DispatcherTimer;$timer.Interval=[TimeSpan]::FromMilliseconds(200);$script:AKCrop.Timer=$timer
 $timer.Add_Tick({
  $s=$script:AKCrop
  try{
   Read-AKPreviewEvents
   if($s.Process -and $s.Process.HasExited){
    Read-AKPreviewEvents;$s.Timer.Stop();$s.Process.Dispose();$s.Process=$null
    if(-not $s.Done -and -not $s.Failed){$s.Failed=$true;$s.Controls.StatusText.Text='预览进程异常结束，请检查 PDF 密码及系统组件。'}
    $s.Controls.SaveButton.IsEnabled=($s.Done -and -not $s.Failed -and $s.Pages.Count -gt 0)
    $s.Controls.ApplyAllButton.IsEnabled=$s.Controls.SaveButton.IsEnabled
    if($s.ClosePending){$s.Window.Close()}
   }
  }catch{
   $s.Failed=$true;$s.Controls.SaveButton.IsEnabled=$false;$s.Controls.StatusText.Text='读取预览状态失败：'+$_.Exception.GetBaseException().Message
   if($s.Process -and -not $s.Process.HasExited){[IO.File]::WriteAllText($s.Cancel,'cancel')}
  }
 })
 $dialog.Add_Closing({param($sender,$ev)
  $s=$script:AKCrop
  if($s.Process -and -not $s.Process.HasExited){$ev.Cancel=$true;$s.ClosePending=$true;[IO.File]::WriteAllText($s.Cancel,'cancel');$s.Controls.SaveButton.IsEnabled=$false;$s.Controls.StatusText.Text='正在停止预览并清理临时文件…'}
 })
 try{
  $request=@{input=$InputPath;output=[IO.Path]::Combine($dir,'pages');progress=$script:AKCrop.Progress;cancel=$script:AKCrop.Cancel;threshold=$Threshold;marginMM=$MarginMM}
  $requestPath=[IO.Path]::Combine($dir,'request.json');[IO.File]::WriteAllText($requestPath,($request | ConvertTo-Json -Depth 8),(New-Object Text.UTF8Encoding($false)))
  $psi=New-Object Diagnostics.ProcessStartInfo;$psi.FileName=$Engine;$psi.Arguments='--crop-preview "'+$requestPath+'"';$psi.UseShellExecute=$false;$psi.CreateNoWindow=$true;$psi.EnvironmentVariables['APPLYKIT_PDF_PASSWORD']=$Password
  $process=New-Object Diagnostics.Process;$process.StartInfo=$psi;$script:AKCrop.Process=$process
  try{[void]$process.Start()}finally{$psi.EnvironmentVariables.Remove('APPLYKIT_PDF_PASSWORD');$Password=$null}
  $timer.Start();[void]$dialog.ShowDialog()
  return $script:AKCrop.SavedPlan
 }finally{
  $timer.Stop()
  $s=$script:AKCrop
  if($s.Process){try{if(-not $s.Process.HasExited){[IO.File]::WriteAllText($s.Cancel,'cancel');if(-not $s.Process.WaitForExit(2000)){$s.Process.Kill()}}}catch{};try{$s.Process.Dispose()}catch{}}
  $controls.PageImage.Source=$null;$controls.ResultPreview.Source=$null;$s.Bitmap=$null
  try{[IO.Directory]::Delete($dir,$true)}catch{}
  $script:AKCrop=$null
 }
}

<div align="center">

# ApplyKit · 投递材料助手

**把 PDF、图片和大小限制，整理成一套顺手的投递材料流程。**

Windows x64 · 本地处理 · PDF 裁剪 · 图片压缩 · 免安装 EXE

[English](README.en.md) · [使用说明](docs/USER_GUIDE.zh-CN.html) · [开发说明](docs/ARCHITECTURE.md) · [测试范围](docs/TESTING.md)

</div>

---

## 为什么做这个工具

报名系统经常只接受 JPG、JPEG、PNG、GIF，而且要求每个文件小于 2MB。手里的材料却可能是多页 PDF、带大白边的扫描件，或体积过大的图片。

ApplyKit 将格式转换、页面裁剪和字节上限校验放在一个中文 Windows 窗口里。没有登录、上传、云端接口、统计或自动更新；原始文件不被覆盖。

> **本交付构建的验证边界：** Go 核心与独立渲染样本已在 Linux 中检查；Windows 可执行文件已经交叉编译，但本交付环境没有实际运行 Windows GUI 或 Windows.Data.Pdf。仓库附带原生冒烟测试及发布工作流；不能把“附有测试”当作“测试已经运行”。具体记录见 [TESTING](docs/TESTING.md)。

## 功能

| 功能 | 使用方式 | 大小规则 |
|---|---|---|
| PDF → 图片 | 逐页输出 JPG / JPEG / PNG / 静态 GIF，支持页码选择 | 每张严格小于所选上限 |
| PDF 裁剪导出 | 逐页自动去白边、同一 PDF 统一内容边界、手动框选 | 先裁剪，再压缩，再校验 |
| 图片压缩 | 批量处理、白底合成、方向修正、分级清晰度 | 生成的压缩结果必须严格小于原件 |
| 图片 → PDF | 调整队列顺序，合并成一份图像型 PDF | 校验整份 PDF 的总大小 |
| 扫描版 PDF 压缩 | 保留全部页，逐页栅格化重建 | 必须变小；需确认文本层/数字签名丢失 |

默认上限是 **2,000,000 字节**，新生成文件以 **1,900,000 字节**为安全目标。原件已合规但无法继续缩小时，压缩模式可以复制原字节并标记“保留原件”，不会假报“压缩成功”。跨格式转换不保证天然变小；勾选“转换结果也必须小于输入总量”可额外启用严格保护。

## 使用

从该项目的 Releases 获取 `ApplyKit_Windows_x64_v1.1.0.zip`；维护者首次发布前，Releases 中可能还没有附件。解压完整软件包，双击 `ApplyKit.exe`。

运行成品不需要安装 Python、Go、Node.js 或额外的 PDF 转换器。目标系统为 Windows 10/11 x64，使用系统自带的 Windows PowerShell 5.1、WPF/.NET Framework 和 Windows.Data.Pdf。裁剪预览为原生 WPF 窗口，不使用网页或 WebView。

推荐首次用 `测试样本/crop-demo.pdf` 验证，再处理正式材料。软件包中的 `运行自检.cmd` 只使用合成测试样本，检查原生渲染、裁剪、PDF 合并和 WPF 加载；它不会处理你的真实简历。未签名构建不代表可以忽略安全提示；受管设备应遵守组织策略，不要关闭防护。

### PDF 白边裁剪

在“PDF 转图片”中添加材料，右侧“页面裁剪”可以直接选择自动或统一模式，然后开始处理。

需要精确框选时，选中队列中的一份 PDF，点击 **“打开裁剪预览 / 手动框选…”**。绿色框内内容会被保留；可以重画框、拖动边角、平移、用方向键微调，或者输入百分比边界。右侧显示裁剪结果。选择“保存裁剪方案”后，主界面切换为手动模式，再点击“开始处理”。

手动框可以用于当前页或本 PDF 全部页，之后仍可单独调整某页。手动方案只保存在当前软件会话，不保存为永久预设。多份 PDF 需要分别保存手动方案，不会把一份材料的框静默套到另一份上。

自动模式只去除页面四周留白，不识别并重排页面中间的空白；全白页会保留，不会自动删除。统一模式取内容区域的**并集**；横竖版或不同纸张尺寸不会被强行拉伸为同样像素尺寸。淡色内容仍需人工核对；对特别浅的文字可选“仅纯白”或手动裁剪。

## 构建源码

需要 Go 1.23 或更新版本；公开发布建议使用当时受支持的稳定版本。仓库只依赖 Go 标准库，因此没有 `go.sum`。图标与 Windows COFF 资源已包含，普通构建不需要 Python。

Windows 下运行：

```powershell
.\build.ps1
```

也可双击 `build_windows.cmd`。结果为 `dist/ApplyKit.exe`。开发机器需要 Go，运行该 EXE 的用户不需要。

跨平台交叉编译：

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w -H=windowsgui" -o dist/ApplyKit.exe .
```

开发时修改原创图标或版本资源，才需要执行 `python tools/make_resources.py`（依赖 Pillow）。重建合成 PDF 样本用 `python tools/make_test_samples.py`（依赖 ReportLab）。这些依赖不是软件运行依赖。

## 测试和发布

```bash
go test -count=1 -v ./...
go vet ./...
go test -race -count=1 ./...
```

Windows 原生冒烟测试：

```powershell
powershell.exe -NoLogo -NoProfile -STA -ExecutionPolicy RemoteSigned -File .\tools\windows_smoke.ps1
```

GitHub Actions 的 `Build and test` 流程在 Linux/Windows 测试，在 Windows 构建、执行原生冒烟测试并上传 ZIP。推送与源码版本一致的 `v1.1.0` 标签会触发发布流程；测试或构建失败时不发布。实际是否通过，以该仓库 Actions 运行记录为准。

详细上传步骤见 [GitHub 上传与发布](docs/GITHUB.md)。**仓库提交源码；软件 ZIP 放 Releases，不要把个人材料或输出报告放进 Git。**

## 目录

```text
assets/                 WPF 界面、裁剪工作台、Windows PDF 渲染脚本
crop.go                 白边检测、归一化坐标、裁剪及安全边距
crop_preview.go         有上限的 PDF 预览与文件指纹
image.go / pdf.go       图像编码、缩放、图像型 PDF 写入
worker.go               批处理、字节预算、裁剪导出、结果报告
engine_test.go          原有核心回归测试
crop_test.go            裁剪及导出流程测试（注入模拟渲染器）
tools/                  构建辅助、原生冒烟测试、合成样本生成
samples/                无个人数据的合成测试材料
.github/workflows/      自动测试、构建和标签发布
```

## 限制与隐私

不支持 OCR、文档内容重排、扫描倾斜校正或保留签名的 PDF 优化。动态 GIF 不处理。单个输入最多 250MB，图片最多 3200 万像素，单 PDF 最多 200 页，每批最多 200 个文件。

扫描版 PDF 压缩会丢失文本层、表单、链接和数字签名。自动裁剪阈值不能证明所有浅色标记都被保留；大小合规也不能证明文字可读或平台必然接受。提交前检查姓名、编号、小字、签字和印章。

正常退出会清理临时预览。崩溃/强制结束可能留下临时文件；报告可能含本机绝对路径，发 Issue 前先脱敏。缓存位置见使用说明。

## 许可

原创项目代码使用 [MIT License](LICENSE)。Go 运行时许可保存在 [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt)。Windows 系统组件不随软件重新分发。

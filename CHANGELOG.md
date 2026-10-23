# 2.2.2 — 2026-10-23

- 修复默认清晰度底线导致 100/200 KB 压缩过早失败；正常尝试失败后自动继续调整质量和尺寸，并明确提示清晰度取舍。
- 自动图片格式在原格式难以达标时转为 JPG；显式选择的 PNG/GIF 保持格式。
- 图片压缩、PDF 转图、图片合并 PDF、PDF 压缩统一支持小体积策略；PDF 全页保留、按整份大小校验。
- 新增 100 KB 快捷选项；说明自动转换与每张/整份文件的限制。

# 2.2.1 — 2026-10-20

- 修复扫描图超过 3200 万像素无法导入的问题；支持最多 1.6 亿像素，并在解码前检查内存预算。
- 超大扫描图等比缩小处理副本，原始文件和原始尺寸信息保持不变；界面和导出报告明确告知。
- 预览直接生成长边 1600 像素缩略图，预览与导出串行执行，避免并发大图处理。

# Changelog

## 2.2.0 — 2026-09-12

- Redesign the desktop workspace in light blue with complete dark-theme coverage, responsive rails and a separate results view.
- Add single-file processing, accessible queue rows, real automatic-crop previews, retryable previews and built-in synthetic examples.
- Expose and persist PDF safety margins from 0 to 20 mm.
- Fix first-launch output defaults, stale preview requests, rotation/flip ordering, and PDF points-to-pixel dimensions.
- Lock the queue and output controls during processing; preserve fatal/canceled states and reconnect job polling after transient failures.
- Add preference/preview ownership tests, frontend geometry tests and native Windows API acceptance.
- Update Windows resources and GitHub Actions; checksums are optional during build and packaging.

## 2.1.0 — 2026-09-10

- Rebuilt the main workspace to closely match the approved light-blue ApplyKit visual mockup: large branded hero, light navigation, file table, persistent preview card, compact export settings, and bottom output bar.
- Moved preview above export settings so users can confirm the selected PDF page or image before processing.
- Added folder import, row selection preview, original/crop preview tabs, page navigation, and direct edit entry from the preview card.
- Tightened responsive breakpoints to prevent card overlap at smaller Windows app sizes while preserving the desktop two-column layout.
- Kept the 1 KB–100 MB custom output limit, saved size presets, PDF white-margin crop, image crop/rotate/flip, batch processing, and byte-accurate validation from 2.0.0.


## 2.0.0 — 2026-09-10

- Rebuilt the desktop workflow around **Add → Edit → Size → Export & Check**.
- Replaced the previous crowded layout with responsive queue/results/settings regions and a fixed export dock; verified the 1246×877 layout geometry does not overlap.
- Added user-defined size limits from 1 KB to 100 MB with KB/MB switching, decimal values and saved presets.
- Added non-destructive image crop, fixed aspect ratios, 90° rotation, horizontal/vertical flip, white-margin trim and maximum-edge resize.
- Kept PDF per-page auto trim, uniform trim and manual crop, now surfaced in the same editor workflow.
- Added local HTTP session isolation using loopback-only binding, random port and random bearer token.
- Added responsive light/dark UI, result preview, cancelable jobs and incremental progress.
- Redesigned the ApplyKit application icon and embedded it into the Windows executable.
- Added new Windows acceptance script for embedded icon, custom 200 KB output, image editing, `Windows.Data.Pdf`, and the local web bundle.

## 1.1.0 — 2026-09-09

- Added PDF white-margin crop modes and manual page crop preview.
- Added source hashing for crop-plan invalidation and output size verification.

## 1.0.0

- Initial PDF/image conversion and compression workflow.

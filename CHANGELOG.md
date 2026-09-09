# Changelog

## 1.1.0 — 2026-09-09

### Added
- PDF 转图片：逐页自动去白边、同一 PDF 所选页的统一内容边界、手动裁剪。
- 本地裁剪工作台：逐页预览、重新画框、边角拖动、移动框、百分比输入、方向键微调、当前页/整份 PDF 范围。
- 手动方案保存的是归一化坐标；正式导出重新渲染，不拿低分辨率预览替代原页。
- 源 PDF 的 SHA-256 校验，拒绝把旧方案套到已变化的文件。
- 物理安全边距、空白页保留、稀疏内容保守回退、报告中的实际裁剪范围。
- GitHub CI、Windows 原生冒烟测试、自检脚本、标签发布工作流、中英文 README、MIT 许可与贡献指南。

### Changed
- 先裁剪再压缩，继续按真实输出字节数校验大小上限。
- 主窗口与裁剪窗口按 Windows 工作区调整初始尺寸。
- 裁剪配置不会隐式应用于图片合并 PDF 或扫描版 PDF 重建。

### Validation
- See `docs/TESTING.md` for the exact checks completed for the attached build.
- The included Windows smoke test and GitHub workflows are supplied, not claimed to have run in this delivery environment.

## 1.0.0

Initial executable build: PDF-to-images, image compression, images-to-PDF, raster PDF rebuilding, file-size limits, reports and batch processing.

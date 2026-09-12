# ApplyKit 2.2.0

An offline Windows desktop workspace for preparing job-application PDFs and images.

![Desktop workspace](docs/ui-v22-desktop.png)

Download `ApplyKit_Windows_x64_v2.2.0.zip` from [Releases](https://github.com/augety121/ApplyKit/releases), extract it, and run `ApplyKit.exe`. Windows 10/11 x64, Edge or Chrome, and Windows PowerShell 5.1 are required. End users do not need Go, Python or Node.js.

Features include PDF-to-image conversion, automatic/manual margin cropping, image crop/rotate/flip, image compression, image-to-PDF merging and scan-PDF reconstruction. Custom exclusive byte limits range from 1 KB to 100 MB (decimal units). Originals are never overwritten.

Version 2.2 adds a light/dark responsive desktop layout, separate queue/results views, configurable physical crop margins, real automatic-crop previews, bundled synthetic examples, corrected rotate/flip previews and PDF size calculations, plus safer job and preview lifecycles.

Build with Go 1.23+ using `./build.ps1`. Run `node --test tests/geometry.test.mjs` and `tools/windows_smoke.ps1`; package with `tools/package.ps1`. CI uses Go 1.26 on Linux and Windows. See [acceptance](docs/ACCEPTANCE.md) for actual checks and limitations.

Scan-PDF reconstruction and image merging produce image-only PDFs without searchable text, forms, links or digital signatures. Animated GIF and OCR are not supported. Review exported small text and stamps before submitting documents.

[中文说明](README.md) · [MIT License](LICENSE)

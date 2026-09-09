# ApplyKit

**Offline document preparation for application portals.**

[中文](README.md) · [Architecture](docs/ARCHITECTURE.md) · [Testing scope](docs/TESTING.md)

ApplyKit is a portable Windows x64 desktop utility for PDF-to-image conversion, page-margin cropping, image compression, image-to-PDF merging, and raster PDF rebuilding. It is designed around upload portals that accept JPG, JPEG, PNG or static GIF files below a strict size limit.

## Run

Extract the release ZIP and launch `ApplyKit.exe`. End users do not need Go, Python, Node.js, a PDF converter or a web account. It targets Windows 10/11 x64 using the operating system's Windows PowerShell 5.1, WPF and Windows.Data.Pdf components. The UI is Chinese.

The application does not upload documents, make network requests, collect analytics or auto-update. It never overwrites input files. The included self-check uses synthetic samples only.

**Validation notice:** the attached build was cross-compiled on Linux. Core tests and independently rendered fixtures were checked, but the Windows GUI and native PDF runtime have not been executed in the delivery environment. The native smoke test and CI workflows are provided, not represented as already passed. See `docs/TESTING.md`.

## Crop before compression

Automatic crop detects outer white margins and retains a physical safety margin. Uniform crop uses the union of content bounds from selected pages of the same PDF. Completely blank pages are retained; heterogeneous page sizes are not stretched into identical dimensions.

The manual workbench supports drawing a keep-rectangle, dragging its edges/corners, moving it, percentage coordinates, keyboard nudging, per-page overrides and applying a frame to the same PDF. The crop result is previewed. Coordinates are normalized and the final export re-renders the PDF rather than reusing preview pixels. SHA-256 fingerprints reject stale manual plans after source changes.

It does not remove internal whitespace, reorder document content, deskew scans or provide OCR. Very faint markings require visual inspection. Cropping is limited to PDF-to-image export, not silently applied to other modes.

## File-size semantics

The default exclusive limit is **2,000,000 bytes**, with a target of **1,900,000 bytes**. Compression output must be strictly smaller than its input. When further reduction is impossible and the original is already compliant, an unchanged byte-for-byte copy can be returned and explicitly labeled as retained.

Format conversion does not inherently reduce size. The optional strict conversion guard additionally requires the aggregate output to be smaller than the source PDF or input images. Failed targets are not labeled as successful. Raster PDF rebuilding requires consent because text, forms, links and digital signatures are discarded.

## Build

Go 1.23 or newer is required for source builds; use a currently supported stable toolchain for public releases. There are no third-party Go modules. The Windows resource object is committed, so ordinary builds need no resource-generation tools.

```powershell
.\build.ps1
```

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build \
  -trimpath -ldflags="-s -w -H=windowsgui" -o dist/ApplyKit.exe .
go test -count=1 ./...
go vet ./...
```

On real Windows, run `tools/windows_smoke.ps1` in Windows PowerShell 5.1 with `-STA`. CI uses a supported stable Go toolchain and publishes a release only after native smoke tests succeed. Interactive behavior and visual layout still require manual review.

## Contribute and publish

See `CONTRIBUTING.md`, `SECURITY.md` and `docs/RELEASE_CHECKLIST.md`. Do not commit personal application materials, identity documents, passwords, output reports or unredacted logs. Synthetic samples are included.

MIT-licensed project code. Go runtime notices are included in `THIRD_PARTY_NOTICES.txt`. Windows components are provided by the operating system, not bundled.

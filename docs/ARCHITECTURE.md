# Architecture / 开发说明

## Processes and files

`ApplyKit.exe` embeds `assets/` using Go embed. It extracts immutable, content-addressed resource files under `%LOCALAPPDATA%/ApplyKit/<version>-<hash>/`, launches Windows PowerShell 5.1 in STA mode and loads the WPF XAML. The GUI creates an isolated job directory and invokes the same EXE as a worker. JSONL progress is polled by WPF dispatch timers; heavy PDF/image work is not performed in mouse event handlers.

Entry points are `--worker <job.json>`, `--crop-preview <request.json>` and `--assets-info <output.json>`. These are internal diagnostics/interfaces, not a network service. Worker request files are consumed and removed after reading. The password is carried in an inherited environment variable; it is never written to request JSON or the command line.

Go's `renderPDF` invokes `assets/render.ps1`. The script uses Windows.Data.Pdf to load and render pages, emits metadata and temporary PNG paths, and disposes page/stream resources. Selected-page order is preserved; duplicate page numbers are ignored. The pipeline has page, file, pixel and render-size caps.

## Conversion pipeline

```text
PDF -> Windows page rendering -> white background image
    -> selected crop policy -> format encoder / bounded resizing
    -> final byte-length check -> non-overwriting output -> report
```

`fitImage` starts at the original cropped dimensions. JPEG quality is reduced within a selected floor; if needed, Lanczos resampling reduces resolution to a configured floor. PNG uses lossless encoding at each tested resolution; GIF is static/paletted. These floors are algorithmic guards, not a text-readability guarantee.

Every export is checked by actual byte length. Compression caps also use `originalBytes - 1`. PDF-to-images strict mode divides the aggregate source budget among selected pages; this is conservative and may reject a page even if a more complex adaptive allocation could fit. Image-to-PDF similarly uses per-page budgets plus structural headroom. There is no promise of optimal compression.

## Crop semantics

`NormalizedRect` describes the region to KEEP in top-left-origin normalized coordinates. `0,0,1,1` means a full page. Export maps it to rendered pixels with outward rounding. Cropped pixel buffers are zero-origin buffers, so image encoders/resamplers cannot accidentally reuse the wrong row offset.

Automatic detection scans every pixel. A pixel is content if any RGB channel is at or below the chosen threshold. It does not use OCR, majority voting or speckle removal. A faint colored edge mark therefore counts when it passes the threshold, regardless of how few pixels are in that row. Marks above the threshold can still be missed; the UI explicitly warns about this. Pure-white-only mode uses threshold 254.

Safety margin is expressed in millimeters and converted using the effective render resolution and physical page dimensions. Completely blank pages stay full-sized. Extremely sparse detections also preserve the entire page. Neither rule drops pages.

Uniform automatic crop performs two passes over selected pages of one PDF. The first computes the union of nonblank content rectangles in normalized coordinates, including margins. The second re-renders and exports. Blank pages stay complete. No intersecting crop or artificial stretching is used; differing page sizes may produce different pixel dimensions. Memory is bounded page-by-page, at the cost of additional rendering.

Manual plans are keyed by input path and bound to the input SHA-256. A per-page rectangle overrides the document-wide rectangle; a page without either stays whole. A missing plan for a queued PDF is an explicit error. The preview checks the file fingerprint both before and after rendering. The export checks it again before rendering. This prevents ordinary stale-plan mistakes; the program does not claim a hardened defense against concurrent hostile local file mutation.

## Preview and UI

The preview process renders at 110 DPI with a 1600-pixel long-edge cap. It sends page paths and suggested rectangles. WPF loads the current page without retaining file handles, displays shaded excluded regions and a CroppedBitmap result, and edits normalized coordinates. Only the current bitmap needs to be resident in the UI. All preview pages remain temporary files until the modal editor closes.

The manual workbench saves only a file fingerprint and numeric selections to the current UI session. It does not persist crop presets across launches. Applying one frame to a PDF's pages clears older per-page overrides after a confirmation, then allows new overrides.

## Failure behavior and privacy

Per-page fitting failures are reported without a fake successful file. Already completed pages remain on cancellation. The program publishes uniquely named outputs; it does not replace source materials. Reports include crop bounds, dimensions, actual sizes and notes. They may also include absolute paths, so they must not be uploaded unredacted.

Normal close removes job/preview temporary directories. Forced process termination or locked files may leave residual data. The application is not a sandbox or secure-deletion system. Its dependencies on Windows parsers should be kept patched.

## Tests

`engine_test.go` tests original encoding, limits, page semantics, PDF writing and file protection. `crop_test.go` tests exact crop logic and orchestration using an explicitly injected mock renderer. `tools/windows_smoke.ps1` exercises real Windows.Data.Pdf, the embedded scripts and WPF XAML loading. Mock tests cannot substitute for that native test or for manual UI acceptance.

## Primary technical references

- Windows PDF render API: https://learn.microsoft.com/en-us/uwp/api/windows.data.pdf.pdfpage.rendertostreamasync
- Render options: https://learn.microsoft.com/en-us/uwp/api/windows.data.pdf.pdfpagerenderoptions
- WPF canvas coordinates: https://learn.microsoft.com/en-us/dotnet/api/system.windows.controls.canvas
- WPF mouse capture: https://learn.microsoft.com/en-us/dotnet/api/system.windows.input.mouse.capture
- Go releases and toolchains: https://go.dev/dl/

The implementation is intentionally dependency-light, not a claim that these system components are available under every enterprise policy.

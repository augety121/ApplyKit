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

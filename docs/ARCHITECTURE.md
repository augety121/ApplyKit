# ApplyKit 2.2 Architecture

## Goals

ApplyKit is a Windows desktop utility for job-application materials. The design optimizes for four constraints: local-only processing, non-destructive input handling, exact byte-limit validation, and a responsive workflow that remains usable on common 1280×720-class laptops.

## Process model

`ApplyKit.exe` contains the Go processing engine, the web UI, the brand assets and the Windows PDF render script. On startup it binds an ephemeral TCP port on `127.0.0.1`, generates a 192-bit random session token, and launches Edge/Chrome in application-window mode. The UI talks only to that loopback session.

The HTTP layer rejects unexpected hosts/origins, requires `Authorization: Bearer <session-token>` for every `/api/*` endpoint, disables caching, applies a restrictive Content Security Policy, limits request sizes, and stores uploaded working copies in a per-launch temporary directory.

## UI

The UI is plain HTML/CSS/ES modules embedded at build time:

- `web/index.html` — semantic workspace and dialogs.
- `web/app.css` — shared components and editor styling.
- `web/desktop.css` — responsive desktop grid and theme tokens.
- `web/geometry.mjs` — PDF point-to-pixel conversion and shared Canvas transforms.
- `web/ui.js` — local API client, toasts and SVG icons.
- `web/app.js` — queue, settings, preferences, job polling and result preview.
- `web/editor.js` — Canvas-based non-destructive editor.

The primary layout regions are queue, results, settings and export dock. Desktop queue/results and settings scroll independently while the bottom export dock stays in its own grid row; this prevents the card-overlap issue from the 1.x layout.

## Processing pipeline

### PDF → images

`Windows.Data.Pdf` renders selected pages to temporary PNG files at 150/200/300 DPI. The page then goes through optional PDF crop, optional max-edge resize, image fitting, real-byte validation and atomic output publication.

Crop modes:

- `none` — full page.
- `auto` — detect non-white pixels on each page and expand by a physical margin.
- `uniform` — detect each selected page, take the union of content bounds, then re-render/export with one normalized region.
- `manual` — normalized rectangles saved from the editor and protected by the source PDF SHA-256.

Blank or extremely sparse pages are preserved conservatively rather than aggressively cropped.

### Image processing

The image pipeline is:

1. decode and normalize image orientation/transparency;
2. validate the edit plan against the source SHA-256;
3. rotate;
4. flip on display axes;
5. automatic trim or manual normalized crop;
6. optional max-edge downscale;
7. fit to target bytes;
8. validate final bytes;
9. publish atomically.

Pure compression requires output to be smaller than the source. If an already-compliant unedited file cannot become smaller within the quality floor, ApplyKit may copy the original bytes and explicitly marks the result as `unchanged`. An edited file never falls back to an unedited original.

### Images → PDF

Images are fitted into per-page budgets, JPEG encoded, and assembled into a simple image-only PDF. The limit applies to the entire final PDF. The generated PDF does not contain OCR/searchable text.

### Scanned PDF compression

All pages are rendered and rebuilt as image-only PDF pages. This is deliberately named a scanned-PDF rebuild: text layers, form fields, hyperlinks and digital signatures are not preserved.

## Byte limits

`parseByteLimit` parses decimal KB/MB values using `big.Rat`, avoiding floating-point boundary errors. Supported range: 1,000 to 100,000,000 bytes. The limit is exclusive. Encoders normally target 95% of the requested limit to leave upload-site headroom.

## Output safety

Each run creates a fresh directory. Individual files are first written to private temporary files and then published without overwriting an existing user file. The application never modifies the input path.

## Windows resources

`tools/make_brand.py` creates the original ApplyKit icon and SVG brand mark. `tools/make_resources.py` packages the icon, version information and manifest into `resource_windows_amd64.syso`, which the Go linker embeds in `ApplyKit.exe`.

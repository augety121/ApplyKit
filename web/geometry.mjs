// Render metadata exposes PDF dimensions in points (72 per inch), not WinRT DIPs.
export function materialSize(item, meta, dpi, rotation = 0) {
  let width = meta.sourceWidth || meta.width;
  let height = meta.sourceHeight || meta.height;
  if (item.kind === 'pdf' && meta.pageWidth && meta.pageHeight) {
    width = Math.round(meta.pageWidth * dpi / 72);
    height = Math.round(meta.pageHeight * dpi / 72);
  }
  return rotation % 180 ? [height, width] : [width, height];
}

// Canvas composes transforms in reverse order. Match the engine's rotate-then-flip pipeline.
export function drawTransformed(context, image, edit, width, height) {
  const iw = image.naturalWidth || image.width;
  const ih = image.naturalHeight || image.height;
  context.save();
  context.translate(width / 2, height / 2);
  context.scale(edit.flipH ? -1 : 1, edit.flipV ? -1 : 1);
  context.rotate((edit.rotation || 0) * Math.PI / 180);
  context.drawImage(image, -iw / 2, -ih / 2);
  context.restore();
}

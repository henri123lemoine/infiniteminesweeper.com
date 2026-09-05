export const OVERVIEW_MIN_ZOOM = 0.125;
export const OVERVIEW_MAX_PIXELS = 4 << 20;
export const OVERVIEW_MAX_SIDE = 4096;
const LEVELS = [64, 32, 16, 12, 8, 4, 2, 1];

function fitsOverviewLOD(lod, zoom, width, height) {
  const columns = Math.ceil(width / zoom / 64) + 2;
  const rows = Math.ceil(height / zoom / 64) + 2;
  return (
    columns * rows * lod * lod <= OVERVIEW_MAX_PIXELS &&
    columns * lod <= OVERVIEW_MAX_SIDE &&
    rows * lod <= OVERVIEW_MAX_SIDE
  );
}

export function targetOverviewLOD(zoom, width = 0, height = 0) {
  return (
    LEVELS.find(
      (lod) => lod <= 64 * zoom && fitsOverviewLOD(lod, zoom, width, height)
    ) || 1
  );
}

export function overviewRegionForView(
  view,
  width,
  height,
  lod,
  marginRatio = 0.25
) {
  const left = Math.floor(view.x / 64);
  const top = Math.floor(view.y / 64);
  const right = Math.ceil((view.x + width / view.zoom) / 64);
  const bottom = Math.ceil((view.y + height / view.zoom) / 64);
  const columns = Math.max(1, right - left);
  const rows = Math.max(1, bottom - top);
  let marginX = Math.ceil(columns * marginRatio);
  let marginY = Math.ceil(rows * marginRatio);
  while (
    ((columns + 2 * marginX) * (rows + 2 * marginY) * lod * lod >
      OVERVIEW_MAX_PIXELS ||
      (columns + 2 * marginX) * lod > OVERVIEW_MAX_SIDE ||
      (rows + 2 * marginY) * lod > OVERVIEW_MAX_SIDE) &&
    (marginX || marginY)
  ) {
    if (marginX && (!marginY || marginX * rows >= marginY * columns)) marginX--;
    else marginY--;
  }
  return {
    originX: left - marginX,
    originY: top - marginY,
    widthChunks: columns + marginX * 2,
    heightChunks: rows + marginY * 2,
  };
}

export function overviewPreviewForView(view, width, height) {
  const preview = {
    x: view.x + width / view.zoom / 2 - width / OVERVIEW_MIN_ZOOM / 2,
    y: view.y + height / view.zoom / 2 - height / OVERVIEW_MIN_ZOOM / 2,
    zoom: OVERVIEW_MIN_ZOOM,
  };
  const lod = Math.min(4, targetOverviewLOD(preview.zoom, width, height));
  return { lod, ...overviewRegionForView(preview, width, height, lod, 0.1) };
}

export function overviewRunwayZoom(lod, width, height, zoom) {
  let lower = lod / 64,
    upper = zoom;
  if (fitsOverviewLOD(lod, lower, width, height)) return lower;
  for (let i = 0; i < 16; i++) {
    const middle = (lower + upper) / 2;
    if (fitsOverviewLOD(lod, middle, width, height)) upper = middle;
    else lower = middle;
  }
  return upper;
}

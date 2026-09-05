import pako from "pako";
import { ms as PB } from "./gen/messages_pb.js";
import { OVERVIEW_MAX_PIXELS, OVERVIEW_MAX_SIDE } from "./overviewGeometry.js";

let palette;
self.onmessage = ({ data }) => {
  if (data.palette) {
    palette = data.palette;
    return;
  }
  const wireBytes = data.byteLength;
  const bytes = pako.ungzip(new Uint8Array(data));
  const snapshot = PB.Msg.decode(bytes).overviewSnapshot;
  if (
    snapshot &&
    !snapshot.unchanged &&
    typeof OffscreenCanvas !== "undefined"
  ) {
    const width = snapshot.widthChunks * snapshot.lod;
    const height = snapshot.heightChunks * snapshot.lod;
    const pixels = snapshot.pixels;
    if (
      width > 0 &&
      height > 0 &&
      width <= OVERVIEW_MAX_SIDE &&
      height <= OVERVIEW_MAX_SIDE &&
      pixels.length === width * height &&
      pixels.length <= OVERVIEW_MAX_PIXELS
    ) {
      const canvas = new OffscreenCanvas(width, height);
      const ctx = canvas.getContext("2d", { alpha: false });
      const image = ctx.createImageData(width, height);
      const rgba = new Uint32Array(image.data.buffer);
      for (let i = 0; i < pixels.length; i++) rgba[i] = palette[pixels[i]];
      ctx.putImageData(image, 0, 0);
      const bitmap = canvas.transferToImageBitmap();
      const { pixels: _, ...fields } = snapshot;
      self.postMessage(
        {
          snapshot: {
            ...fields,
            revision: Number(snapshot.revision),
            pixelByteLength: pixels.length,
            bitmap,
          },
          wireBytes,
        },
        [bitmap]
      );
      return;
    }
  }
  self.postMessage({ bytes, wireBytes }, [bytes.buffer]);
};

import React, { useCallback, useEffect, useRef, useState } from "react";
import { usePinchPanZoom } from "./hooks/usePinchPanZoom.js";
import { getHexForFlag } from "./sprites/index.js";

import {
  OVERVIEW_MIN_ZOOM as MIN_ZOOM,
  targetOverviewLOD,
  overviewRegionForView,
  overviewPreviewForView,
  overviewRunwayZoom,
} from "./overviewGeometry.js";
export {
  targetOverviewLOD,
  overviewRegionForView,
} from "./overviewGeometry.js";
const CHUNK = 64;
const MAX_ZOOM = 8;
const LOD_FADE_MS = 280;
const benchmarkEnabled = new URLSearchParams(window.location.search).has(
  "benchmark"
);

function logBenchmarkEvent(event) {
  if (!benchmarkEnabled || typeof window === "undefined") return;
  window.__overviewEventLog ||= [];
  window.__overviewEventLog.push({ at: performance.now(), ...event });
  if (window.__overviewEventLog.length > 100) window.__overviewEventLog.shift();
}

export default function OverviewMinimap({
  CELL_SIZE,
  zoom,
  viewX,
  viewY,
  containerRef,
  mainViewMoveToken,
  overviewCacheRef,
  overviewDiagnosticsRef,
  overviewTick,
  overviewConnectionGeneration,
  requestOverview,
  releaseOverview,
  activePlayersRef,
  onNavigate,
}) {
  const rootRef = useRef(null);
  const canvasRef = useRef(null);
  const [view, setView] = useState({ x: 0, y: 0, zoom: 2 });
  const viewRef = useRef(view);
  const [size, setSize] = useState({ width: 0, height: 0 });
  const [activeRecord, setActiveRecord] = useState(null);
  const activeRecordRef = useRef(null);
  const previousRecordRef = useRef(null);
  const transitionStartedRef = useRef(0);
  const requestedRef = useRef(null);
  const interactionAtRef = useRef(0);
  const zoomSpeedRef = useRef(0);
  const zoomAtRef = useRef(0);
  const hasZoomedRef = useRef(false);
  const rafRef = useRef(null);
  const lod = targetOverviewLOD(view.zoom, size.width, size.height);

  const setCurrentView = useCallback((next) => {
    viewRef.current = next;
    setView(next);
  }, []);

  const centerOnMainView = useCallback(() => {
    const root = rootRef.current;
    const game = containerRef?.current;
    if (!root) return;
    const gameWidth = game?.clientWidth || 0;
    const gameHeight = game?.clientHeight || 0;
    const centerX = (viewX + gameWidth / 2 / zoom) / CELL_SIZE;
    const centerY = (viewY + gameHeight / 2 / zoom) / CELL_SIZE;
    setCurrentView({
      x: centerX - root.clientWidth / (2 * viewRef.current.zoom),
      y: centerY - root.clientHeight / (2 * viewRef.current.zoom),
      zoom: viewRef.current.zoom,
    });
  }, [CELL_SIZE, containerRef, setCurrentView, viewX, viewY, zoom]);

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const resize = () =>
      setSize((current) => {
        const width = root.clientWidth;
        const height = root.clientHeight;
        return current.width === width && current.height === height
          ? current
          : { width, height };
      });
    resize();
    const observer = new ResizeObserver(resize);
    observer.observe(root);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!size.width || !size.height) return;
    centerOnMainView();
  }, [size.width, size.height, centerOnMainView]);

  useEffect(() => {
    if (
      !mainViewMoveToken ||
      performance.now() - interactionAtRef.current < 300
    )
      return;
    centerOnMainView();
  }, [mainViewMoveToken, centerOnMainView]);

  const requestLOD = useCallback(
    (requestedLOD) => {
      if (!size.width || !size.height) return;
      const current = viewRef.current;
      const bounds = {
        left: current.x,
        top: current.y,
        right: current.x + size.width / current.zoom,
        bottom: current.y + size.height / current.zoom,
      };
      const cache = overviewCacheRef.current;
      const record = cache.findForView(
        requestedLOD,
        bounds.left,
        bounds.top,
        bounds.right,
        bounds.bottom
      );
      const regionZoom = overviewRunwayZoom(
        requestedLOD,
        size.width,
        size.height,
        current.zoom
      );
      const regionView = {
        x:
          current.x +
          size.width / current.zoom / 2 -
          size.width / regionZoom / 2,
        y:
          current.y +
          size.height / current.zoom / 2 -
          size.height / regionZoom / 2,
        zoom: regionZoom,
      };
      const desiredRegion = overviewRegionForView(
        regionView,
        size.width,
        size.height,
        requestedLOD
      );
      const global = false;
      const region = record
        ? {
            originX: record.originX,
            originY: record.originY,
            widthChunks: record.widthChunks,
            heightChunks: record.heightChunks,
          }
        : desiredRegion;
      const knownRevision = record?.revision || 0;
      const previous = requestedRef.current;
      if (
        record &&
        previous?.lod === requestedLOD &&
        previous.generation === overviewConnectionGeneration &&
        cache.recordContainsView(
          previous.region,
          bounds.left,
          bounds.top,
          bounds.right,
          bounds.bottom
        )
      )
        return;
      if (
        requestOverview({
          lod: requestedLOD,
          ...region,
          global,
          knownRevision,
          subscribe: true,
          replaceSubscription: true,
        })
      ) {
        requestedRef.current = {
          lod: requestedLOD,
          generation: overviewConnectionGeneration,
          global,
          region: { lod: requestedLOD, global, ...region },
        };
      }
    },
    [
      overviewCacheRef,
      overviewConnectionGeneration,
      requestOverview,
      size.height,
      size.width,
    ]
  );

  useEffect(() => {
    if (!size.width || !size.height) return;
    const preview = overviewPreviewForView(
      viewRef.current,
      size.width,
      size.height
    );
    if (!overviewCacheRef.current.findExact({ ...preview, global: false })) {
      requestOverview({ ...preview, global: false, subscribe: false });
    }
  }, [
    overviewConnectionGeneration,
    overviewCacheRef,
    requestOverview,
    size.height,
    size.width,
  ]);

  useEffect(() => {
    if (!size.width || !size.height) return;
    const current = viewRef.current;
    const cache = overviewCacheRef.current;
    const candidate = cache.findClosestForView(
      lod,
      current.x,
      current.y,
      current.x + size.width / current.zoom,
      current.y + size.height / current.zoom
    );
    const active = activeRecordRef.current;
    if (candidate?.lod < lod && active?.lod >= lod && active.lod <= lod * 2)
      return;
    if (candidate && candidate !== activeRecordRef.current) {
      previousRecordRef.current = cache.recordContainsView(
        activeRecordRef.current,
        current.x,
        current.y,
        current.x + size.width / current.zoom,
        current.y + size.height / current.zoom
      )
        ? activeRecordRef.current
        : null;
      if (candidate.lod <= (activeRecordRef.current?.lod || 0))
        previousRecordRef.current = null;
      transitionStartedRef.current = performance.now();
      activeRecordRef.current = candidate;
      setActiveRecord(candidate);
    }
  }, [lod, overviewCacheRef, overviewTick, size.height, size.width, view]);

  useEffect(() => {
    if (!size.width || !size.height) return;
    const delay = !hasZoomedRef.current
      ? 100
      : zoomSpeedRef.current > 0.004
        ? 180
        : 50;
    logBenchmarkEvent({
      type: "schedule",
      lod,
      delay,
      hasZoomed: hasZoomedRef.current,
      speed: zoomSpeedRef.current,
    });
    let timer;
    const requestWhenSettled = () => {
      const settleDelay = zoomSpeedRef.current > 0.004 ? 180 : 50;
      const sinceZoom = performance.now() - zoomAtRef.current;
      logBenchmarkEvent({
        type: "timer",
        lod: targetOverviewLOD(viewRef.current.zoom, size.width, size.height),
        settleDelay,
        sinceZoom,
        speed: zoomSpeedRef.current,
      });
      if (hasZoomedRef.current && sinceZoom < settleDelay) {
        timer = setTimeout(requestWhenSettled, settleDelay - sinceZoom + 1);
        return;
      }
      requestLOD(
        targetOverviewLOD(viewRef.current.zoom, size.width, size.height)
      );
      timer = setTimeout(requestWhenSettled, 1000);
    };
    timer = setTimeout(requestWhenSettled, delay);
    return () => clearTimeout(timer);
  }, [lod, overviewCacheRef, requestLOD, size.height, size.width, view]);

  const paint = useCallback(() => {
    rafRef.current = null;
    const canvas = canvasRef.current;
    const root = rootRef.current;
    if (!canvas || !root) return;
    const width = root.clientWidth;
    const height = root.clientHeight;
    const dpr = Math.min(
      window.devicePixelRatio || 1,
      2,
      Math.sqrt((8 << 20) / (width * height))
    );
    const pixelWidth = Math.max(1, Math.floor(width * dpr));
    const pixelHeight = Math.max(1, Math.floor(height * dpr));
    if (canvas.width !== pixelWidth || canvas.height !== pixelHeight) {
      canvas.width = pixelWidth;
      canvas.height = pixelHeight;
    }
    const ctx = canvas.getContext("2d", { alpha: false });
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.imageSmoothingEnabled = false;
    ctx.fillStyle = "#c0c0c0";
    ctx.fillRect(0, 0, width, height);

    const current = viewRef.current;
    const record = activeRecordRef.current;
    const drawRecord = (source) => {
      const worldX = source.originX * CHUNK;
      const worldY = source.originY * CHUNK;
      const sourcePixelScale = (CHUNK / source.lod) * current.zoom;
      ctx.imageSmoothingEnabled = sourcePixelScale < 1.25;
      ctx.imageSmoothingQuality = "low";
      ctx.drawImage(
        source.canvas,
        (worldX - current.x) * current.zoom,
        (worldY - current.y) * current.zoom,
        source.widthChunks * CHUNK * current.zoom,
        source.heightChunks * CHUNK * current.zoom
      );
    };
    if (
      record &&
      !overviewCacheRef.current.recordContainsView(
        record,
        current.x,
        current.y,
        current.x + width / current.zoom,
        current.y + height / current.zoom
      )
    ) {
      const preview = overviewCacheRef.current.findClosestForView(
        targetOverviewLOD(current.zoom, width, height),
        current.x,
        current.y,
        current.x + width / current.zoom,
        current.y + height / current.zoom
      );
      if (preview && preview !== record) drawRecord(preview);
    }
    const previous = previousRecordRef.current;
    if (record && previous) {
      const elapsed = performance.now() - transitionStartedRef.current;
      const progress = Math.min(1, elapsed / LOD_FADE_MS);
      const eased = progress * progress * (3 - 2 * progress);
      ctx.globalAlpha = 1;
      drawRecord(previous);
      ctx.globalAlpha = eased;
      drawRecord(record);
      ctx.globalAlpha = 1;
      if (progress < 1) {
        rafRef.current = requestAnimationFrame(paint);
      } else {
        previousRecordRef.current = null;
      }
    } else if (record) {
      drawRecord(record);
    }

    const game = containerRef?.current;
    const gameWidth = game?.clientWidth || 0;
    const gameHeight = game?.clientHeight || 0;
    if (gameWidth && gameHeight) {
      const gameLeft = viewX / CELL_SIZE;
      const gameTop = viewY / CELL_SIZE;
      ctx.strokeStyle = "rgba(0,0,0,0.9)";
      ctx.lineWidth = 2;
      ctx.strokeRect(
        (gameLeft - current.x) * current.zoom,
        (gameTop - current.y) * current.zoom,
        (gameWidth / zoom / CELL_SIZE) * current.zoom,
        (gameHeight / zoom / CELL_SIZE) * current.zoom
      );
    }

    if (activePlayersRef?.current) {
      const radius = Math.max(2, Math.min(8, 2 * current.zoom));
      for (const player of activePlayersRef.current.values()) {
        player.x += (player.targetX - player.x) * 0.15;
        player.y += (player.targetY - player.y) * 0.15;
        const x = (player.x - current.x) * current.zoom;
        const y = (player.y - current.y) * current.zoom;
        if (x < 0 || y < 0 || x >= width || y >= height) continue;
        ctx.fillStyle = getHexForFlag(player.flagId || 0);
        ctx.globalAlpha = 0.85;
        ctx.beginPath();
        ctx.arc(x, y, radius, 0, Math.PI * 2);
        ctx.fill();
        ctx.globalAlpha = 1;
      }
    }
  }, [
    CELL_SIZE,
    activePlayersRef,
    containerRef,
    overviewCacheRef,
    viewX,
    viewY,
    zoom,
  ]);

  const schedulePaint = useCallback(() => {
    if (rafRef.current == null) rafRef.current = requestAnimationFrame(paint);
  }, [paint]);

  useEffect(
    () => schedulePaint(),
    [activeRecord, overviewTick, schedulePaint, view]
  );

  useEffect(() => {
    const timer = setInterval(schedulePaint, 100);
    return () => clearInterval(timer);
  }, [schedulePaint]);

  useEffect(
    () => () => {
      if (rafRef.current != null) cancelAnimationFrame(rafRef.current);
      releaseOverview();
    },
    [releaseOverview]
  );

  const { bind } = usePinchPanZoom({
    elementRef: canvasRef,
    getPointToWorld: () => ({ ...viewRef.current, mode: "overlay" }),
    onPan: (deltaX, deltaY) => {
      const current = viewRef.current;
      interactionAtRef.current = performance.now();
      setCurrentView({
        ...current,
        x: current.x - deltaX / current.zoom,
        y: current.y - deltaY / current.zoom,
      });
    },
    onZoom: (factor, _anchor, point) => {
      const current = viewRef.current;
      const nextZoom = Math.min(
        MAX_ZOOM,
        Math.max(MIN_ZOOM, current.zoom * factor)
      );
      const worldX = current.x + point.x / current.zoom;
      const worldY = current.y + point.y / current.zoom;
      const now = performance.now();
      const elapsed = now - zoomAtRef.current;
      const zoomDistance = Math.abs(Math.log(nextZoom / current.zoom));
      zoomSpeedRef.current =
        zoomAtRef.current > 0 && elapsed > 0 && elapsed < 500
          ? zoomDistance / elapsed
          : zoomDistance / 40;
      zoomAtRef.current = now;
      hasZoomedRef.current = true;
      logBenchmarkEvent({
        type: "zoom",
        from: current.zoom,
        to: nextZoom,
        speed: zoomSpeedRef.current,
      });
      interactionAtRef.current = now;
      setCurrentView({
        x: worldX - point.x / nextZoom,
        y: worldY - point.y / nextZoom,
        zoom: nextZoom,
      });
    },
    onInteraction: () => {
      interactionAtRef.current = performance.now();
    },
    enableRightClick: false,
    dragDelayMs: 0,
    useIncrementalPan: true,
  });

  const onDoubleClick = useCallback(
    (event) => {
      if (!onNavigate) return;
      const rect = canvasRef.current?.getBoundingClientRect();
      if (!rect) return;
      const current = viewRef.current;
      onNavigate(
        Math.floor(current.x + (event.clientX - rect.left) / current.zoom),
        Math.floor(current.y + (event.clientY - rect.top) / current.zoom)
      );
    },
    [onNavigate]
  );

  const targetReady =
    activeRecord?.lod === lod &&
    overviewCacheRef.current.recordContainsView(
      activeRecord,
      view.x,
      view.y,
      view.x + size.width / view.zoom,
      view.y + size.height / view.zoom
    );
  const cacheStats = overviewCacheRef.current.stats();
  if ((__DEV__ || benchmarkEnabled) && typeof window !== "undefined") {
    window.__minimapBenchmark = {
      targetLOD: lod,
      activeLOD: activeRecord?.lod || 0,
      targetReady,
      activeRegion: activeRecord
        ? {
            originX: activeRecord.originX,
            originY: activeRecord.originY,
            widthChunks: activeRecord.widthChunks,
            heightChunks: activeRecord.heightChunks,
            global: activeRecord.global,
          }
        : null,
      view,
      cache: cacheStats,
      network: { ...overviewDiagnosticsRef.current },
      events: [...(window.__overviewEventLog || [])],
    };
  }

  return (
    <div
      ref={rootRef}
      data-testid="overview-minimap"
      data-target-lod={lod}
      data-active-lod={activeRecord?.lod || 0}
      data-target-ready={targetReady ? "true" : "false"}
      data-coherent="true"
      data-cache-bytes={cacheStats.bytes}
      data-request-count={overviewDiagnosticsRef.current.requests}
      style={{ width: "100%", height: "100%", overflow: "hidden" }}
    >
      <canvas
        ref={canvasRef}
        {...bind}
        onDoubleClick={onDoubleClick}
        style={{
          display: "block",
          width: "100%",
          height: "100%",
          imageRendering: "pixelated",
          cursor: "grab",
          touchAction: "none",
        }}
      />
    </div>
  );
}

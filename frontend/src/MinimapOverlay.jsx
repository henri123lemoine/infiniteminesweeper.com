import React, { useEffect, useRef, useCallback, useState } from 'react';
import { CHUNK } from './useGameState.js';

export default function MinimapOverlay({ updateMinimapSubscriptions, clearMinimapSubscriptionsFor, minimapTilesRef }) {
  const canvasRef = useRef(null);
  const containerRef = useRef(null);
  const [view, setView] = useState({ x: 0, y: 0, zoom: 2 });
  const viewRef = useRef(view);
  const rafRef = useRef(null);

  const schedulePaint = useCallback(() => {
    if (rafRef.current) return;
    rafRef.current = requestAnimationFrame(() => {
      rafRef.current = null;
      const canvas = canvasRef.current;
      const container = containerRef.current;
      if (!canvas || !container) return;
      const ctx = canvas.getContext('2d');
      const dpr = window.devicePixelRatio || 1;
      const w = container.clientWidth;
      const h = container.clientHeight;
      canvas.width = Math.floor(w * dpr);
      canvas.height = Math.floor(h * dpr);
      canvas.style.width = `${w}px`;
      canvas.style.height = `${h}px`;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.imageSmoothingEnabled = false;
      ctx.clearRect(0, 0, w, h);
      // Fill background for overlay with unseen cell color (pale gray)
      ctx.fillStyle = '#808080';
      ctx.fillRect(0, 0, w, h);

      const { x, y, zoom } = viewRef.current;
      // Draw tiles
      const tilePx = CHUNK * zoom;
      // visible tile bounds in chunk coords
      const startChunkX = Math.floor(x / CHUNK);
      const startChunkY = Math.floor(y / CHUNK);
      const endChunkX = Math.floor((x + w / zoom) / CHUNK);
      const endChunkY = Math.floor((y + h / zoom) / CHUNK);

      for (let cy = startChunkY; cy <= endChunkY; cy++) {
        for (let cx = startChunkX; cx <= endChunkX; cx++) {
          const key = `${cx},${cy}`;
          const rec = minimapTilesRef.current.get(key);
          if (!rec || !rec.canvas) continue;
          const ox = Math.floor(cx * CHUNK - x);
          const oy = Math.floor(cy * CHUNK - y);
          ctx.drawImage(rec.canvas, 0, 0, CHUNK, CHUNK, Math.floor(ox * zoom), Math.floor(oy * zoom), Math.ceil(tilePx), Math.ceil(tilePx));
        }
      }
    });
  }, [minimapTilesRef]);

  // Center the minimap on world origin (0,0) so that origin appears in the middle
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const w = el.clientWidth || 0;
    const h = el.clientHeight || 0;
    const { zoom } = viewRef.current;
    // choose view so that world (0,0) is centered
    const nx = -w / (2 * zoom);
    const ny = -h / (2 * zoom);
    const next = { x: nx, y: ny, zoom };
    viewRef.current = next;
    setView(next);
    // trigger initial paint and subscriptions
    schedulePaint();
    const widthCells = Math.ceil(w / zoom);
    const heightCells = Math.ceil(h / zoom);
    const centerWorldX = 0;
    const centerWorldY = 0;
    updateMinimapSubscriptions(centerWorldX, centerWorldY, widthCells, heightCells, 1, 'overlay');
    return () => {
      // Clear overlay-specific subscriptions when this page unmounts
      clearMinimapSubscriptionsFor?.('overlay');
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Mouse pan, wheel zoom, and touch support
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    let dragging = false;
    let last = { x: 0, y: 0 };
    
    const onMouseDown = (e) => { 
      dragging = true; 
      last = { x: e.clientX, y: e.clientY }; 
    };
    
    const onMouseMove = (e) => {
      if (!dragging) return;
      const dx = (e.clientX - last.x) / viewRef.current.zoom;
      const dy = (e.clientY - last.y) / viewRef.current.zoom;
      last = { x: e.clientX, y: e.clientY };
      viewRef.current = { ...viewRef.current, x: viewRef.current.x - dx, y: viewRef.current.y - dy };
      setView(viewRef.current);
      schedulePaint();
    };
    
    const onMouseUp = () => { 
      dragging = false; 
    };
    
    const onWheel = (e) => {
      e.preventDefault();
      const rect = el.getBoundingClientRect();
      const mx = e.clientX - rect.left;
      const my = e.clientY - rect.top;
      const wz = viewRef.current.zoom;
      const factor = Math.exp(-e.deltaY * 0.0006); // slightly faster
      const nz = Math.min(Math.max(wz * factor, 0.125), 8);
      const worldX = viewRef.current.x + mx / wz;
      const worldY = viewRef.current.y + my / wz;
      const nx = worldX - mx / nz;
      const ny = worldY - my / nz;
      viewRef.current = { x: nx, y: ny, zoom: nz };
      setView(viewRef.current);
      schedulePaint();
    };
    
    // Touch event handlers for mobile support
    const onTouchStart = (e) => {
      if (e.touches.length === 1) {
        e.preventDefault();
        const touch = e.touches[0];
        dragging = true;
        last = { x: touch.clientX, y: touch.clientY };
      }
    };
    
    const onTouchMove = (e) => {
      if (!dragging || e.touches.length !== 1) return;
      e.preventDefault();
      const touch = e.touches[0];
      const dx = (touch.clientX - last.x) / viewRef.current.zoom;
      const dy = (touch.clientY - last.y) / viewRef.current.zoom;
      last = { x: touch.clientX, y: touch.clientY };
      viewRef.current = { ...viewRef.current, x: viewRef.current.x - dx, y: viewRef.current.y - dy };
      setView(viewRef.current);
      schedulePaint();
    };
    
    const onTouchEnd = () => {
      dragging = false;
    };
    
    // Add event listeners with appropriate passive settings
    el.addEventListener('mousedown', onMouseDown);
    el.addEventListener('touchstart', onTouchStart, { passive: false });
    el.addEventListener('touchmove', onTouchMove, { passive: false });
    el.addEventListener('touchend', onTouchEnd, { passive: false });
    el.addEventListener('wheel', onWheel, { passive: false });
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
    
    return () => {
      el.removeEventListener('mousedown', onMouseDown);
      el.removeEventListener('touchstart', onTouchStart);
      el.removeEventListener('touchmove', onTouchMove);
      el.removeEventListener('touchend', onTouchEnd);
      el.removeEventListener('wheel', onWheel);
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    };
  }, [schedulePaint]);

  // Keep subscriptions in sync with viewport
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const w = el.clientWidth;
    const h = el.clientHeight;
    const { x, y, zoom } = viewRef.current;
    const widthCells = Math.ceil(w / zoom);
    const heightCells = Math.ceil(h / zoom);
    const centerWorldX = Math.floor(x + widthCells / 2);
    const centerWorldY = Math.floor(y + heightCells / 2);
    updateMinimapSubscriptions(centerWorldX, centerWorldY, widthCells, heightCells, 1, 'overlay');
  }, [view, updateMinimapSubscriptions]);

  // Repaint when tiles update (simple interval)
  useEffect(() => {
    const id = setInterval(schedulePaint, 100);
    return () => clearInterval(id);
  }, [schedulePaint]);

  return (
    <div ref={containerRef} style={{ position: 'relative', width: '100%', height: '100%', background: '#202020' }}>
      <canvas ref={canvasRef} style={{ width: '100%', height: '100%' }} />
    </div>
  );
}



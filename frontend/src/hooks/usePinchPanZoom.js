import { useCallback, useRef, useEffect } from "react";

export function usePinchPanZoom({
  getPointToWorld,
  onPan,
  onZoom,
  onLongPress,
  onInteraction,
  minZoom = 0.1,
  maxZoom = 5,
  dragDelayMs = 150,
  enableRightClick = true,
  useIncrementalPan = false,
  elementRef = null,
}) {
  const dragStateRef = useRef({
    isDragging: false,
    startX: 0,
    startY: 0,
    lastX: 0,
    lastY: 0,
    viewStartX: 0,
    viewStartY: 0,
  });

  const touchStateRef = useRef({
    touches: [],
    initialDistance: 0,
    initialZoom: 1,
    initialMidpoint: { x: 0, y: 0 },
    initialViewX: 0,
    initialViewY: 0,
  });

  const dragTimeoutRef = useRef(null);
  const lastInteractionRef = useRef(0);

  const clearDragTimeout = useCallback(() => {
    if (dragTimeoutRef.current) {
      clearTimeout(dragTimeoutRef.current);
      dragTimeoutRef.current = null;
    }
  }, []);

  const resetDragState = useCallback(() => {
    dragStateRef.current = {
      isDragging: false,
      startX: 0,
      startY: 0,
      lastX: 0,
      lastY: 0,
      viewStartX: 0,
      viewStartY: 0,
    };
  }, []);

  const resetTouchState = useCallback(() => {
    touchStateRef.current = {
      touches: [],
      initialDistance: 0,
      initialZoom: 1,
      initialMidpoint: { x: 0, y: 0 },
      initialViewX: 0,
      initialViewY: 0,
    };
  }, []);

  const onWheel = useCallback(
    (e) => {
      e.preventDefault();
      lastInteractionRef.current = performance.now();
      onInteraction && onInteraction();

      const rect = e.currentTarget.getBoundingClientRect();
      const mouseX = e.clientX - rect.left;
      const mouseY = e.clientY - rect.top;

      const zoomFactor = Math.exp(-e.deltaY * 0.0007);
      const anchor = getPointToWorld(e.clientX, e.clientY);

      onZoom && onZoom(zoomFactor, anchor, { x: mouseX, y: mouseY });
    },
    [getPointToWorld, onZoom, onInteraction]
  );

  const onPointerDown = useCallback(
    (e) => {
      e.preventDefault();
      lastInteractionRef.current = performance.now();
      onInteraction && onInteraction();

      if (
        enableRightClick &&
        e.pointerType === "mouse" &&
        e.button === 2 &&
        onLongPress
      ) {
        const world = getPointToWorld(e.clientX, e.clientY);
        onLongPress(world.x, world.y);
        return;
      }

      if (e.pointerType === "mouse" && e.button !== 0) return;

      const current = getPointToWorld(0, 0);
      dragStateRef.current = {
        isDragging: false,
        startX: e.clientX,
        startY: e.clientY,
        lastX: e.clientX,
        lastY: e.clientY,
        viewStartX: current.viewX || 0,
        viewStartY: current.viewY || 0,
      };

      if (e.pointerType === "touch" && onLongPress) {
        dragTimeoutRef.current = setTimeout(() => {
          const world = getPointToWorld(e.clientX, e.clientY);
          onLongPress(world.x, world.y);
          resetDragState();
        }, 200);
      } else if (e.pointerType === "mouse") {
        dragTimeoutRef.current = setTimeout(() => {
          dragStateRef.current.isDragging = true;
        }, dragDelayMs);
      } else {
        dragTimeoutRef.current = setTimeout(() => {
          dragStateRef.current.isDragging = true;
        }, dragDelayMs);
      }

      e.currentTarget.setPointerCapture(e.pointerId);
    },
    [getPointToWorld, onLongPress, dragDelayMs]
  );

  const onPointerMove = useCallback(
    (e) => {
      if (!dragStateRef.current.startX) return;

      const dx = Math.abs(e.clientX - dragStateRef.current.startX);
      const dy = Math.abs(e.clientY - dragStateRef.current.startY);

      if ((dx > 10 || dy > 10) && !dragStateRef.current.isDragging) {
        clearDragTimeout();
        dragStateRef.current.isDragging = true;
      }

      if (dragStateRef.current.isDragging) {
        lastInteractionRef.current = performance.now();
        onInteraction && onInteraction();

        if (useIncrementalPan) {
          // Incremental pan (for minimap) - use frame-to-frame delta
          const deltaX = e.clientX - dragStateRef.current.lastX;
          const deltaY = e.clientY - dragStateRef.current.lastY;
          dragStateRef.current.lastX = e.clientX;
          dragStateRef.current.lastY = e.clientY;
          onPan && onPan(deltaX, deltaY, { incremental: true });
        } else {
          // Absolute pan (for main board) - use total delta from start
          const deltaX = e.clientX - dragStateRef.current.startX;
          const deltaY = e.clientY - dragStateRef.current.startY;
          onPan &&
            onPan(deltaX, deltaY, {
              startX: dragStateRef.current.startX,
              startY: dragStateRef.current.startY,
              viewStartX: dragStateRef.current.viewStartX,
              viewStartY: dragStateRef.current.viewStartY,
            });
        }
      }
    },
    [onPan, clearDragTimeout, onInteraction, useIncrementalPan]
  );

  const onPointerUp = useCallback(
    (e) => {
      clearDragTimeout();

      if (!dragStateRef.current.isDragging && dragStateRef.current.startX) {
        const world = getPointToWorld(e.clientX, e.clientY);
        onLongPress && onLongPress(world.x, world.y, false);
      }

      resetDragState();
      e.currentTarget.releasePointerCapture?.(e.pointerId);
    },
    [getPointToWorld, onLongPress, clearDragTimeout, resetDragState]
  );

  const onTouchStart = useCallback(
    (e) => {
      if (e.touches.length !== 2) return;

      e.preventDefault();
      clearDragTimeout();
      resetDragState();

      const touches = Array.from(e.touches).map((t) => ({
        id: t.identifier,
        x: t.clientX,
        y: t.clientY,
      }));

      const dx = touches[1].x - touches[0].x;
      const dy = touches[1].y - touches[0].y;
      const distance = Math.sqrt(dx * dx + dy * dy);
      const midX = (touches[0].x + touches[1].x) / 2;
      const midY = (touches[0].y + touches[1].y) / 2;

      const current = getPointToWorld(0, 0);
      touchStateRef.current = {
        touches,
        initialDistance: distance,
        initialZoom: current.zoom || 1,
        initialMidpoint: { x: midX, y: midY },
        initialViewX: current.viewX || 0,
        initialViewY: current.viewY || 0,
      };
    },
    [getPointToWorld, clearDragTimeout, resetDragState]
  );

  const onTouchMove = useCallback(
    (e) => {
      if (e.touches.length !== 2 || touchStateRef.current.initialDistance === 0)
        return;

      e.preventDefault();
      lastInteractionRef.current = performance.now();
      onInteraction && onInteraction();

      const touches = Array.from(e.touches).map((t) => ({
        id: t.identifier,
        x: t.clientX,
        y: t.clientY,
      }));

      const dx = touches[1].x - touches[0].x;
      const dy = touches[1].y - touches[0].y;
      const distance = Math.sqrt(dx * dx + dy * dy);
      const midX = (touches[0].x + touches[1].x) / 2;
      const midY = (touches[0].y + touches[1].y) / 2;

      const {
        initialDistance,
        initialZoom,
        initialMidpoint,
        initialViewX,
        initialViewY,
      } = touchStateRef.current;

      const zoomFactor = distance / initialDistance;
      const targetZoom = Math.min(
        Math.max(initialZoom * zoomFactor, minZoom),
        maxZoom
      );

      const worldX = initialViewX + initialMidpoint.x / initialZoom;
      const worldY = initialViewY + initialMidpoint.y / initialZoom;

      const newViewX = worldX - midX / targetZoom;
      const newViewY = worldY - midY / targetZoom;

      onZoom &&
        onZoom(
          targetZoom / initialZoom,
          { x: worldX, y: worldY },
          {
            x: midX,
            y: midY,
            newViewX,
            newViewY,
            targetZoom,
          }
        );
    },
    [onZoom, minZoom, maxZoom, onInteraction]
  );

  const onTouchEnd = useCallback(
    (e) => {
      if (e.touches.length === 0) {
        resetTouchState();
      } else if (
        e.touches.length === 1 &&
        touchStateRef.current.touches.length === 2
      ) {
        const remainingTouch = e.touches[0];
        const current = getPointToWorld(0, 0);

        dragStateRef.current = {
          isDragging: false,
          startX: remainingTouch.clientX,
          startY: remainingTouch.clientY,
          viewStartX: current.viewX || 0,
          viewStartY: current.viewY || 0,
        };

        touchStateRef.current.touches = [
          {
            id: remainingTouch.identifier,
            x: remainingTouch.clientX,
            y: remainingTouch.clientY,
          },
        ];
        touchStateRef.current.initialDistance = 0;
      }
    },
    [getPointToWorld, resetTouchState]
  );

  const onContextMenu = useCallback((e) => {
    e.preventDefault();
  }, []);

  useEffect(() => {
    const element = elementRef?.current;
    if (!element) return;

    element.addEventListener("wheel", onWheel, { passive: false });
    element.addEventListener("touchstart", onTouchStart, { passive: false });
    element.addEventListener("touchmove", onTouchMove, { passive: false });

    return () => {
      element.removeEventListener("wheel", onWheel);
      element.removeEventListener("touchstart", onTouchStart);
      element.removeEventListener("touchmove", onTouchMove);
    };
  }, [elementRef, onWheel, onTouchStart, onTouchMove]);

  const bind = {
    onPointerDown,
    onPointerMove,
    onPointerUp,
    onTouchEnd,
    onContextMenu,
    style: { touchAction: "none" },
  };

  return { bind, lastInteractionRef };
}

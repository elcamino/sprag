// Sprag - a post-quantum-safe end-to-end encrypted file dropbox.
// Copyright (C) 2026 Tobias von Dewitz <tobias@vondewitz.org>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

import { useEffect, useRef } from "react";
import { generateCircuitPaths, influencePoint } from "./circuitLineField";
import type { PointerState } from "./circuitLineField";

const POINTER_RADIUS = 180;
const POINTER_STRENGTH = 28;
const FRAME_MS = 1000 / 30;

export function RootLineField() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const pointerRef = useRef<PointerState>({ x: 0, y: 0, active: false });

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const context = canvas.getContext("2d");
    if (!context) return;
    const fieldCanvas: HTMLCanvasElement = canvas;
    const drawingContext: CanvasRenderingContext2D = context;

    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
    let frame = 0;
    let lastFrame = 0;
    let paths = generateCircuitPaths({ width: window.innerWidth, height: window.innerHeight, seed: 41 });

    function resize() {
      const ratio = Math.max(1, window.devicePixelRatio || 1);
      const width = Math.max(320, window.innerWidth);
      const height = Math.max(320, window.innerHeight);
      fieldCanvas.width = Math.round(width * ratio);
      fieldCanvas.height = Math.round(height * ratio);
      fieldCanvas.style.width = `${width}px`;
      fieldCanvas.style.height = `${height}px`;
      drawingContext.setTransform(ratio, 0, 0, ratio, 0, 0);
      paths = generateCircuitPaths({ width, height, seed: Math.round(width + height) });
      draw(0);
    }

    function draw(time: number) {
      const ratio = Math.max(1, window.devicePixelRatio || 1);
      const width = fieldCanvas.width / ratio;
      const height = fieldCanvas.height / ratio;
      const styles = getComputedStyle(document.documentElement);
      const motif = styles.getPropertyValue("--motif").trim() || "rgba(26, 24, 19, 0.05)";
      const accent = styles.getPropertyValue("--accent").trim() || "#c9952f";
      const muted = styles.getPropertyValue("--text-muted").trim() || "#6f6b5e";
      const pointer = reducedMotion.matches ? { ...pointerRef.current, active: false } : pointerRef.current;

      drawingContext.clearRect(0, 0, width, height);
      drawingContext.lineCap = "square";
      drawingContext.lineJoin = "miter";

      for (const path of paths) {
        drawingContext.beginPath();
        path.points.forEach((point, index) => {
          const animated = reducedMotion.matches
            ? point
            : {
                x: point.x + Math.sin(time / 2600 + index) * 1.8,
                y: point.y + Math.cos(time / 3100 + index) * 1.8
              };
          const influenced = influencePoint(animated, pointer, POINTER_RADIUS, POINTER_STRENGTH);
          if (index === 0) {
            drawingContext.moveTo(influenced.x, influenced.y);
          } else {
            drawingContext.lineTo(influenced.x, influenced.y);
          }
        });
        drawingContext.strokeStyle = path.accent ? accent : motif;
        drawingContext.globalAlpha = path.accent ? 0.34 : 1;
        drawingContext.lineWidth = path.accent ? 1.05 : 0.8;
        drawingContext.stroke();

        const end = path.points[path.points.length - 1];
        drawingContext.fillStyle = path.accent ? accent : muted;
        drawingContext.globalAlpha = path.accent ? 0.52 : 0.18;
        drawingContext.fillRect(end.x - 1.5, end.y - 1.5, 3, 3);
      }
      drawingContext.globalAlpha = 1;
    }

    function animate(time: number) {
      if (time - lastFrame > FRAME_MS) {
        draw(time);
        lastFrame = time;
      }
      if (!reducedMotion.matches) {
        frame = window.requestAnimationFrame(animate);
      }
    }

    function move(event: PointerEvent) {
      pointerRef.current = { x: event.clientX, y: event.clientY, active: true };
    }

    function leave() {
      pointerRef.current = { ...pointerRef.current, active: false };
    }

    const observer = new MutationObserver(() => draw(0));
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });

    resize();
    window.addEventListener("resize", resize);
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerleave", leave);
    reducedMotion.addEventListener("change", resize);
    if (!reducedMotion.matches) {
      frame = window.requestAnimationFrame(animate);
    }

    return () => {
      window.cancelAnimationFrame(frame);
      window.removeEventListener("resize", resize);
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerleave", leave);
      reducedMotion.removeEventListener("change", resize);
      observer.disconnect();
    };
  }, []);

  return <canvas className="circuit-line-field" ref={canvasRef} aria-hidden="true" />;
}

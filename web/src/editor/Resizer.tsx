/**
 * Vertical drag handle resizing the panel next to it with the pointer or the arrow keys.
 *
 * @param props.width Current panel width in pixels.
 * @param props.onChange Receives the new panel width, kept between 160 and 640 pixels.
 * @param props.side Side of the window the panel is on, the handle grows the panel towards the window centre.
 * @param props.label Accessible name of the handle.
 * @returns The handle.
 */
export function Resizer({
  width,
  onChange,
  side,
  label,
}: {
  width: number;
  onChange: (width: number) => void;
  side: 'left' | 'right';
  label: string;
}) {
  const resize = (value: number) => onChange(Math.round(Math.min(640, Math.max(160, value))));

  return (
    <div
      className="resizer"
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuenow={width}
      aria-valuemin={160}
      aria-valuemax={640}
      tabIndex={0}
      onPointerDown={(event) => {
        event.preventDefault();
        event.currentTarget.setPointerCapture(event.pointerId);
      }}
      onPointerMove={(event) => {
        if (event.currentTarget.hasPointerCapture(event.pointerId)) {
          resize(side === 'left' ? event.clientX : window.innerWidth - event.clientX);
        }
      }}
      onKeyDown={(event) => {
        const step = event.key === 'ArrowRight' ? 16 : event.key === 'ArrowLeft' ? -16 : 0;
        if (step) {
          event.preventDefault();
          resize(width + (side === 'left' ? step : -step));
        }
      }}
    />
  );
}

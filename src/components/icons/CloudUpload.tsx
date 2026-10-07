// The cloud-upload icon of moving icons (https://www.movingicons.dev,
// @jis3r/icons), MIT License, Copyright (c) 2024 jis3r, after Lucide.
// Its arrow rises while the icon, or an ancestor with the class
// "cloud-upload-hover", is hovered, or while animate is set.

/** CloudUpload is a cloud with an arrow that rises on hover. */
export function CloudUpload({ size = 24, strokeWidth = 2, animate = false, className = "" }: { size?: number; strokeWidth?: number; animate?: boolean; className?: string }) {
  return (
    <svg
      className={`cloud-upload${animate ? " animate" : ""}${className ? ` ${className}` : ""}`}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <g className="cloud-upload-arrow">
        <path d="M12 13v8" />
        <path d="m8 17 4-4 4 4" />
      </g>
      <path d="M4 14.899A7 7 0 1 1 15.71 8h1.79a4.5 4.5 0 0 1 2.5 8.242" />
    </svg>
  );
}

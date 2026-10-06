/** BrandMark is the app's mark: a guitar pick on the brand's gradient. */
export function BrandMark({ size = 26 }: { size?: number }) {
  return (
    <span className="brand-mark" style={{ width: size, height: size, borderRadius: size * 0.31 }}>
      <svg width={size * 0.62} height={size * 0.62} viewBox="0 0 24 24" aria-hidden="true">
        <path
          d="M12 22.2c-1.1 0-2.3-1.4-4.1-4.2C5.4 14.1 3.7 10.4 3.4 8.1 3.1 5.6 4 4.3 6.5 3.3 8.3 2.6 10.1 2.2 12 2.2s3.7.4 5.5 1.1c2.5 1 3.4 2.3 3.1 4.8-.3 2.3-2 6-4.5 9.9-1.8 2.8-3 4.2-4.1 4.2Z"
          fill="#fff"
        />
        <path d="M7.2 8.6h9.6" stroke="currentColor" strokeOpacity="0.55" strokeWidth="1.6" strokeLinecap="round" />
      </svg>
    </span>
  );
}

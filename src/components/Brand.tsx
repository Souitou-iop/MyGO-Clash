import mark from "../assets/mark.png";

/** BrandMark is the app's mark: the badge of the app icon. */
export function BrandMark({ size = 26 }: { size?: number }) {
  return <img className="brand-mark" src={mark} width={size} height={size} alt="" draggable={false} />;
}

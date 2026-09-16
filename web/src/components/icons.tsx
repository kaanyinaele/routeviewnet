// Inline icon set. A dependency (lucide, heroicons) would add a package and
// ship a component per glyph; the dashboard bundle is already large. These are
// stroke paths on a 24px grid, drawn in currentColor so they inherit whatever
// the surrounding text is doing.
//
// Multiple subpaths in one glyph are separated by "|".
const paths: Record<string, string> = {
  activity: "M3 12h3.5l2.5 7 4-14 2.5 7H21",
  globe: "M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0|M3 12h18|M12 3a15 15 0 0 1 0 18|M12 3a15 15 0 0 0 0 18",
  gauge: "M20 18a9 9 0 1 0-16 0|M12 14l4.5-4.5",
  bell: "M18 8a6 6 0 1 0-12 0c0 6-2 7-2 7h16s-2-1-2-7|M10.3 20a2 2 0 0 0 3.4 0",
  shield: "M12 3l7.5 3v5.5c0 4.5-3.2 7.9-7.5 9.5-4.3-1.6-7.5-5-7.5-9.5V6z",
  restart: "M21 12a9 9 0 1 1-2.6-6.4|M21 3.5V9h-5.5",
  warning: "M12 4l9 16H3z|M12 10v4|M12 17h.01",
  check: "M4 12.5l5 5L20 7",
  send: "M21 3L10.5 13.5|M21 3l-6.8 18-3.7-7.5L3 9.8z",
  block: "M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0|M5.6 5.6l12.8 12.8",
  clock: "M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0|M12 7v5l3 2",
  eye: "M2 12s3.6-6.5 10-6.5S22 12 22 12s-3.6 6.5-10 6.5S2 12 2 12z|M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0",
  undo: "M3 12a9 9 0 1 0 2.6-6.4|M3 3.5V9h5.5",
};

export type IconName = keyof typeof paths;

export function Icon({
  name,
  size = 16,
  className = "",
}: {
  name: IconName;
  size?: number;
  className?: string;
}) {
  return (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={`shrink-0 ${className}`}
      aria-hidden
    >
      {paths[name].split("|").map((d) => (
        <path key={d} d={d} />
      ))}
    </svg>
  );
}

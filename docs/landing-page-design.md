# RouteViewNet Landing Page: Design Concept & Motion Spec

Companion to [landing-page.md](landing-page.md) (content blueprint). This
document defines the visual direction, the design concept, and the full
GSAP animation choreography, written to Awwwards-submission standards:
a strong central concept, purposeful motion, obsessive detail, and
flawless performance.

---

## 1. Design concept: "The Signal Path"

RouteViewNet's core insight is that every web page travels a path:
**your computer → your router → the internet → the DNS address book**,
and the product tells you which step broke.

The entire page is built around that idea: **a single continuous signal
line** (an SVG path, 2px, cyan `#38bdf8`) that enters at the hero,
travels vertically through every section, branches at the "three lights"
section into Gateway / Internet / DNS nodes, degrades into a red flatline
in the "problem" section, recovers to green in the troubleshoot section,
and terminates in the footer plugged into the tagline. The line draws
itself as you scroll (scrubbed, not timed), so the visitor physically
pulls the signal through the page.

Everything else supports the metaphor: pulse dots traveling along the
line, node cards that light up as the line reaches them, and data
readouts that count up as if being measured live.

**Mood keywords:** instrument panel, oscilloscope, calm confidence,
terminal heritage, zero noise. Think "mission control for your Wi-Fi",
not "crypto dashboard".

---

## 2. Visual language

### Color (tokens, taken from the app so product shots blend in)

| Token | Value | Use |
|---|---|---|
| `--bg-0` | `#0B1120` | page background (darker than app for depth) |
| `--bg-1` | `#0f172a` | section panels, cards |
| `--bg-2` | `#1e293b` | raised surfaces, code blocks |
| `--ink-1` | `#e2e8f0` | headlines, body |
| `--ink-2` | `#94a3b8` | secondary text |
| `--signal` | `#38bdf8` | the line, links, primary accents |
| `--good` | `#22c55e` | healthy states, success beats |
| `--warn` | `#f59e0b` | degraded states |
| `--crit` | `#ef4444` | failure beats in the story |

Dark theme only. Light theme is explicitly out of scope; the concept
depends on the glow.

Texture: a barely-visible dot grid (`radial-gradient` dots, 24px pitch,
3% opacity) over `--bg-0`, plus a slow-breathing radial glow behind the
hero (GSAP `yoyo` opacity 0.06 → 0.12, 8s). No noise GIFs, no heavy
shaders.

### Typography

| Role | Face | Notes |
|---|---|---|
| Display | **Space Grotesk** (700) | headlines, tight tracking (-2%), sizes fluid via `clamp(2.5rem, 6vw, 5.5rem)` |
| Body | **Inter** (400/500) | 1.125rem / 1.7 line height, max 65ch |
| Data / mono | **JetBrains Mono** | numbers, code, labels, the score counter, nav |

Numbers always render in mono with `font-variant-numeric: tabular-nums`
so count-up animations do not jitter.

### Layout

- 12-column grid, 1200px max content width, generous 160px vertical
  section rhythm (96px on mobile).
- The signal line lives in a fixed 80px gutter left of the content on
  desktop; on mobile it runs behind the content at 8% opacity.
- Real dashboard screenshots sit in device-less browser frames with a
  1px `--bg-2` border and a soft 40px `--signal` glow at 8% opacity.

---

## 3. Motion principles

1. **Scroll is the timeline.** Nearly everything is `ScrollTrigger`
   scrubbed or triggered; almost nothing autoplays. The visitor conducts.
2. **One hero moment per viewport.** Never two competing animations on
   screen; supporting elements use opacity/transform only.
3. **Physics, not bounce.** Eases: `power3.out` for entrances,
   `power2.inOut` for scrubbed moves, `expo.out` for numbers. No
   `elastic`, no `back` beyond 1.2 overshoot.
4. **Fast in, calm settle.** Entrance durations 0.6 to 0.9s, stagger 0.06
   to 0.1s. Anything longer must be scrub-bound.
5. **Respect `prefers-reduced-motion`:** every timeline gets a
   `gsap.matchMedia()` variant that swaps motion for simple fades and
   sets the signal line fully drawn.

### Stack

- GSAP core + `ScrollTrigger` (required) + `ScrollToPlugin` (hero CTA
  anchor scroll; both free)
- `ScrollSmoother` optional (0.8 smoothing, desktop only); acceptable to
  skip and stay native for better INP
- Text splitting: `SplitText` if a Club GSAP license is available,
  otherwise the free `split-type` package; animate `chars` in the hero
  only, `lines` elsewhere
- No other animation libraries; drawing the line is plain
  `stroke-dashoffset` tweened by GSAP

---

## 4. Section-by-section choreography

### 4.0 Preloader (max 1.2s, once per session)
Mono text `establishing connection…` types on, three dots ping
(stagger 0.12), then the whole layer wipes upward with `clipPath` inset
while the hero line begins drawing. Skips entirely on repeat visits
(sessionStorage flag) and under reduced motion.

### 4.1 Hero
- Signal line draws from top center down to the CTA (scrub-free intro
  tween, 1.2s, `power2.inOut`), a pulse dot then loops along it every 4s.
- Headline "Know why your internet feels slow." reveals per word,
  `yPercent: 110 → 0` inside overflow-hidden masks, stagger 0.08.
- The subheadline and CTAs fade up 24px, 0.15s after the headline ends.
- A live-looking score dial next to the headline counts 0 → 96
  (`textContent` snap tween, `expo.out`, 1.4s) then breathes.
- Dashboard screenshot enters at 12deg `rotateX` perspective and
  flattens to 0 as the user starts scrolling (scrubbed over 60vh).
- CTA buttons: magnetic hover (element translates up to 8px toward the
  cursor via `gsap.quickTo`, returns on leave), plus a 1px border-glow
  pulse on the primary. The primary ("Install now") smooth-scrolls to the
  install block with `gsap.to(window, { scrollTo: "#install" })`, and a
  pulse dot travels down the signal line ahead of the scroll, leading the
  eye to the destination.

### 4.2 The problem
- Pinned for 150vh. The signal line, center stage, degrades: cyan →
  amber → red as scrub progresses, with jitter (`gsap.utils.random`
  displacement of path points via a `MotionPathHelper`-free approach:
  three pre-baked SVG path variants cross-faded).
- Rotating complaint words ("the Wi-Fi is slow", "pages will not load",
  "is it the router?") swap in mono type, each entering with a glitchy
  1-frame `skewX` snap.
- Exit beat: the line snaps back to cyan the instant the section
  releases, cueing "RouteViewNet fixes the guessing".

### 4.3 Feature grid (10 cards)
- Cards rise 40px + fade with `ScrollTrigger.batch`, stagger 0.08,
  `once: true`.
- Each card's icon is a 24px SVG that draws its own strokes
  (`drawSVG`-style dashoffset, 0.5s) when its card lands.
- Hover: card lifts 4px, border shifts to `--signal` at 40%, the icon
  replays its draw. All hover tweens ≤ 0.25s.
- The three marquee stats between grid rows (0 telemetry calls, 1
  SQLite file, 0-100 score) count up on enter, mono, `expo.out`.

### 4.4 The three lights (signature section, pinned 300vh)
The signal line branches into three nodes: Gateway, Internet, DNS.
Scrub-driven story in three beats:
1. All three nodes pulse green; readouts show real-looking latencies
   counting live.
2. The Internet node flickers amber then red; its branch flatlines; a
   plain-English caption types on: "Your router is fine. The problem is
   your provider."
3. The Troubleshoot card slides in from the right with the suggested
   actions checking themselves off (stagger 0.2), the node recovers to
   green, the line reunifies and continues downward.
Each beat maps to one third of the pin distance with `snap` at
`[0, 0.33, 0.66, 1]` so the story never stalls between states.

### 4.5 Data usage by app
- A horizontal bar chart builds per bar (scaleX from 0, transform-origin
  left, stagger 0.1, scrubbed over the section).
- The "View all with 20 MB or more" pill appears with a single soft
  overshoot (`back.out(1.2)`), the one playful moment on the page.

### 4.6 How it works (3 steps) + the install block
- Numbered steps connected by the signal line; each step's number fills
  with `--signal` as the line reaches it (scrub).
- **The install block (`#install`)** is a tabbed code panel with three
  tabs: One-liner, Download the .deb, Build from source (copy in the
  content doc). Choreography:
  - On first reveal (60% visible), the active tab's command types itself
    character by character (mono, 24 chars/s), then the copy button pings
    once (scale 1 → 1.15 → 1, 0.3s).
  - The tab underline is a single element that slides and stretches to
    the active tab with `gsap.to`, 0.3s `power2.inOut`; never two
    underlines at once.
  - Switching tabs crossfades the code panes (outgoing opacity 0.15s,
    incoming 0.2s, 8px rise). The typing effect plays only on the first
    reveal of the section; after that, switched-to tabs render their
    command instantly so repeat visitors are never made to wait.
  - The panel height animates between tabs (measured heights, 0.25s) so
    the multi-line source tab does not snap the layout.
  - The copy button swaps its label to "copied" for 1.2s with a green
    `--good` tick; no toast, no confetti.
  - Small print under each tab fades in 0.1s after its pane settles.
  - Reduced motion: no typing, no crossfade; tabs switch instantly with
    full text, the underline still moves (position only, no stretch).

### 4.7 Privacy block
- Background inverts to near-black `#070b14`; the dot grid fades out
  (data going quiet is the point).
- Headline "Your data never leaves your machine." assembles from
  letters that fly in from ±16px with blur 8px → 0.
- Each bullet gets a mono `[x]` checkbox that stamps in, stagger 0.12.
- The signal line here visibly loops back on itself around a machine
  icon: nothing exits the loop.

### 4.8 Honest limits + FAQ
- Deliberately the calmest section: fades only, 0.5s, no transforms.
  After the fireworks, honesty reads as stillness.
- FAQ accordions animate height with `gsap.to` on a measured
  `scrollHeight`, 0.35s `power2.inOut`, chevron rotates 180deg.

### 4.9 Footer
- The signal line terminates in a socket icon next to the tagline; on
  arrival it emits one final green pulse ring (scale 1 → 2.5, opacity
  1 → 0, 0.9s).
- Tagline words stagger in; links underline with scaleX origin-left
  on hover.

---

## 5. Micro-interactions (global)

- **Cursor:** default cursor everywhere (custom cursors hurt usability
  scores); interactive elements get the magnetic effect instead.
- **Nav:** mono, top-right, background blurs in (`backdrop-filter`)
  after 100vh; active-section indicator is a 4px pulse dot.
- **Scroll cue:** a 1-character mono `▼` in the hero, opacity yoyo,
  removed after first scroll.
- **Link hover:** underline draws left → right, 0.25s; never color-only.
- **Button press:** scale 0.97 for 0.1s, `power1.out`.

---

## 6. Performance & accessibility budget (Awwwards judges check)

- Lighthouse: Performance ≥ 90, Accessibility ≥ 95 on mid-range mobile.
- LCP ≤ 2.0s: hero headline is system-rendered text, fonts
  `font-display: swap` with size-adjusted fallbacks; screenshot lazy
  after first paint.
- JS budget ≤ 130KB gzipped total including GSAP (~45KB core + ST).
- All animation on `transform`/`opacity`/`clipPath` only; `will-change`
  applied by GSAP, never hand-sprinkled.
- Pinned sections use `pinType: transform` inside ScrollSmoother, or
  default fixed pinning without it; test iOS Safari address-bar resize.
- Full keyboard path: pinned storytelling must not trap focus; all
  scrub content also readable statically (text is in the DOM, not
  canvas).
- `prefers-reduced-motion`: line pre-drawn, pins unpinned to normal
  flow, count-ups render final values, zero autoplaying loops.
- No layout shift from font swap or entering elements (entrances animate
  from within their reserved box).

---

## 7. Build order (suggested)

1. Static page, full content, tokens, grid: ship-quality with zero JS.
2. Signal line SVG plumbing + scrubbed draw (the concept proves here).
3. Hero + feature grid entrances.
4. Pinned sections 4.2 and 4.4 (the two complex ones), desktop first.
5. Micro-interactions, preloader, reduced-motion pass, perf pass.

Each stage is shippable; the page degrades gracefully if any later
stage is cut.

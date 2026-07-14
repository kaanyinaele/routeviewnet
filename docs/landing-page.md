# RouteViewNet Landing Page (content blueprint)

Copy and structure for the product landing page, written for an AI builder.
Each numbered section is one block on the page, top to bottom. All copy is
final and sourced from the app and [user-guide.md](user-guide.md); keep the
two in sync. Motion and visual direction live in
[landing-page-design.md](landing-page-design.md); section numbers below
reference its choreography where relevant.

Style rules: plain English, honest, no jargon, and never an em dash.

---

## 1. Hero

**Headline:** Know why your internet feels slow.

**Subheadline:** RouteViewNet is a local-first network and memory health
monitor for Linux. It watches your connection and your computer around the
clock, scores their health from 0 to 100, and explains problems in plain
English. No cloud. No account. No telemetry.

**Primary CTA:** Install now (smooth-scrolls to the install block in
section 7)
**Secondary CTA:** Read the User Guide

**Hero visual:** dashboard screenshot showing the health score, the
Gateway / Internet / DNS status cards, and the bandwidth tiles.

**Trust line (under CTA):** Free and open source (MIT). Installs in one
command. Runs entirely on your machine.

---

## 2. The problem (one short paragraph)

"The Wi-Fi is slow" has a dozen possible causes: your router, your
provider, the DNS address book, a bad cable, or a computer that is simply
out of memory. Slow computers and slow internet feel identical, and
guessing means restarting things at random. RouteViewNet measures each
link in the chain continuously, so when something breaks you can see
exactly which step failed and what to try.

---

## 3. Plain words strip

A horizontal strip (marquee or static row) of tiny definition cards, taken
from the user guide's glossary. These prove the product's voice before a
single feature is shown:

- **Router:** the box your internet comes through.
- **DNS:** the internet's address book. When it is slow, every site feels
  slow to start.
- **Latency:** how long a round trip takes. Lower is better.
- **Packet loss:** "5% loss" means 1 in 20 parcels never arrived.
- **Memory:** your computer's working desk.
- **Swap:** emergency overflow on the slow disk. Heavy use is the classic
  reason a computer feels sluggish.

---

## 4. What it does (feature grid, 12 cards)

### One score that means something
A deterministic 0 to 100 health score computed from every check: router
reachability, internet reachability, DNS speed, packet loss, memory, swap,
and processor load. 90 or above is healthy; every lost point is explained.

### The three lights: Gateway, Internet, DNS
The three-step path every web page takes, each checked every few seconds.
Which one fails tells you where the problem lives: your home network, your
provider, or the address book. (Expanded in section 5.)

### Live bandwidth, per connection
Download and upload rates for every network interface, with automatic
detection of the primary one on multi-homed machines. History from 15
minutes to 7 days, plus error and drop counters that expose bad cables,
weak Wi-Fi, and overloaded links.

### Data usage by app
See which programs moved the most data in any time window, with a view of
every app that used 20 MB or more. Counted from the kernel's own
connection counters and attributed to the owning process.

### Response times with a verdict, not a squiggle
Each tested address gets a card named in plain terms ("Your router",
"Internet (Cloudflare)") with a one-word verdict: excellent, good, fair,
slow, or unstable. The verdict uses the typical (median) reply, so one bad
spike cannot skew it; worst reply and answer rate are shown honestly
alongside a small trend line.

### Devices on your network
Every device your computer talks to, with vendor lookup, mDNS names when
devices announce them ("living-room-tv.local"), your router labeled as
such, nicknames you assign, and a trusted flag. An unfamiliar device on
your Wi-Fi raises a gentle alert.

### Memory and load, demystified
System RAM counted honestly (Linux borrows spare memory for speed and
gives it back; that does not count as used), swap pressure, 1/5/15-minute
load against your core count, and the top memory consumers by name, so
the culprit is one glance away.

### Alerts that do not cry wolf
A problem must be seen twice in a row to open an alert, and alerts resolve
themselves when the signal recovers. Debounced, deduplicated, and never
requiring a dismiss button.

### A Troubleshoot page that talks like a person
Rule-based diagnosis of the currently open alerts: a one-line likely
cause, the evidence behind it, and ordered suggested actions. "Your
gateway is fine, DNS is slow; check your resolver."

### Tune it without restarting
Every knob lives in the dashboard: alert sensitivity (memory percentage,
DNS latency, packet loss), check intervals and targets, per-collector
toggles, data retention, and privacy switches. Changes apply the moment
you save; the few that need a restart say so explicitly.

### A real desktop app
RouteViewNet lives in your applications menu with its own window and
icon, not just a browser tab. If the background monitor is ever stopped,
the app offers to start it with your system's normal password prompt.
The browser dashboard at localhost:4545 keeps working too.

### The monitor watches itself
The daemon reports its own memory footprint in the dashboard, so you can
verify the monitoring tool is not the thing slowing you down. It typically
uses less memory than a single browser tab.

---

## 5. Reading the three lights (storytelling section)

This is the page's signature scroll section (design doc section 4.4, the
pinned "three lights" story). Copy for the four beats, straight from the
user guide:

**Gateway ✕.** Your computer cannot even reach your router. The problem is
inside your home: Wi-Fi signal, a cable, or the router itself. Your
internet provider is innocent (this time).

**Gateway ✓ but Internet ✕.** Your router answers, but nothing beyond it
does. Restart the router and modem; if it persists, it is usually an
outage on the provider's side.

**Gateway ✓, Internet ✓, but DNS slow.** You are connected, but the
address book is misbehaving, so pages stall before they even start
loading. The Troubleshoot page suggests concrete steps.

**All three ✓ but things still feel slow.** Check the System page. If
memory is nearly full or swap is heavily used, the network was never the
problem.

---

## 6. The health score, explained (transparency section)

Short intro line: "The score starts at 100 and loses points for each
problem currently detected. Bigger problems cost more, and related
problems are capped so one bad area cannot zero the score alone."

Render as a compact two-column table (mono numerals):

| Problem | Points lost |
|---|---|
| Router unreachable | 40 |
| Internet unreachable | 30 |
| Noticeable packet loss | 15 |
| Website-name lookups failing | 20 |
| Website-name lookups slow | 10 |
| A network connection went down | 15 |
| Network hiccups increasing | 10 |
| Memory nearly full | 15 |
| Heavy swap use | 10 |
| Processor overloaded | 10 |
| Unusual traffic spike | 5 |

Closing line: "90 to 100 is healthy, 60 to 89 is degraded, below 60 is
critical. The exact reasons behind any deduction are always spelled out."

---

## 7. How it works (three steps)

Frame with the two-parts model from the guide: an invisible monitor that
measures around the clock, and a window that just shows what it found.
Closing the window never stops the monitoring.

1. **Install.** One command or one .deb. The background monitor starts
   immediately and survives reboots.
2. **Open.** Launch RouteViewNet from your applications menu (a native
   desktop window) or visit localhost:4545 in a browser.
3. **Understand.** Glance at the score. If something is wrong, the
   Troubleshoot page tells you where and what to try.

### The install block (the page's copy-to-clipboard moment)

A tabbed code panel with three ways in; the one-liner tab is active by
default. This is the target of the hero's "Install now" CTA.

**Tab 1: One-liner (recommended)**

```bash
curl -fsSL https://raw.githubusercontent.com/kaanyinaele/routeviewnet/main/install.sh | sudo sh
```

Small print: "Fetches the latest release from GitHub and installs it.
Cautious? Download install.sh and read it first; it is 60 lines of plain
shell."

**Tab 2: Download the .deb**

```bash
sudo dpkg -i routeviewnet_1.0.0_amd64.deb
```

Small print: "Grab the file from GitHub Releases. The daemon is enabled
and started automatically; remove with dpkg -r (keeps your data) or
dpkg -P (removes everything)."

**Tab 3: Build from source**

```bash
git clone https://github.com/kaanyinaele/routeviewnet.git
cd routeviewnet && make deb
sudo dpkg -i routeviewnet_1.0.0_amd64.deb
```

Small print: "Needs Go 1.24+, Node 20+, and the GTK/WebKit dev headers
(sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev)."

Every tab: Debian/Ubuntu, amd64, systemd. A copy button on each code
block.

---

## 8. Privacy section (own block, this is the differentiator)

**Headline:** Your data never leaves your machine.

- No cloud, no account, no telemetry of any kind. You could unplug your
  internet and RouteViewNet would keep working (and politely tell you the
  internet is down).
- Everything lives in a single SQLite file on your disk; history older
  than 7 days is cleaned up automatically (configurable).
- Process command lines are not stored by default, since they can contain
  secrets. Enabling them is an explicit setting with a warning.
- The dashboard answers only on the computer itself. Opening it to your
  LAN is an explicit setting with a persistent warning, because this
  version has no password protection.
- Defense in depth: host-header validation, WebSocket origin checks,
  strict CORS, a sandboxed systemd service with least-privilege,
  read-only capabilities.

---

## 9. Built for Linux users, honest by design

Two columns.

**Column A, the stack:** a single Go daemon, SQLite storage, a React
dashboard embedded in the binary, systemd-managed, hot-reloadable
settings. Ships with a table of everyday commands (pause, resume, disable
at boot, update, uninstall keeping data, purge) in the user guide.

**Column B, honest limits (from the guide, verbatim tone):**
- It monitors this computer's view of the network, not the whole network.
  It cannot tell you your phone's Wi-Fi is weak.
- Device discovery misses devices that never talk to this computer, and
  IPv6-only neighbors are not shown.
- Per-app data usage counts direct (TCP) connections only; some video
  calls and games are not attributed.
- Processor coverage is the overall load number; it names memory hogs,
  not CPU hogs.
- No authentication and no HTTPS in v1, which is exactly why it stays
  local-only by default.

(Honesty converts better with this audience than overclaiming.)

---

## 10. FAQ (collapsible, answers from the user guide)

**Does it slow down my computer or use up my internet?**
No. The test messages are tiny, and the monitor itself typically uses
less memory than a single browser tab. You can verify both claims on the
System page; it measures itself.

**Do I need to keep the window open?**
No. The monitor runs in the background regardless. The window is only a
viewer; open it once a week and the history will be there.

**Why does "Devices seen" miss some devices?**
It lists devices your computer has recently exchanged messages with, not
everything on the Wi-Fi. This is a deliberate, privacy-friendly design;
the app never scans or probes your network aggressively.

**The memory number looks scary. Is 70% bad?**
Usually not. Linux uses spare memory to make things faster and hands it
back when needed. Worry when memory sits near 100% and swap keeps
growing; that combination is what actually feels slow, and it raises an
alert.

**What are 1.1.1.1 and 8.8.8.8?**
Two well-known, always-online public addresses (run by Cloudflare and
Google) used as "is the internet up?" reference points. Changeable in
Settings.

**Can I see it from my phone?**
Not out of the box: the dashboard only answers on the computer itself,
because this version has no password protection. Opening it to your home
network is possible in Settings, but only do that on a network you trust.

**What Linux versions?**
Debian/Ubuntu with systemd; Go 1.24+ and Node 20+ to build from source.

**Is it really free?**
MIT licensed.

---

## 11. Footer

- Links: User Guide, API docs, Architecture, GitHub repository, License.
- Tagline repeat: One daemon, one SQLite file, one dashboard.
  No cloud, no account, no telemetry.

---

## Page-wide notes for the AI builder

- Dark theme first (matches the dashboard); light theme optional.
- Use real dashboard screenshots, not mockups; the product is the visual.
- Keep every claim verifiable in the app (scores, self-measurement,
  privacy defaults). Nothing aspirational.
- Accent colors from the app: green for healthy, amber for degraded, red
  for critical; status icons always pair with labels, never color alone.
- Verdict words on the page (excellent, good, fair, slow, unstable) must
  match the app's exactly.
- No em dashes anywhere in page copy (project style rule).
- Follow [landing-page-design.md](landing-page-design.md) for the theme,
  GSAP choreography, performance budget, and reduced-motion rules; this
  file owns the words, that file owns the motion.

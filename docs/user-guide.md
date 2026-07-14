# RouteViewNet User Guide

*A friendly guide for people who are new to Linux. No prior networking
knowledge needed.*

---

## 1. What is RouteViewNet?

RouteViewNet watches the two things that make a computer feel slow or
broken, **your internet connection** and **your computer's memory**, and
explains what it sees in plain English.

Instead of you running mysterious terminal commands when the Wi-Fi acts up,
RouteViewNet quietly runs small tests in the background, around the clock:

- Can I reach the router? How fast does it answer?
- Can I reach the internet beyond the router?
- How long do website-name lookups take?
- How much data is flowing in and out?
- How much memory is in use, and which programs are using it?

It turns all of that into one **health score from 0 to 100**, a short list
of **alerts** when something is wrong, and a **Troubleshoot page** that
tells you, in a sentence, where the problem probably is and what to try.

**Everything stays on your computer.** There is no account, no cloud, and
nothing is ever sent anywhere. You could unplug your internet and
RouteViewNet would keep working (and would politely tell you the internet
is down).

---

## 2. Seven words worth knowing

You don't need to memorize these. The app explains itself as you go, but
these words appear often, so here is what they mean in everyday terms:

| Word | What it really means |
|---|---|
| **Router (or gateway)** | The box in your home that your internet comes through. Your computer talks to the router; the router talks to the world. |
| **DNS** | The internet's address book. When you type `wikipedia.org`, DNS turns that name into a numeric address computers understand. When DNS is slow, *every* website feels slow to start, even on fast internet. |
| **Download / Upload** | Download is data coming *into* your computer (watching videos, loading pages). Upload is data going *out* (sending photos, video calls). |
| **Latency (or ping)** | How long a round trip to somewhere takes, in milliseconds (ms). Under 20 ms feels instant; a few hundred ms feels laggy. Lower is better. |
| **Packet loss** | Data travels in small parcels. "5% packet loss" means 1 in 20 parcels never arrived and had to be sent again. It makes calls stutter and games lag. 0% is normal. |
| **Memory (RAM)** | Your computer's working desk. Programs spread out their papers on it while running. When the desk is full, everything slows down. |
| **Swap** | An emergency overflow: when memory is full, Linux moves the least-used papers to a slot on the disk. The disk is far slower than memory, so heavy swap use is the classic reason a computer "feels slow". |

One more: the **monitor** (technically called a *daemon*, pronounced
"demon") is the invisible background helper that does all the measuring.
The window you open is just a viewer; more on that below.

---

## 3. Installing and opening it

RouteViewNet runs on Ubuntu and other Debian-style Linux systems. There
are three ways to install it; pick whichever feels most comfortable.

**The easy way (one command).** Open a terminal, paste this line, and
press Enter:

```bash
curl -fsSL https://raw.githubusercontent.com/kaanyinaele/routeviewnet/main/install.sh | sudo sh
```

It finds the newest version of RouteViewNet on the project's download
page, fetches it, and installs it. (`sudo` asks for your password because
installing software affects the whole computer; that's normal. If you
like to check before you run things, you can open that install.sh link in
a browser first; it is a short, readable script.)

**The download way.** Get the `.deb` file from the project's Releases
page on GitHub, then install it:

```bash
sudo dpkg -i routeviewnet_1.0.0_amd64.deb
```

Most file managers also let you double-click the .deb, though the
terminal command gives clearer messages if something goes wrong.

**The builder's way.** If you want to compile it yourself from the source
code, the README covers that; it needs some developer tools installed.

Whichever way you choose, the install does everything for you: the
background monitor starts immediately and will start itself again after
every reboot. You never need to touch a terminal again for day-to-day
use. Updating later is the same step again; your settings and history
survive updates.

To open the app, look for **RouteViewNet** in your applications menu (the
icon is a small chart with green and blue dots). You can also see the same
dashboard in any web browser at <http://localhost:4545>. "Localhost"
means "this computer", so this address works only on the machine itself.

> **If you see "Waiting for the RouteViewNet daemon…"**: the background
> monitor isn't running. The app will offer to start it and your system
> will show its normal password prompt; approve it and the dashboard
> appears a moment later.

---

## 4. The window and the background monitor

RouteViewNet is two parts:

1. **The monitor**: invisible, always running, taking measurements every
   few seconds and saving them on your computer.
2. **The window**: what you open from the menu. It only *shows* what the
   monitor has collected.

This means:

- **Closing the window never stops the monitoring.** Open it once a week
  or once a month; the history will be there.
- The monitor is deliberately tiny. You can check its own memory use right
  inside the app (System page) to confirm it isn't the thing slowing your
  computer down.
- By default the last **7 days** of measurements are kept; older ones are
  cleaned up automatically.

---

## 5. A tour of the app

Every card in the app has a small **"?" mark** next to its title. Hover
your mouse over it (or reach it with the Tab key) for a one-sentence
explanation. Below is the longer version of each page.

### Overview: "Is everything okay?"

The page to glance at. From top to bottom:

- **Health score**: one number, 0 to 100. It starts at 100 and loses
  points for each problem currently detected. 90 or higher shows as
  *healthy*, 60–89 as *degraded* (something's off but working), and below
  60 as *critical*. The small graph shows the last 24 hours, so you can
  spot "it got worse this afternoon" at a glance.
- **Download / Upload**: how fast data is moving right now on the
  connection your computer actually uses for internet.
- **Devices seen**: how many other devices (phones, TVs, printers…) your
  computer has recently noticed on your home network.
- **Open alerts**: how many problems are active right now. Zero is the
  goal.
- **Gateway / Internet / DNS**: the three-step path every web page
  takes: your router, the wider internet, and the address book. Each shows
  a ✓ or ✕ with a response time. *Which one fails tells you where the
  problem lives*; see section 7.
- **Memory / Load / Daemon memory**: the computer-health side. How full
  the memory is, how busy the processor is, and how little RouteViewNet
  itself is using.
- **Recent events**: the last few things that happened, like an alert
  opening or clearing.

### Traffic: "How much data is flowing?"

A chart of download and upload speed over time; pick a window from 15
minutes to 7 days with the buttons at the top right. Below it, one card per
**network connection** your computer has: Wi-Fi, wired network, and
sometimes virtual ones created by software (you can ignore those). Click a
card to chart that connection. The one marked **primary** is the one your
internet traffic actually uses.

The **data usage by app** card shows which programs moved the most data in
the selected window: the five biggest by default, and a **View all** button
that lists every app that used at least 20 MB. Two honest notes: it counts
the computer's direct connections (a technology called TCP), so some
traffic, like video calls that use other protocols, is not included; and a
connection that starts and finishes entirely between two measurements can
be missed. Treat the numbers as a very good estimate, not an invoice.

The **errors & drops** numbers count network hiccups. Everyone has a few;
numbers that *keep climbing* while you watch can mean a bad cable, weak
Wi-Fi, or an overloaded connection.

### Devices: "What's on my network?"

A list of devices your computer has recently talked to on your home
network: their address, name (when discoverable), and when they were first
and last seen. Your router's row is labeled "your router", and devices
that announce themselves on the network (many phones, TVs, and printers)
show their friendly name, like "living-room-tv.local".

Two handy features:

- **Nickname**: click "+ add" to name a device ("Kitchen TV") so you
  recognize it later.
- **Trust**: mark the devices you know. Anything new shows as
  *new / untrusted* and raises a gentle alert, so an unfamiliar device on
  your Wi-Fi won't go unnoticed.

Honest note: this is a list of devices *your computer has recently
exchanged messages with*, not a full scan of the network. A device that
never talks to your computer may not appear.

### Internet Health: "How good is my connection, really?"

RouteViewNet regularly sends tiny test messages to two well-known,
always-on internet addresses (1.1.1.1 and 8.8.8.8, public services run by
Cloudflare and Google) and to your router. This page shows the results:

- **One card per tested address**, named in plain terms ("Your router",
  "Internet (Cloudflare)"). Each card gives a one-word verdict (excellent,
  good, fair, slow, or unstable), the typical reply time (the middle value
  over the window, so one bad spike does not skew it), the worst reply,
  the share of tests that were answered, and a small trend line where
  lower is better.
- **DNS checks**: each row is one "address book" lookup of a test website
  and how long it took. (The "A" and "AAAA" types are just the older and
  newer address formats; both are normal.)

If a card says **"via tcp"**, RouteViewNet wasn't allowed to use the usual
kind of test message on your system and fell back to a different method;
the results are still meaningful, just measured slightly differently.

### System: "Is my computer itself okay?"

- **Memory used**: how full the working desk is. Don't panic at high-ish
  numbers: Linux deliberately borrows spare memory to speed things up and
  returns it instantly when programs need it. RouteViewNet only counts
  what is *truly* busy.
- **Swap used**: how much of the slow emergency overflow is in use.
  Consistently high swap plus high memory is the classic "everything is
  sluggish" situation. The fix is usually closing (or replacing) the
  hungriest program.
- **Load average**: how busy the processor is, averaged over 1, 5, and
  15 minutes. Rule of thumb: trouble starts when the number stays higher
  than your number of processor cores (shown right next to it).
- **The chart**: memory and swap over time. A line that climbs slowly and
  never comes down can mean some program is "leaking" memory; a restart of
  that program (or the machine) resets it.
- **Top memory consumers**: the programs using the most memory right
  now, biggest first. If your computer feels slow, the culprit is usually
  at the top of this list.

### Alerts: "What went wrong, and when?"

Every problem RouteViewNet detects becomes an alert with a plain-English
message, the time it started, and, once things recover, the time it
cleared. Alerts resolve **automatically**; there is nothing to dismiss.

To avoid crying wolf, an alert only opens after the problem is seen **twice
in a row**, and only clears after things look fine twice in a row. So a
single half-second blip won't page you, but a real outage shows up within
seconds.

### Troubleshoot: "Just tell me what to do."

The page to open when something feels wrong. It looks at all currently
open alerts together and gives you:

- **A one-line diagnosis**, for example: *"Your router is fine, but the
  internet beyond it is unreachable; this points at your internet
  provider."*
- **Evidence**: the measurements that led to that conclusion, in plain
  words.
- **Suggested actions**: a short, ordered list of things to try, starting
  with the easiest.

It updates by itself as conditions change; if you restart the router and
things recover, the page will say so.

### Settings: "Tune it (or leave it alone)."

The defaults are sensible; most people never need this page. Everything
here applies immediately when you save (no restart needed), except the
few items the app explicitly flags. Things you *might* care about:

- **How often to measure** (every 5 seconds by default) and which
  measurements to take.
- **Which addresses to test** against.
- **Alert sensitivity**: for example, at what memory percentage to warn
  (90% by default), or what counts as slow DNS (half a second).
- **Privacy**: see the next section.

---

## 6. The health score, explained

The score starts at 100 and loses points for each *currently open* alert.
Bigger problems cost more:

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

Related problems are grouped so one bad area can't zero the score by
itself; for example, all connection problems together can cost at most
40 points. The same problem also only counts once, no matter how many
devices report it. The result: **90–100 = healthy · 60–89 = degraded
· below 60 = critical**, and the exact reasons behind any deduction are
always spelled out on the Troubleshoot page.

---

## 7. "My internet is slow": reading the three lights

The Gateway / Internet / DNS trio on the Overview page is a built-in
process of elimination. Four common patterns:

**Gateway ✕**: your computer can't even reach your router. The problem
is *inside your home*: Wi-Fi signal, a cable, or the router itself.
Your internet provider is innocent (this time). Try moving closer to the
router, checking cables, or restarting the router.

**Gateway ✓ but Internet ✕**: your router answers, but nothing beyond it
does. The problem is *between your router and your provider*. Restart the
router and modem; if it persists, it's usually an outage on the provider's
side. Check their status page (on your phone, using mobile data).

**Gateway ✓, Internet ✓, but DNS slow or failing**: you're connected,
but the address book is misbehaving, so pages stall before they even start
loading. This often comes from the router's or provider's DNS service. The
Troubleshoot page will suggest concrete steps.

**All three ✓ but things still feel slow**: check the System page. Slow
computers and slow internet feel identical; if memory is nearly full or
swap is heavily used, the network was never the problem.

---

## 8. Your privacy and your data

- **Nothing leaves your computer.** No accounts, no telemetry, no cloud.
- All measurements live in a single database file on your machine, at
  `/var/lib/routeviewnet/`, and anything older than 7 days is deleted
  automatically (you can change the retention in Settings).
- The names of programs using memory are recorded, but the **full commands
  that started them are not**, because those can accidentally contain
  passwords. (There is a setting to enable it, with a warning, if you ever
  need that level of detail.)
- The dashboard is only reachable **from the computer itself**. There is
  an advanced setting to open it to your home network; the app shows a
  persistent warning if you do, because this version has no password
  protection.

---

## 9. Everyday tasks

You shouldn't need these often, but for reference; each is one line in a
terminal:

| I want to… | Command |
|---|---|
| Pause monitoring until next boot | `sudo systemctl stop routeviewnetd` |
| Resume monitoring | `sudo systemctl start routeviewnetd` |
| Stop it from starting at boot | `sudo systemctl disable routeviewnetd` |
| Check whether the monitor is running | `systemctl status routeviewnetd` |
| Update to a newer version | re-run the install one-liner from section 3, or `sudo dpkg -i <new .deb file>` (settings and history survive) |
| Uninstall, but keep my history & settings | `sudo dpkg -r routeviewnet` |
| Uninstall and erase everything | `sudo dpkg -P routeviewnet` |

---

## 10. Questions people ask

**Does it slow down my computer or use up my internet?**
No. The test messages are tiny (a few per second at most), and the monitor
itself typically uses less memory than a single browser tab. You can
verify both claims on the System page; it measures itself.

**Do I need to keep the window open?**
No. The monitor runs regardless. The window is only a viewer.

**Why does "Devices seen" miss some devices?**
It lists devices your computer has recently exchanged messages with, not
everything on the Wi-Fi. Quiet devices that never talk to your computer
won't show up. This is a deliberate, privacy-friendly design; the app
never scans or probes your network aggressively.

**The memory number looks scary. Is 70% bad?**
Usually not. Linux uses spare memory to make things faster and hands it
back when needed. Worry when memory sits near 100% *and* swap keeps
growing; that combination is what actually feels slow, and it raises an
alert.

**What are 1.1.1.1 and 8.8.8.8?**
Two well-known, always-online public addresses (run by Cloudflare and
Google) that are conventionally used as "is the internet up?" reference
points. You can change them in Settings.

**Can I check my computer from my phone?**
Not out of the box: the dashboard only answers on the computer itself,
because this version has no password protection. Opening it to your home
network is possible in Settings, but only do that on a network you trust.

**I closed an alert's problem but the alert is still there.**
Give it a few seconds. Alerts clear automatically after the measurement
comes back healthy twice in a row.

---

## 11. What RouteViewNet can't do (yet)

Honesty section: current limits of version 1.

- It monitors **this computer's view** of the network, not the whole
  network. It can't tell you your phone's Wi-Fi is weak.
- Device discovery misses devices that never talk to this computer, and
  doesn't list newer-format (IPv6-only) neighbors.
- Processor coverage is the overall "how busy" number only; it won't name
  which program is hogging the processor (it *does* name memory hogs).
- Per-app data usage counts direct (TCP) connections only. Traffic using
  other protocols, like many video calls and some games, is not attributed
  to an app.
- No password protection and no encrypted access, which is exactly why
  it stays local-only by default.
- Linux only, Debian-style packages only, for now.

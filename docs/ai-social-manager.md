# AI Social Media Manager: Promoting RouteViewNet

A concept spec for a small AI-driven tool whose only job is to run
RouteViewNet's social media presence: draft posts, keep a content
calendar, adapt one message into per-platform formats, and report back
on what's working. This is a companion to
[landing-page.md](landing-page.md) (site) and
[landing-page-design.md](landing-page-design.md) (visual language) —
the social presence should feel like the same product talking, not a
separate marketing voice.

---

## 1. Why this exists

RouteViewNet is a local-first, no-account, no-telemetry tool built by
one person. It won't out-market ad-budget competitors, so the social
strategy has to win on **authenticity and cadence**: real build
updates, real "your gateway is fine, DNS is slow" moments, real Linux
users nerding out — posted consistently, without eating the maintainer's
time. The AI manager's whole value proposition is protecting that time
while keeping the cadence up.

**Non-goals:** growth hacking, engagement-bait, buying followers, or
posting anything the maintainer wouldn't personally stand behind. If a
generated post sounds like it's trying too hard, it's wrong for this
product.

---

## 2. Brand voice (carried over from the product)

| Trait | Do | Don't |
|---|---|---|
| Calm confidence | "Your gateway is fine. DNS is slow." | "🚨 Is your WiFi BROKEN?? 🚨" |
| Terminal heritage | Monospace snippets, real CLI output, real config | Stock photos of servers/clouds |
| Zero noise | One idea per post | Threads that pad a single fact into 10 tweets |
| Honest about limits | Mentions v1 limitations openly (see README) | Overclaiming features it doesn't have |
| No hype | "local-first", "no cloud", "one SQLite file" | "revolutionary", "game-changing" |

Mood keywords carried from the landing page spec: *instrument panel,
oscilloscope, mission control for your Wi-Fi* — not "crypto dashboard."

---

## 3. Platforms and role of each

| Platform | Purpose | Cadence |
|---|---|---|
| **Mastodon** (`fosstodon.org` or similar) | Primary home — this audience *is* the audience (self-hosting, privacy, Linux) | 3–4×/week |
| **X/Twitter** | Reach + screenshots/GIFs of the dashboard, release announcements | 2–3×/week |
| **Reddit** (r/selfhosted, r/linux, r/homelab, r/networking) | Longer-form, one real post per relevant release or milestone, never auto-posted | Manual approval only, ~monthly |
| **Hacker News** | Launch/major-release "Show HN" posts only | Per major version, manual only |
| **GitHub Discussions/Releases** | Source of truth the AI drafts *from*, not a promo channel | N/A |

Reddit and HN posts are **never** auto-published — see §6. Communities
there are quick to smell automated marketing, and one bad post can burn
the account permanently.

---

## 4. Content pillars

1. **Release notes → posts.** Every changelog entry becomes a
   candidate post: "what changed" in plain English, matching the
   README's no-jargon style.
2. **"Health score" moments.** Short posts built around one concrete
   diagnostic scenario (packet loss vs. DNS latency vs. gateway down),
   optionally with a dashboard screenshot.
3. **Build-in-public.** Short notes on what's being worked on this
   week, pulled from commit messages/PR titles — filtered for anything
   interesting to a human, not "bump dependency."
4. **Security/privacy posture.** Recurring reminders of what
   RouteViewNet *doesn't* do (no cloud, no account, no telemetry,
   binds to localhost) — this is the product's actual differentiator.
5. **User-facing tips.** One feature of the dashboard explained per
   post (device nicknames, the troubleshoot page, per-app bandwidth).

---

## 5. System architecture

```
                 ┌───────────────────────┐
   GitHub repo → │  Source watcher        │  (releases, merged PRs,
   (webhook/poll)│  internal/promoagent/  │   CHANGELOG diffs)
                 └──────────┬────────────┘
                            │ "raw events"
                            ▼
                 ┌───────────────────────┐
                 │  Draft generator (LLM) │  turns one event into
                 │  brand-voice prompt +  │  3 platform variants
                 │  few-shot post examples│  (Mastodon/X/long-form)
                 └──────────┬────────────┘
                            │ drafts
                            ▼
                 ┌───────────────────────┐
                 │  Review queue (human)  │  every draft needs a
                 │  Slack/Telegram/CLI    │  ✅ before it can post
                 └──────────┬────────────┘
                            │ approved
                            ▼
                 ┌───────────────────────┐
                 │  Scheduler + poster    │  rate-limits, spacing,
                 │  (per-platform APIs)   │  best-time-of-day
                 └──────────┬────────────┘
                            │
                            ▼
                 ┌───────────────────────┐
                 │  Metrics collector     │  impressions/replies/
                 │  → weekly digest       │  clicks → what to repeat
                 └───────────────────────┘
```

This mirrors RouteViewNet's own philosophy: small, local-first,
single binary where possible, no unnecessary cloud dependency for a
one-person project. A lightweight Go service (reusing patterns from
`internal/scheduler/` — interval loops, hot-reload config) plus a
thin LLM-calling layer is enough; no need for a heavyweight agent
framework.

### Components

- **Source watcher** — polls the GitHub repo (or listens on a webhook)
  for new releases, merged PRs, and CHANGELOG changes. Emits structured
  "events" (`{type: release, version, summary}`).
- **Draft generator** — one LLM call per event per platform, using a
  fixed system prompt encoding the voice table in §2, few-shot examples
  of past *approved* posts, and hard constraints (character limits, no
  emoji spam, no unverifiable claims).
- **Review queue** — every single draft requires human approval before
  it can be scheduled. No auto-post, ever, at launch. This is a
  deliberate constraint, not a v2 feature to remove — see §6.
- **Scheduler** — spacing rules (no more than 1 post/platform/day
  unless it's a release), platform-specific posting windows, basic
  A/B of two phrasings when useful.
- **Metrics collector** — pulls public engagement stats via each
  platform's API, produces a weekly digest ("these 3 posts did best,
  here's the pattern") that also feeds back into future prompts.

---

## 6. Guardrails (the important part)

- **Human-in-the-loop by default.** Every post is drafted, never
  auto-published, until there's a long track record of the AI's drafts
  needing zero edits. Even then, Reddit/HN stay manual forever.
- **No fabricated metrics or testimonials.** The AI can summarize real
  GitHub stars/issues/download counts; it must never invent user quotes
  or numbers.
- **No engagement-bait patterns.** No "comment below," no fake
  urgency, no controversy-farming.
- **Consistent with README honesty.** If a post touches a feature
  covered by "Honest limitations" in the README, the draft generator
  is required to reflect that limitation, not paper over it.
- **Rate limits and kill switch.** Scheduler respects each platform's
  API rate limits; a single config flag (`enabled: false`) halts all
  posting immediately, mirroring the "hot-reloadable settings" pattern
  already used in RouteViewNet's own config.
- **No credentials in the repo.** Platform API tokens live in the
  operator's secrets store, never committed — same bar as the main
  project's "process command lines are not stored by default" stance
  on sensitive data.

---

## 7. Example generated drafts (illustrative, not final copy)

**Mastodon, release post:**
> RouteViewNet v1.2 is out. Multi-homed machines now get correct
> primary-interface detection, so the health score stops guessing which
> NIC actually matters. One daemon, one SQLite file, still no cloud.
> Changelog → [link]

**X, feature post:**
> "Your gateway is fine. DNS is slow." That's the kind of sentence
> RouteViewNet's troubleshoot page gives you instead of a chart you
> have to interpret yourself.

**Mastodon, privacy-posture post:**
> No account. No telemetry. No cloud. RouteViewNet is one Go daemon
> and one SQLite file on your machine, dashboard at
> `localhost:4545`. That's the whole architecture.

---

## 8. Build phases

1. **Phase 0 (manual):** maintainer posts by hand using the voice
   guide in §2 as a checklist. Establishes the few-shot examples the
   AI will later learn from.
2. **Phase 1:** source watcher + draft generator + review queue.
   Everything still posted manually after AI drafts it — this phase
   just removes the "stare at a blank compose box" step.
3. **Phase 2:** scheduler automates timing for *approved* drafts only.
4. **Phase 3:** metrics digest closes the loop and starts informing
   which content pillars (§4) get emphasized.

Reddit/HN posting is never automated past Phase 0 — those stay a
manual, occasional, high-effort action by the maintainer.

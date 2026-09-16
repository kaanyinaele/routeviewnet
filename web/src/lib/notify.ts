// Browser notifications for alert transitions.
//
// These complement the webhook rather than replacing it: a browser
// notification only fires while a dashboard tab is open, so it solves "don't
// make me watch the tab", not "tell me while I'm away".
//
// The Notifications API is also gated on a secure context. Browsers treat
// localhost as trustworthy, so http://localhost:4545 works — but the LAN mode
// (server.host: 0.0.0.0, reached at http://192.168.x.x:4545) is plain HTTP on
// a non-local origin, where the Notification constructor is not exposed at
// all. That is exactly the setup where remote alerting matters most, so the
// support state is surfaced to the Settings page instead of leaving a toggle
// that silently does nothing.

const PREF_KEY = "routeviewnet.notifications";

export type NotifySupport = "ok" | "insecure-context" | "unsupported";

export interface AlertLike {
  rule_key: string;
  severity: string;
  title: string;
  message: string;
  source: string;
}

export function notifySupport(): NotifySupport {
  if (typeof window === "undefined") return "unsupported";
  if (typeof Notification === "undefined") {
    // A non-secure context hides the constructor entirely, which is the
    // common case here — distinguish it so the UI can explain why.
    return window.isSecureContext ? "unsupported" : "insecure-context";
  }
  return "ok";
}

export function notifyPermission(): NotificationPermission | "unavailable" {
  return notifySupport() === "ok" ? Notification.permission : "unavailable";
}

// The stored preference is per-browser on purpose: notification permission is
// itself per-browser, so syncing this through the daemon would promise a
// consistency the platform cannot deliver.
export function notifyPreferred(): boolean {
  try {
    return localStorage.getItem(PREF_KEY) === "on";
  } catch {
    return false; // private windows and blocked site data throw on access
  }
}

export function setNotifyPreferred(on: boolean): void {
  try {
    localStorage.setItem(PREF_KEY, on ? "on" : "off");
  } catch {
    /* preference simply will not persist; notifications still work */
  }
}

export function notifyEnabled(): boolean {
  return notifySupport() === "ok" && Notification.permission === "granted" && notifyPreferred();
}

export async function requestNotifyPermission(): Promise<NotificationPermission | "unavailable"> {
  if (notifySupport() !== "ok") return "unavailable";
  try {
    return await Notification.requestPermission();
  } catch {
    return "denied";
  }
}

// showAlertNotification renders one alert transition. Repeat notifications for
// the same rule and source replace the previous one rather than stacking, so a
// flapping interface cannot bury the screen in toasts.
export function showAlertNotification(event: string, alert: AlertLike): void {
  if (!notifyEnabled()) return;
  const resolved = event === "alert.resolved";
  try {
    const n = new Notification(resolved ? `Resolved: ${alert.title}` : alert.title, {
      body: alert.message,
      tag: `routeviewnet:${alert.rule_key}:${alert.source}`,
      // A critical alert stays up until dismissed; everything else times out
      // the way the platform prefers.
      requireInteraction: !resolved && alert.severity === "critical",
    });
    n.onclick = () => {
      window.focus();
      if (location.pathname !== "/alerts") location.assign("/alerts");
      n.close();
    };
  } catch {
    /* a malformed or blocked notification must never break the live feed */
  }
}

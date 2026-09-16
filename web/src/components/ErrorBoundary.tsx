import { Component, ErrorInfo, ReactNode } from "react";

// A render crash anywhere below this leaves a blank white page, which is the
// least diagnosable failure the dashboard can produce — the user cannot tell
// it apart from the daemon being down, and the sidebar disappears with it, so
// there is not even a link to another page.
//
// Kept as a class because that is still the only way to catch a render error;
// the alternative (a data router with errorElement) would mean restructuring
// the routes in App.tsx for no extra benefit here.
export class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // The message is the one thing worth keeping for a bug report; the
    // component stack goes with it so the page can be identified.
    console.error("dashboard render error:", error, info.componentStack);
  }

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <div className="flex min-h-screen items-center justify-center p-6">
        <div
          className="card max-w-md p-5 text-sm"
          style={{ color: "var(--ink-secondary)" }}
        >
          <div className="mb-2 text-base font-semibold" style={{ color: "var(--ink-primary)" }}>
            This page stopped working
          </div>
          <p>
            Something in the dashboard hit an error it could not recover from. Your monitoring is
            unaffected — the background monitor keeps recording while this window is broken.
          </p>
          <button
            onClick={() => window.location.reload()}
            className="mt-4 rounded-lg px-3 py-1.5 text-sm font-medium"
            style={{ background: "var(--series-1)", color: "#fff" }}
          >
            Reload the dashboard
          </button>
          <details className="mt-3">
            <summary className="cursor-pointer text-xs" style={{ color: "var(--ink-muted)" }}>
              Technical details
            </summary>
            <pre className="mt-2 overflow-auto text-xs" style={{ color: "var(--ink-muted)" }}>
              {this.state.error.message}
            </pre>
          </details>
        </div>
      </div>
    );
  }
}

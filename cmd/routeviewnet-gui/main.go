// routeviewnet-gui is the desktop window for RouteViewNet.
//
// The daemon (routeviewnetd) owns all data collection and serves the
// dashboard on 127.0.0.1:4545; this program is a thin native WebKitGTK
// window onto that dashboard, so closing it never interrupts monitoring.
// If the daemon is not up yet it shows a waiting page and connects
// automatically once the API answers.
//
// It deliberately does not read /etc/routeviewnet/config.yaml (that file
// is 0600 root:routeviewnet); a non-default address is passed with --url.
package main

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <stdlib.h>
#include <string.h>
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

static WebKitWebView *rvn_view;

static void rvn_on_destroy(GtkWidget *widget, gpointer data) {
	(void)widget; (void)data;
	gtk_main_quit();
}

static int rvn_init(void) {
	return gtk_init_check(NULL, NULL) ? 1 : 0;
}

// rvn_start builds the window and shows either uri (daemon reachable) or
// the inline html waiting page. Must be called on the GTK main thread,
// before rvn_main.
static void rvn_start(const char *title, const char *uri, const char *html,
                      int width, int height) {
	GtkWidget *window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
	gtk_window_set_title(GTK_WINDOW(window), title);
	gtk_window_set_default_size(GTK_WINDOW(window), width, height);
	gtk_window_set_icon_name(GTK_WINDOW(window), "routeviewnet");
	g_signal_connect(window, "destroy", G_CALLBACK(rvn_on_destroy), NULL);

	rvn_view = WEBKIT_WEB_VIEW(webkit_web_view_new());
	gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(rvn_view));

	if (uri != NULL) {
		webkit_web_view_load_uri(rvn_view, uri);
	} else {
		webkit_web_view_load_html(rvn_view, html, NULL);
	}
	gtk_widget_show_all(window);
}

static gboolean rvn_navigate_idle(gpointer data) {
	webkit_web_view_load_uri(rvn_view, (const char *)data);
	free(data);
	return G_SOURCE_REMOVE;
}

// rvn_navigate is safe to call from any thread.
static void rvn_navigate(const char *uri) {
	gdk_threads_add_idle(rvn_navigate_idle, strdup(uri));
}

static void rvn_main(void) {
	gtk_main();
}
*/
import "C"

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unsafe"
)

// GTK requires init and the main loop to run on the same OS thread.
func init() {
	runtime.LockOSThread()
}

const waitingHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
  body { background:#0f172a; color:#e2e8f0; font-family:system-ui,sans-serif;
         display:flex; align-items:center; justify-content:center;
         height:100vh; margin:0; }
  .card { text-align:center; max-width:34rem; padding:2rem; }
  h1 { font-size:1.4rem; font-weight:600; margin-bottom:.75rem; }
  p  { color:#94a3b8; line-height:1.6; }
  code { background:#1e293b; border-radius:.375rem; padding:.2rem .5rem;
         color:#38bdf8; font-size:.95em; }
  .dot { display:inline-block; width:.6rem; height:.6rem; border-radius:50%;
         background:#38bdf8; margin-right:.5rem;
         animation:pulse 1.2s ease-in-out infinite; }
  @keyframes pulse { 50% { opacity:.25; } }
</style></head><body><div class="card">
  <h1><span class="dot"></span>Waiting for the RouteViewNet daemon&hellip;</h1>
  <p>The dashboard will open automatically as soon as the daemon is
     reachable. If a password prompt appeared, authorizing it starts the
     daemon now, or start it yourself with:</p>
  <p><code>sudo systemctl enable --now routeviewnetd</code></p>
</div></body></html>`

// tryStartDaemon asks the system to start the local daemon via a graphical
// polkit prompt. Best-effort: a cancelled or failed prompt simply leaves
// the waiting page up, and the manual command still applies.
func tryStartDaemon() {
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return
	}
	_ = exec.Command(pkexec, "systemctl", "start", "routeviewnetd.service").Run()
}

func daemonUp(healthURL string, timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(healthURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

const defaultURL = "http://127.0.0.1:4545/"

func main() {
	url := flag.String("url", defaultURL,
		"dashboard URL to open (daemon server address)")
	flag.Parse()

	if C.rvn_init() == 0 {
		fmt.Fprintln(os.Stderr, "routeviewnet-gui: cannot open display")
		os.Exit(1)
	}

	dashURL := *url
	healthURL := strings.TrimRight(dashURL, "/") + "/api/v1/health"

	title := C.CString("RouteViewNet")
	defer C.free(unsafe.Pointer(title))

	if daemonUp(healthURL, 400*time.Millisecond) {
		curi := C.CString(dashURL)
		C.rvn_start(title, curi, nil, 1280, 860)
		C.free(unsafe.Pointer(curi))
	} else {
		chtml := C.CString(waitingHTML)
		C.rvn_start(title, nil, chtml, 1280, 860)
		C.free(unsafe.Pointer(chtml))
		go func() {
			// Only offer to start the local service when targeting it.
			if dashURL == defaultURL {
				tryStartDaemon()
			}
			for !daemonUp(healthURL, time.Second) {
				time.Sleep(700 * time.Millisecond)
			}
			curi := C.CString(dashURL)
			C.rvn_navigate(curi)
			C.free(unsafe.Pointer(curi))
		}()
	}

	C.rvn_main()
}

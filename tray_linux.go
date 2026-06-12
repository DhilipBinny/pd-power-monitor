//go:build linux

package main

/*
#cgo pkg-config: ayatana-appindicator3-0.1
#include <libayatana-appindicator/app-indicator.h>

static AppIndicator* create_indicator(const char *id, const char *icon) {
	return app_indicator_new(id, icon, APP_INDICATOR_CATEGORY_HARDWARE);
}
static void indicator_set_active(AppIndicator *ind) {
	app_indicator_set_status(ind, APP_INDICATOR_STATUS_ACTIVE);
}
static void indicator_set_menu(AppIndicator *ind, GtkWidget *menu) {
	app_indicator_set_menu(ind, GTK_MENU(menu));
}
static void indicator_set_label(AppIndicator *ind, const char *label) {
	app_indicator_set_label(ind, label, "");
}

// Emit standard PropertiesChanged signal so GNOME Shell's AppIndicator
// extension picks up XAyatanaLabel. The extension's GDBusProxy doesn't
// receive XAyatanaNewLabel (not in interface XML), but it always listens
// for the standard PropertiesChanged signal.
#include <gio/gio.h>
// NOTE: the object path is the indicator id "power-monitor" with dashes
// mapped to underscores by libayatana — renaming the id breaks this emit.
static void emit_label_changed(const char *label) {
	// g_bus_get_sync returns a singleton; cache our ref instead of
	// ref/unref churn every tick
	static GDBusConnection *conn = NULL;
	if (!conn) {
		GError *err = NULL;
		conn = g_bus_get_sync(G_BUS_TYPE_SESSION, NULL, &err);
		if (!conn) {
			if (err) g_error_free(err);
			return;
		}
	}
	GVariantBuilder changed;
	g_variant_builder_init(&changed, G_VARIANT_TYPE("a{sv}"));
	g_variant_builder_add(&changed, "{sv}", "XAyatanaLabel",
		g_variant_new_string(label));

	GVariantBuilder invalidated;
	g_variant_builder_init(&invalidated, G_VARIANT_TYPE("as"));

	g_dbus_connection_emit_signal(conn, NULL,
		"/org/ayatana/NotificationItem/power_monitor",
		"org.freedesktop.DBus.Properties",
		"PropertiesChanged",
		g_variant_new("(sa{sv}as)",
			"org.kde.StatusNotifierItem", &changed, &invalidated),
		NULL);
}

#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>

extern void goOnQuit();

static GtkWidget* new_menu() { return gtk_menu_new(); }
static GtkWidget* new_separator() { return gtk_separator_menu_item_new(); }

static GtkWidget* new_label_item(const char *label) {
	GtkWidget *item = gtk_menu_item_new_with_label(label);
	gtk_widget_set_sensitive(item, FALSE);
	return item;
}

static GtkWidget* new_quit_item() {
	GtkWidget *item = gtk_menu_item_new_with_label("Quit");
	g_signal_connect_swapped(item, "activate", G_CALLBACK(goOnQuit), NULL);
	return item;
}

static void menu_append(GtkWidget *menu, GtkWidget *item) {
	gtk_menu_shell_append(GTK_MENU_SHELL(menu), item);
}

static void menu_insert(GtkWidget *menu, GtkWidget *item, gint pos) {
	gtk_menu_shell_insert(GTK_MENU_SHELL(menu), item, pos);
}

// Quit must be marshaled onto the GTK main loop: the signal handler runs on
// a Go goroutine, and a direct gtk_main_quit before gtk_main() starts is
// silently lost. An idle source queued early fires as soon as the loop runs.
static gboolean quit_idle(gpointer data) { gtk_main_quit(); return FALSE; }
static void schedule_quit(void) { g_idle_add(quit_idle, NULL); }

static void menu_remove(GtkWidget *menu, GtkWidget *item) {
	gtk_container_remove(GTK_CONTAINER(menu), item);
}

static void set_item_label(GtkWidget *item, const char *label) {
	gtk_menu_item_set_label(GTK_MENU_ITEM(item), label);
}

static void show_all(GtkWidget *w) { gtk_widget_show_all(w); }

static guint add_timeout(guint interval) {
	extern gboolean goOnUpdate();
	return g_timeout_add(interval, (GSourceFunc)goOnUpdate, NULL);
}

*/
import "C"

var trayInstance *LinuxTray

type LinuxTray struct {
	source     PowerSource
	indicator  *C.AppIndicator
	menu       *C.GtkWidget
	portItems  []*C.GtkWidget
	separator1 *C.GtkWidget
	itemBat    *C.GtkWidget
	itemTotal  *C.GtkWidget
	itemThresh *C.GtkWidget

	// last rendered state, to skip cgo/GTK work when nothing changed
	lastState DisplayState
	rendered  bool
}

// Port rows are inserted after "Power Monitor" + separator.
const portInsertIndex = 2

func NewTray() TrayUI {
	t := &LinuxTray{}
	trayInstance = t
	return t
}

func setItemLabel(item *C.GtkWidget, label string) {
	withCStr(label, func(cs *C.char) {
		C.set_item_label(item, cs)
	})
}

func (t *LinuxTray) Init(source PowerSource) {
	t.source = source

	C.gtk_init(nil, nil)

	withCStr("power-monitor", func(id *C.char) {
		withCStr("thunderbolt-symbolic", func(icon *C.char) {
			t.indicator = C.create_indicator(id, icon)
		})
	})
	C.indicator_set_active(t.indicator)

	t.menu = C.new_menu()

	withCStr("Power Monitor", func(cs *C.char) {
		C.menu_append(t.menu, C.new_label_item(cs))
	})
	C.menu_append(t.menu, C.new_separator())

	// Port rows are inserted at portInsertIndex by the first update()

	t.separator1 = C.new_separator()
	C.menu_append(t.menu, t.separator1)

	withCStr("Battery: --", func(cs *C.char) {
		t.itemBat = C.new_label_item(cs)
		C.menu_append(t.menu, t.itemBat)
	})

	withCStr("Power input: --", func(cs *C.char) {
		t.itemTotal = C.new_label_item(cs)
		C.menu_append(t.menu, t.itemTotal)
	})

	withCStr("Charge range: --", func(cs *C.char) {
		t.itemThresh = C.new_label_item(cs)
		C.menu_append(t.menu, t.itemThresh)
	})

	C.menu_append(t.menu, C.new_separator())
	C.menu_append(t.menu, C.new_quit_item())

	C.show_all(t.menu)
	C.indicator_set_menu(t.indicator, t.menu)
}

func (t *LinuxTray) rebuildPortItems(ports []USBCPort) {
	// Remove old port items from menu
	for _, item := range t.portItems {
		C.menu_remove(t.menu, item)
	}
	t.portItems = nil

	for i, p := range ports {
		withCStr(p.Name, func(cs *C.char) {
			item := C.new_label_item(cs)
			// Insert into the port section; appending would land below Quit
			C.menu_insert(t.menu, item, C.gint(portInsertIndex+i))
			t.portItems = append(t.portItems, item)
		})
	}
}

func (t *LinuxTray) update() {
	// One snapshot per tick; rebuildPortItems reuses the same slice so the
	// menu rows and the computed labels can't come from different reads
	ports := t.source.USBCPorts()
	bat := t.source.Battery()
	ac := t.source.ACOnline()

	if len(ports) != len(t.portItems) {
		t.rebuildPortItems(ports)
		C.show_all(t.menu)
		// show_all re-shows the hidden thresh row; force a full re-render
		t.rendered = false
	}

	state := ComputeDisplay(ports, bat, ac)
	prev, force := t.lastState, !t.rendered

	for i, label := range state.PortLabels {
		if i >= len(t.portItems) {
			break
		}
		if force || i >= len(prev.PortLabels) || prev.PortLabels[i] != label {
			setItemLabel(t.portItems[i], label)
		}
	}

	if force || prev.BatLabel != state.BatLabel {
		setItemLabel(t.itemBat, state.BatLabel)
	}
	if force || prev.TotalLabel != state.TotalLabel {
		setItemLabel(t.itemTotal, state.TotalLabel)
	}
	if force || prev.ThreshLabel != state.ThreshLabel {
		// Hide the row entirely on machines without charge thresholds
		if state.ThreshLabel != "" {
			setItemLabel(t.itemThresh, state.ThreshLabel)
			C.gtk_widget_show(t.itemThresh)
		} else {
			C.gtk_widget_hide(t.itemThresh)
		}
	}

	if force || prev.BarLabel != state.BarLabel {
		// GNOME's bar label needs space padding so the text doesn't touch
		// the neighboring status icons
		withCStr("  "+state.BarLabel+"  ", func(cs *C.char) {
			C.indicator_set_label(t.indicator, cs)
			C.emit_label_changed(cs)
		})
	}

	t.lastState = state
	t.rendered = true
}

func (t *LinuxTray) Run() {
	C.add_timeout(C.guint(refreshPeriod.Milliseconds()))
	t.update()
	C.gtk_main()
}

func (t *LinuxTray) Quit() {
	C.schedule_quit()
}

//export goOnQuit
func goOnQuit() {
	if trayInstance != nil {
		trayInstance.Quit()
	}
}

//export goOnUpdate
func goOnUpdate() C.gboolean {
	if trayInstance != nil {
		trayInstance.update()
	}
	return C.TRUE
}

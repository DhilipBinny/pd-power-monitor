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
import "unsafe"

const refreshInterval = 3000

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
}

func NewTray() TrayUI {
	t := &LinuxTray{}
	trayInstance = t
	return t
}

func withCStr(s string, fn func(*C.char)) {
	cs := C.CString(s)
	fn(cs)
	C.free(unsafe.Pointer(cs))
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

	t.rebuildPortItems()

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

	withCStr("Charge limit: --", func(cs *C.char) {
		t.itemThresh = C.new_label_item(cs)
		C.menu_append(t.menu, t.itemThresh)
	})

	C.menu_append(t.menu, C.new_separator())
	C.menu_append(t.menu, C.new_quit_item())

	C.show_all(t.menu)
	C.indicator_set_menu(t.indicator, t.menu)
}

func (t *LinuxTray) rebuildPortItems() {
	// Remove old port items from menu
	for _, item := range t.portItems {
		C.menu_remove(t.menu, item)
	}
	t.portItems = nil

	ports := t.source.USBCPorts()
	for _, p := range ports {
		withCStr(p.Name, func(cs *C.char) {
			item := C.new_label_item(cs)
			// Insert before the separator
			C.menu_append(t.menu, item)
			t.portItems = append(t.portItems, item)
		})
	}
}

func (t *LinuxTray) update() {
	ports := t.source.USBCPorts()
	bat := t.source.Battery()
	ac := t.source.ACOnline()

	if len(ports) != len(t.portItems) {
		t.rebuildPortItems()
		C.show_all(t.menu)
	}

	state := ComputeDisplay(ports, bat, ac)

	for i, label := range state.PortLabels {
		if i < len(t.portItems) {
			setItemLabel(t.portItems[i], label)
		}
	}

	setItemLabel(t.itemBat, state.BatLabel)
	setItemLabel(t.itemTotal, state.TotalLabel)
	if state.ThreshLabel != "" {
		setItemLabel(t.itemThresh, state.ThreshLabel)
	}

	withCStr(state.BarLabel, func(cs *C.char) {
		C.indicator_set_label(t.indicator, cs)
	})
}

func (t *LinuxTray) Run() {
	C.add_timeout(refreshInterval)
	t.update()
	C.gtk_main()
}

func (t *LinuxTray) Quit() {
	C.gtk_main_quit()
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

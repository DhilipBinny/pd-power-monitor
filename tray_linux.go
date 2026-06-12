//go:build linux

package main

// Pure-Go system tray: implements the StatusNotifierItem and dbusmenu
// D-Bus protocols directly, with the Ayatana label extension GNOME's
// AppIndicator extension renders next to the icon. No GTK, no cgo —
// linux binaries cross-compile from any platform with CGO_ENABLED=0.

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

const (
	sniPath      = dbus.ObjectPath("/StatusNotifierItem")
	menuPath     = dbus.ObjectPath("/MenuBar")
	sniInterface = "org.kde.StatusNotifierItem"
	menuIface    = "com.canonical.dbusmenu"
	watcherName  = "org.kde.StatusNotifierWatcher"
	watcherPath  = dbus.ObjectPath("/StatusNotifierWatcher")
)

// Menu item ids (dbusmenu requires stable int32 ids; 0 is the root)
const (
	idTitle    = 1
	idSep1     = 2
	idSep2     = 3
	idBattery  = 4
	idTotal    = 5
	idThresh   = 6
	idSep3     = 7
	idQuit     = 8
	idPortBase = 100 // port rows: 100, 101, ...
)

type LinuxTray struct {
	source PowerSource

	mu       sync.Mutex
	state    DisplayState
	revision uint32

	conn     *dbus.Conn
	props    *prop.Properties
	quitOnce sync.Once
	quitCh   chan struct{}
}

var trayInstance *LinuxTray

func NewTray() TrayUI {
	t := &LinuxTray{quitCh: make(chan struct{})}
	trayInstance = t
	return t
}

func (t *LinuxTray) Init(source PowerSource) {
	t.source = source

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot connect to session bus: %v\n", err)
		os.Exit(1)
	}
	t.conn = conn

	t.exportSNI()
	t.exportMenu()

	if err := t.register(); err != nil {
		fmt.Fprintf(os.Stderr, "cannot register with StatusNotifierWatcher: %v\n", err)
		os.Exit(1)
	}
	go t.reRegisterOnWatcherRestart()
}

// sniName is the well-known bus name libappindicator-style hosts expect.
func sniName() string {
	return fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
}

type sniPixmap struct {
	W, H int32
	Data []byte
}

type sniTooltip struct {
	IconName    string
	Pixmaps     []sniPixmap
	Title       string
	Description string
}

func (t *LinuxTray) exportSNI() {
	// Methods (no-ops: ItemIsMenu means the menu is the only interaction)
	_ = t.conn.Export(sniMethods{}, sniPath, sniInterface)

	spec := map[string]map[string]*prop.Prop{
		sniInterface: {
			"Category":          constProp("Hardware"),
			"Id":                constProp("power-monitor"),
			"Title":             constProp("Power Monitor"),
			"Status":            constProp("Active"),
			"WindowId":          constProp(uint32(0)),
			"IconName":          constProp("thunderbolt-symbolic"),
			"IconThemePath":     constProp(""),
			"IconPixmap":        constProp([]sniPixmap{}),
			"OverlayIconName":   constProp(""),
			"AttentionIconName": constProp(""),
			"ToolTip":           constProp(sniTooltip{Title: "Power Monitor"}),
			"ItemIsMenu":        constProp(true),
			"Menu":              constProp(menuPath),
			// GNOME's AppIndicator extension renders this next to the icon;
			// it picks up changes via the standard PropertiesChanged signal
			// (the prop package emits it for us).
			"XAyatanaLabel":         {Value: "", Writable: false, Emit: prop.EmitTrue},
			"XAyatanaLabelGuide":    {Value: "", Writable: false, Emit: prop.EmitTrue},
			"XAyatanaOrderingIndex": constProp(uint32(0)),
		},
	}
	props, err := prop.Export(t.conn, sniPath, spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot export SNI properties: %v\n", err)
		os.Exit(1)
	}
	t.props = props
}

func constProp(v interface{}) *prop.Prop {
	return &prop.Prop{Value: v, Writable: false, Emit: prop.EmitFalse}
}

type sniMethods struct{}

func (sniMethods) Activate(x, y int32) *dbus.Error          { return nil }
func (sniMethods) SecondaryActivate(x, y int32) *dbus.Error { return nil }
func (sniMethods) ContextMenu(x, y int32) *dbus.Error       { return nil }
func (sniMethods) Scroll(delta int32, dir string) *dbus.Error {
	return nil
}

func (t *LinuxTray) exportMenu() {
	_ = t.conn.Export((*menuObject)(t), menuPath, menuIface)
	spec := map[string]map[string]*prop.Prop{
		menuIface: {
			"Version":       constProp(uint32(3)),
			"Status":        constProp("normal"),
			"TextDirection": constProp("ltr"),
			"IconThemePath": constProp([]string{}),
		},
	}
	if _, err := prop.Export(t.conn, menuPath, spec); err != nil {
		fmt.Fprintf(os.Stderr, "cannot export menu properties: %v\n", err)
		os.Exit(1)
	}
}

func (t *LinuxTray) register() error {
	if _, err := t.conn.RequestName(sniName(), dbus.NameFlagDoNotQueue); err != nil {
		return err
	}
	watcher := t.conn.Object(watcherName, watcherPath)
	return watcher.Call(watcherName+".RegisterStatusNotifierItem", 0, sniName()).Err
}

// reRegisterOnWatcherRestart keeps the icon alive across host restarts
// (GNOME extension reloads, lock/unlock, panel crashes).
func (t *LinuxTray) reRegisterOnWatcherRestart() {
	err := t.conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, watcherName),
	)
	if err != nil {
		return
	}
	ch := make(chan *dbus.Signal, 8)
	t.conn.Signal(ch)
	for {
		select {
		case sig, ok := <-ch:
			if !ok {
				return
			}
			if len(sig.Body) == 3 {
				if newOwner, _ := sig.Body[2].(string); newOwner != "" {
					_ = t.register()
				}
			}
		case <-t.quitCh:
			return
		}
	}
}

// --- dbusmenu ---

type menuNode struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

func item(id int32, label string, enabled bool) menuNode {
	return menuNode{
		ID: id,
		Props: map[string]dbus.Variant{
			"label":   dbus.MakeVariant(label),
			"enabled": dbus.MakeVariant(enabled),
			"visible": dbus.MakeVariant(true),
		},
	}
}

func separator(id int32) menuNode {
	return menuNode{
		ID: id,
		Props: map[string]dbus.Variant{
			"type":    dbus.MakeVariant("separator"),
			"visible": dbus.MakeVariant(true),
		},
	}
}

// layout builds the whole menu from the current display state.
// Callers must hold t.mu.
func (t *LinuxTray) layout() menuNode {
	nodes := []menuNode{
		item(idTitle, "Power Monitor", false),
		separator(idSep1),
	}
	for i, l := range t.state.PortLabels {
		nodes = append(nodes, item(int32(idPortBase+i), l, false))
	}
	nodes = append(nodes, separator(idSep2))

	bat, total := t.state.BatLabel, t.state.TotalLabel
	if bat == "" {
		bat = "Battery: --"
	}
	if total == "" {
		total = "Power input: --"
	}
	nodes = append(nodes, item(idBattery, bat, false))
	nodes = append(nodes, item(idTotal, total, false))
	if t.state.ThreshLabel != "" {
		nodes = append(nodes, item(idThresh, t.state.ThreshLabel, false))
	}
	nodes = append(nodes,
		separator(idSep3),
		item(idQuit, "Quit", true),
	)

	children := make([]dbus.Variant, len(nodes))
	for i, n := range nodes {
		children[i] = dbus.MakeVariant(n)
	}
	return menuNode{
		ID: 0,
		Props: map[string]dbus.Variant{
			"children-display": dbus.MakeVariant("submenu"),
		},
		Children: children,
	}
}

// menuObject exposes the com.canonical.dbusmenu methods.
type menuObject LinuxTray

func (m *menuObject) tray() *LinuxTray { return (*LinuxTray)(m) }

func (m *menuObject) GetLayout(parentID int32, depth int32, names []string) (uint32, menuNode, *dbus.Error) {
	t := m.tray()
	t.mu.Lock()
	defer t.mu.Unlock()
	root := t.layout()
	if parentID == 0 {
		return t.revision, root, nil
	}
	for _, c := range root.Children {
		if n, ok := c.Value().(menuNode); ok && n.ID == parentID {
			return t.revision, n, nil
		}
	}
	return t.revision, menuNode{ID: parentID, Props: map[string]dbus.Variant{}}, nil
}

type menuItemProps struct {
	ID    int32
	Props map[string]dbus.Variant
}

func (m *menuObject) GetGroupProperties(ids []int32, names []string) ([]menuItemProps, *dbus.Error) {
	t := m.tray()
	t.mu.Lock()
	defer t.mu.Unlock()
	want := make(map[int32]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var out []menuItemProps
	for _, c := range t.layout().Children {
		if n, ok := c.Value().(menuNode); ok && (len(ids) == 0 || want[n.ID]) {
			out = append(out, menuItemProps{ID: n.ID, Props: n.Props})
		}
	}
	return out, nil
}

func (m *menuObject) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	t := m.tray()
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, c := range t.layout().Children {
		if n, ok := c.Value().(menuNode); ok && n.ID == id {
			if v, ok := n.Props[name]; ok {
				return v, nil
			}
		}
	}
	return dbus.MakeVariant(""), nil
}

func (m *menuObject) Event(id int32, eventID string, data dbus.Variant, timestamp uint32) *dbus.Error {
	if eventID == "clicked" && id == idQuit {
		m.tray().Quit()
	}
	return nil
}

func (m *menuObject) EventGroup(events []struct {
	ID        int32
	EventID   string
	Data      dbus.Variant
	Timestamp uint32
}) ([]int32, *dbus.Error) {
	for _, e := range events {
		_ = m.Event(e.ID, e.EventID, e.Data, e.Timestamp)
	}
	return nil, nil
}

func (m *menuObject) AboutToShow(id int32) (bool, *dbus.Error) {
	return false, nil
}

func (m *menuObject) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	return nil, nil, nil
}

// --- update loop ---

func (t *LinuxTray) update() {
	ports := t.source.USBCPorts()
	bat := t.source.Battery()
	ac := t.source.ACOnline()
	state := ComputeDisplay(ports, bat, ac)

	t.mu.Lock()
	changed := !sameState(t.state, state)
	labelChanged := t.state.BarLabel != state.BarLabel
	if changed {
		t.state = state
		t.revision++
	}
	rev := t.revision
	t.mu.Unlock()

	if !changed {
		return
	}

	// The host re-fetches the layout on this signal
	_ = t.conn.Emit(menuPath, menuIface+".LayoutUpdated", rev, int32(0))

	if labelChanged {
		// Padding keeps the text off the neighboring status icons
		label := "  " + state.BarLabel + "  "
		// SetMust (not Set: that's the bus-facing setter, which rejects
		// non-writable properties) emits the standard PropertiesChanged
		// signal GNOME's extension listens for (the GNOME 46 fix);
		// XAyatanaNewLabel covers hosts using the Ayatana signal instead.
		t.props.SetMust(sniInterface, "XAyatanaLabel", label)
		_ = t.conn.Emit(sniPath, sniInterface+".XAyatanaNewLabel", label, "")
	}
}

func sameState(a, b DisplayState) bool {
	if a.BarLabel != b.BarLabel || a.BatLabel != b.BatLabel ||
		a.TotalLabel != b.TotalLabel || a.ThreshLabel != b.ThreshLabel ||
		len(a.PortLabels) != len(b.PortLabels) {
		return false
	}
	for i := range a.PortLabels {
		if a.PortLabels[i] != b.PortLabels[i] {
			return false
		}
	}
	return true
}

func (t *LinuxTray) Run() {
	t.update()
	ticker := time.NewTicker(refreshPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t.update()
		case <-t.quitCh:
			t.conn.Close()
			return
		}
	}
}

func (t *LinuxTray) Quit() {
	t.quitOnce.Do(func() { close(t.quitCh) })
}

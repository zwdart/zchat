package app

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/i18n"
	"xin.dart.zchat/internal/lan"
	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/proto"
	"xin.dart.zchat/internal/share"
	"xin.dart.zchat/internal/store"
	"xin.dart.zchat/internal/transfer"
)

type pageKind int

const (
	pageChat pageKind = iota
	pageDiscover
	pageShare
)

// typingState says when a peer was last seen typing in a conversation.
type typingState struct {
	At time.Time
}

// transferState is a file download's progress, shown under its message.
type transferState struct {
	Done      int64
	Total     int64
	Err       string
	Working   bool
	Transport string // "TCP" or "HTTP"
}

// sendState is the sender's view of a file transfer, keyed by message ID.
type sendState struct {
	Sent      int64
	Total     int64
	Speed     float64
	Err       string
	Working   bool
	Done      bool
	Transport string // "TCP" or "HTTP"
	lastAt    time.Time
	lastSent  int64
}

// globalApp is the instance built by Run. Pin windows live outside the main
// window's lifetime and use it to show toasts.
var globalApp *app

// app is the state both windows show. It is read and changed on the main
// thread: by views, and by network goroutines through post.
type app struct {
	win         *mygo.Window
	tray        *mygo.Tray
	settingsWin *mygo.Window
	quitting    bool

	settings settings
	store    *store.Store
	dbStats  store.Stats
	profile  model.Profile
	peers    map[string]*model.Peer
	convs    map[string]*model.Conversation
	msgs     map[string][]model.Message
	outbox   map[string][]proto.Frame

	page          pageKind
	active        string
	query         string
	msgQuery      string
	input         string
	searchBox     ui.Handle
	msgSearch     ui.Handle
	focusSearch   bool
	focusMsgQuery bool

	msgList        ui.ListState
	rows           []chatRow
	rowsConv       string
	rowsLen        int
	rowsQuery      string
	msgSearchShown bool

	confirmOpen   bool
	confirmTitle  string
	confirmText   string
	confirmAction func()

	aliasOpen bool
	aliasPeer string
	aliasName string

	userInfoOpen bool
	userInfoID   string
	aboutOpen    bool

	profileName  string
	profileSig   string
	portEdit     float64
	transferEdit float64
	shareEdit    float64

	// settingsSection is the section the settings page should scroll to on
	// its next layout; settingsNav is the highlighted item of its menu.
	settingsSection string
	settingsNav     string
	// updateSupported says whether this build and install can replace
	// itself; otherwise the UI offers the release page instead.
	updateSupported bool

	node          *lan.Node
	share         *share.Server
	transfer      *transfer.Server
	shareQR       *ui.Bitmap
	qrForURL      string
	shareExplicit bool

	pal          palette
	avatars      map[string]*ui.Bitmap
	images       map[string]*ui.Bitmap
	imageFail    map[string]bool
	imageLoading map[string]bool

	typing       map[string]typingState
	transfers    map[string]*transferState
	sends        map[string]*sendState
	sendTokenMsg map[string]string

	toast      string
	lastTyping time.Time
	notifs     map[string]*mygo.Notification

	newGroupOpen   bool
	groupName      string
	groupPicked    map[string]bool
	groupInfoOpen  bool
	groupInfoID    string
	groupAddOpen   bool
	groupAddID     string
	groupAddPicked map[string]bool
	groupRename    string
	manualOpen     bool
	manualAddr     string

	stateDue    bool
	saveDue     map[string]bool
	lastRead    map[string]time.Time
	version     string
	startedOnce bool

	shot                *shotSession
	shotPins            []*pinWindow
	pins                []*pinWindow
	shotIncludeShortcut string
	shotExcludeShortcut string
	shotIncludeEdit     string
	shotExcludeEdit     string
	shotChooserOpen     bool
	shotChooserAnchored bool
	shotPending         bool
	shotReveal          func()
	shotSettingsFocus   bool
}

// post runs fn on the main thread and redraws the window. Without a
// window, as in tests, it drops fn.
func (a *app) post(fn func()) {
	if a.win == nil {
		return
	}
	a.win.Update(func() {
		if a.quitting {
			return
		}
		fn()
		a.redraw()
	})
}

func (a *app) redraw() {
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}

func (a *app) showMain() {
	if a.win == nil {
		return
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	a.win.Focus()
	mygo.App.Focus()
	// A window hidden to the tray may come back with an empty surface on
	// some platforms. Redraw the current state so the conversation shows
	// without waiting for the next message.
	a.redraw()
}

// hasArg reports whether args contains flag, with or without a leading
// value separator.
func hasArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// Run runs the app until it quits.
func Run() error {
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	setupDataDir()
	a := &app{
		peers:          map[string]*model.Peer{},
		convs:          map[string]*model.Conversation{},
		msgs:           map[string][]model.Message{},
		outbox:         map[string][]proto.Frame{},
		avatars:        map[string]*ui.Bitmap{},
		images:         map[string]*ui.Bitmap{},
		imageFail:      map[string]bool{},
		imageLoading:   map[string]bool{},
		typing:         map[string]typingState{},
		transfers:      map[string]*transferState{},
		sends:          map[string]*sendState{},
		sendTokenMsg:   map[string]string{},
		notifs:         map[string]*mygo.Notification{},
		groupPicked:    map[string]bool{},
		groupAddPicked: map[string]bool{},
		saveDue:        map[string]bool{},
		lastRead:       map[string]time.Time{},
	}
	globalApp = a
	shotApp = a
	a.useUpdater()
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if hasArg(args, "--capture") {
			a.startScreenshot(false)
			return
		}
		a.showMain()
	})
	mygo.App.OnNotificationClick(func(id string) {
		if !strings.HasPrefix(id, "chat:") {
			a.showMain()
			return
		}
		convID := strings.TrimPrefix(id, "chat:")
		a.post(func() {
			a.showMain()
			a.openConv(convID)
		})
	})
	mygo.App.WhenReady(func() {
		a.settings = loadSettings()
		i18n.Set(a.settings.Language)
		setLocale()
		a.settings.applyAppearance()
		a.version = mygo.App.Version()
		a.updateSupported = mygo.Updater.Enabled()
		a.openStore()
		a.loadData()
		a.rebuildSendTokens()
		a.share = share.New(filepath.Join(a.store.Dir(), "transfers.json"))
		a.share.SetProgress(a.onShareProgress)
		lan.SetVersion(a.version)
		a.startTransfer()

		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:                "zchat",
			Width:                1160,
			Height:               780,
			MinWidth:             900,
			MinHeight:            580,
			StateKey:             "main",
			TitleBarStyle:        mygo.TitleBarHidden,
			TitleBarHeight:       38,
			TrafficLightPosition: &mygo.Point{X: 18, Y: 18},
			Vibrancy:             mygo.VibrancySidebar,
			BackgroundColor:      "light-dark(#ffffff, #141416)",
			Content:              ui.View(a.view),
		})
		a.setMenu()
		a.startNetwork()
		a.applyTray()
		a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			a.post(func() { a.sendFiles(e.Paths) })
		})
		a.win.OnFocus(func() {
			a.post(func() { a.markActiveRead() })
		})
		a.win.OnShow(func() { a.redraw() })
		a.win.OnRestore(func() { a.redraw() })
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.quitting {
				return
			}
			e.PreventDefault()
			if a.settings.CloseToTray && (a.tray != nil || runtime.GOOS == "darwin") {
				a.win.Hide()
				return
			}
			mygo.App.Quit()
		})
		if a.settings.ShareOnStart && (a.settings.ShareDir != "" || a.settings.UploadDir != "") {
			a.startShare(false)
		}
		a.updateBadge()
		a.applyScreenshotShortcut()
		a.saveStateNow()
		if hasArg(os.Args[1:], "--capture") {
			a.startScreenshot(false)
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		for _, p := range append([]*pinWindow(nil), a.pins...) {
			if p.win != nil && !p.win.IsDestroyed() {
				p.win.Close()
			}
		}
		a.pins = nil
		a.unregisterScreenshotShortcuts()
		a.settings.Status = a.profile.Status
		_ = a.settings.save()
		a.saveAll()
		if a.store != nil {
			_ = a.store.Close()
		}
		if a.node != nil {
			a.node.Stop()
		}
		if a.share != nil {
			a.share.Stop()
		}
		a.stopTransfer()
		for _, n := range a.notifs {
			n.Close()
		}
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.tray != nil {
			a.tray.Destroy()
			a.tray = nil
		}
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(bool) { a.showMain() })
	return mygo.App.Run()
}

func (a *app) openStore() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return
	}
	a.store, err = store.Open(dir)
	if err != nil {
		mygo.Dialog.Error("zchat", tr("无法打开数据目录：")+err.Error())
		return
	}
	a.refreshDBStats()
}

// refreshDBStats keeps the size and counts the settings page shows current
// without querying the database on every frame.
func (a *app) refreshDBStats() {
	if a.store == nil {
		a.dbStats = store.Stats{}
		return
	}
	a.dbStats = a.store.Stats()
}

func (a *app) loadData() {
	if a.store == nil {
		return
	}
	st, err := a.store.Load()
	if err != nil {
		mygo.Dialog.Error("zchat", tr("无法读取数据：")+err.Error())
	}
	a.profile = st.Profile
	if a.profile.ID == "" {
		name, _ := os.Hostname()
		if strings.TrimSpace(name) == "" || name == "localhost" {
			name = tr("我的 zchat")
		}
		a.profile = model.Profile{ID: model.NewID("u"), Name: name, Status: a.settings.Status}
	}
	if a.profile.Status == "" {
		a.profile.Status = a.settings.Status
	}
	if a.profile.Name == "" {
		a.profile.Name = tr("我的 zchat")
	}
	a.settings.Status = a.profile.Status
	for id, p := range st.Peers {
		if p != nil {
			p.Connected = false
			a.peers[id] = p
		}
	}
	if st.Outbox != nil {
		a.outbox = st.Outbox
	}
	for _, c := range st.Conversations {
		if c == nil || c.ID == "" {
			continue
		}
		a.convs[c.ID] = c
		msgs, err := a.store.LoadMessages(c.ID)
		if err != nil {
			continue
		}
		for i := range msgs {
			msgs[i].Mine = msgs[i].SenderID == a.profile.ID
		}
		a.msgs[c.ID] = msgs
	}
	if a.active == "" {
		if list := a.orderedConvs(); len(list) > 0 {
			a.active = list[0].ID
		}
	}
	a.startedOnce = true
}

// saveAll writes everything without the debounce, for quitting.
func (a *app) saveAll() {
	if a.store == nil {
		return
	}
	if err := a.store.Save(a.snapshot()); err != nil {
		return
	}
	for id, msgs := range a.msgs {
		_ = a.store.SaveMessages(id, msgs)
	}
}

func (a *app) snapshot() store.State {
	convs := make([]*model.Conversation, 0, len(a.convs))
	for _, c := range a.convs {
		convs = append(convs, c)
	}
	sort.Slice(convs, func(i, j int) bool { return convs[i].ID < convs[j].ID })
	return store.State{
		Profile:       a.profile,
		Conversations: convs,
		Peers:         a.peers,
		Outbox:        a.outbox,
	}
}

func (a *app) saveStateNow() {
	if a.store == nil {
		return
	}
	a.stateDue = false
	_ = a.store.Save(a.snapshot())
}

// scheduleSave keeps the state a moment after the last change, so typing
// many messages does not write the file for each.
func (a *app) scheduleSave() {
	if a.stateDue || a.store == nil {
		return
	}
	a.stateDue = true
	time.AfterFunc(300*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			if a.quitting {
				return
			}
			a.saveStateNow()
		})
	})
}

func (a *app) saveMessages(convID string) {
	if a.store == nil || a.saveDue[convID] {
		return
	}
	a.saveDue[convID] = true
	time.AfterFunc(300*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			a.saveDue[convID] = false
			if a.quitting {
				return
			}
			if a.store != nil {
				_ = a.store.SaveMessages(convID, a.msgs[convID])
			}
		})
	})
}

func (a *app) orderedConvs() []*model.Conversation {
	out := make([]*model.Conversation, 0, len(a.convs))
	for _, c := range a.convs {
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (a *app) currentConv() *model.Conversation {
	if a.active == "" {
		return nil
	}
	return a.convs[a.active]
}

func (a *app) convTitle(c *model.Conversation) string {
	if c == nil {
		return ""
	}
	if c.Kind == model.KindGroup {
		if c.Title != "" {
			return c.Title
		}
		return tr("群聊")
	}
	if p := a.peers[c.PeerID]; p != nil {
		return tr(p.Title())
	}
	return tr("未知设备")
}

func (a *app) convPeer(c *model.Conversation) *model.Peer {
	if c == nil || c.Kind != model.KindDirect {
		return nil
	}
	return a.peers[c.PeerID]
}

func (a *app) ensureDirect(p *model.Peer) *model.Conversation {
	if p == nil {
		return nil
	}
	if c := a.convs[p.ID]; c != nil {
		if c.Kind != model.KindDirect {
			c.Kind = model.KindDirect
			c.PeerID = p.ID
		}
		return c
	}
	now := time.Now()
	c := &model.Conversation{ID: p.ID, Kind: model.KindDirect, PeerID: p.ID, CreatedAt: now, UpdatedAt: now}
	a.convs[c.ID] = c
	a.msgs[c.ID] = nil
	a.scheduleSave()
	return c
}

func (a *app) setPage(p pageKind) {
	a.page = p
	a.redraw()
}

func (a *app) openConv(id string) {
	c := a.convs[id]
	if c == nil {
		return
	}
	if a.active != "" && a.active != id {
		a.stopTyping(a.active)
		a.msgList = ui.ListState{}
	}
	a.page = pageChat
	a.active = id
	a.msgQuery = ""
	a.msgSearchShown = false
	a.resetRows()
	a.input = c.Draft
	c.Unread = 0
	a.updateBadge()
	a.markActiveRead()
	a.queueActiveImages()
	a.scheduleSave()
	a.redraw()
}

func (a *app) activeVisible() bool {
	if a.win == nil || a.win.IsDestroyed() {
		return false
	}
	return a.win.IsVisible() && a.win.IsFocused()
}

func (a *app) markActiveRead() {
	c := a.currentConv()
	if c == nil {
		return
	}
	if c.Unread != 0 {
		c.Unread = 0
		a.updateBadge()
		a.scheduleSave()
	}
	if c.Kind != model.KindDirect {
		return
	}
	last := a.lastRead[c.ID]
	if time.Since(last) < time.Second {
		return
	}
	a.lastRead[c.ID] = time.Now()
	msgs := a.msgs[c.ID]
	var at time.Time
	for i := len(msgs) - 1; i >= 0; i-- {
		if !msgs[i].Mine {
			at = msgs[i].SentAt
			break
		}
	}
	if at.IsZero() {
		return
	}
	f := proto.NewFrame(a.profile.ID, proto.TypeRead, proto.Read{ConvID: c.ID, Time: at.UnixMilli()})
	a.sendFrame(c.PeerID, f)
}

func (a *app) updateBadge() {
	total := 0
	for _, c := range a.convs {
		if !c.Muted {
			total += c.Unread
		}
	}
	mygo.App.SetBadgeCount(total)
	if a.tray != nil {
		// The tooltip is the only badge many Linux panels show.
		tip := "zchat"
		if total > 0 {
			tip = "zchat · " + itoa(total) + tr(" 条未读")
		}
		a.tray.SetToolTip(tip)
	}
}

// setStatus changes the presence shown to peers and keeps it for the next
// launch.
func (a *app) setStatus(status model.Presence) {
	if status == "" {
		status = model.Online
	}
	a.profile.Status = status
	a.settings.Status = status
	_ = a.settings.save()
	if a.node != nil {
		a.node.SetStatus(status)
	}
	a.applyTray()
	a.updateBadge()
	a.scheduleSave()
	a.redraw()
}

// confirm asks a question in a modal and runs action when the user agrees.
func (a *app) confirm(title, text string, action func()) {
	a.confirmOpen = true
	a.confirmTitle = title
	a.confirmText = text
	a.confirmAction = action
	a.redraw()
}

func (a *app) msgSearchOpen() bool { return a.msgSearchShown }

// resetRows forgets the laid-out rows. Clearing only a.rows would leave the
// cache keys matching, so rebuildRows would return early and the list would
// stay empty until a new message changed its length.
func (a *app) resetRows() {
	a.rows = nil
	a.rowsConv = ""
	a.rowsLen = -1
	a.rowsQuery = ""
}

func (a *app) clearMsgSearch() {
	a.msgQuery = ""
	a.msgSearchShown = false
	a.resetRows()
	if conv := a.currentConv(); conv != nil {
		a.rebuildRows(conv)
	}
	a.redraw()
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func (a *app) setToast(msg string) {
	a.toast = msg
	a.redraw()
}

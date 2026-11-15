package app

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
	"github.com/floatpane/matcha/daemonrpc"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/palette"
	"github.com/floatpane/matcha/internal/testhome"
	"github.com/floatpane/matcha/tui"
	"github.com/floatpane/matcha/view"
)

// TestMain keeps the tests away from the developer's real matcha config and
// caches.
func TestMain(m *testing.M) {
	// Image protocol detection changes inside tmux.
	_ = os.Unsetenv("TMUX")
	_ = os.Unsetenv("TMUX_PANE")
	testhome.Main(m)
}

// recordingView is a minimal view that records the messages it receives.
type recordingView struct {
	msgs []tea.Msg
}

func (r *recordingView) Init() tea.Cmd { return nil }
func (r *recordingView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	r.msgs = append(r.msgs, msg)
	return r, nil
}
func (r *recordingView) View() tea.View { return tea.NewView("recording") }

func (r *recordingView) received(match func(tea.Msg) bool) bool {
	for _, msg := range r.msgs {
		if match(msg) {
			return true
		}
	}
	return false
}

func newTestModel(t *testing.T) *Model {
	t.Helper()
	m := NewModel(nil, nil, nil, false, nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

func setMouse(t *testing.T, enabled *bool) {
	t.Helper()
	previous := config.MouseEnabled
	config.MouseEnabled = enabled
	t.Cleanup(func() { config.MouseEnabled = previous })
}

func TestViewAppliesMouseModeToEveryScreen(t *testing.T) {
	m := newTestModel(t)
	if _, ok := m.current.(*tui.Login); !ok {
		t.Fatalf("expected the login screen without a config, got %T", m.current)
	}

	on := true
	setMouse(t, &on)
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("mouse enabled: MouseMode = %v on the login screen, want cell motion", got)
	}
	off := false
	setMouse(t, &off)
	if got := m.View().MouseMode; got != tea.MouseModeNone {
		t.Fatalf("mouse disabled: MouseMode = %v, want none", got)
	}
}

func TestPaletteSwallowsClicks(t *testing.T) {
	m := newTestModel(t)
	rec := &recordingView{}
	m.current = rec
	m.palette.Open(palette.BuildCommands(m.current, nil, nil), m.width, m.contentHeight())
	if !m.palette.IsOpen() {
		t.Fatal("palette did not open")
	}
	m.Update(tea.MouseClickMsg{X: 50, Y: 15, Button: tea.MouseLeft})
	if rec.received(func(msg tea.Msg) bool { _, ok := msg.(tea.MouseClickMsg); return ok }) {
		t.Fatal("a click on the open palette reached the view underneath")
	}
}

func TestStaleNotificationTimerKeepsNewerToast(t *testing.T) {
	m := newTestModel(t)
	m.showErrorCmd("first")
	firstGen := m.errorNotifGen
	m.showInfoCmd("second")
	m.Update(clearErrorNotifMsg{gen: firstGen})
	if !m.showErrorNotif {
		t.Fatal("the first toast's timer hid the second toast")
	}
	m.Update(clearErrorNotifMsg{gen: m.errorNotifGen})
	if m.showErrorNotif {
		t.Fatal("the toast's own timer did not hide it")
	}
}

func TestNotificationsFitNarrowScreens(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 20})
	m.showErrorCmd("Something went wrong while doing a thing")
	for _, r := range m.notificationRects() {
		if r[1] < 0 || r[1]+r[3] > 30+2 {
			t.Fatalf("toast rectangle %v does not fit a 30-column screen", r)
		}
	}
}

func TestReturningViewReceivesCurrentSize(t *testing.T) {
	m := newTestModel(t)
	back := &recordingView{}
	m.current = &recordingView{}
	m.previousModel = back
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.Update(tui.RestoreViewMsg{})
	if m.current != tea.Model(back) {
		t.Fatalf("RestoreViewMsg did not restore the previous view, current is %T", m.current)
	}
	if !back.received(func(msg tea.Msg) bool {
		size, ok := msg.(tea.WindowSizeMsg)
		return ok && size.Width == 120 && size.Height == 40
	}) {
		t.Fatal("restored view was not told the current window size")
	}
}

func TestViewChangedHandlesValueViews(t *testing.T) {
	a, b := &recordingView{}, &recordingView{}
	if !viewChanged(a, b) || viewChanged(a, a) {
		t.Fatal("pointer views must be compared by identity")
	}
	// Comparing non-comparable value views must not panic.
	if viewChanged(tui.NewStatus("one"), tui.NewStatus("two")) {
		t.Fatal("value views of the same type are treated as unchanged")
	}
	if !viewChanged(tui.NewStatus("one"), a) {
		t.Fatal("different view types must count as a change")
	}
}

// fakeService is a mail service whose calls are recorded.
type fakeService struct {
	daemonclient.Service
	daemon bool
	events chan *daemonrpc.Event

	mu           sync.Mutex
	gate         chan struct{} // when set, Subscribe/Unsubscribe wait for it
	subscribeErr error
	queueErr     error
	subs         []string
	unsubs       []string
	closed       int
	queued       int
	listens      int
}

func newFakeService(daemon bool) *fakeService {
	return &fakeService{daemon: daemon, events: make(chan *daemonrpc.Event, 4)}
}

func (f *fakeService) IsDaemon() bool { return f.daemon }
func (f *fakeService) Events() <-chan *daemonrpc.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listens++
	return f.events
}

func (f *fakeService) waitGate() {
	f.mu.Lock()
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
}

func (f *fakeService) Subscribe(account, folder string) error {
	f.waitGate()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subs = append(f.subs, account+"/"+folder)
	return f.subscribeErr
}
func (f *fakeService) Unsubscribe(account, folder string) error {
	f.waitGate()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unsubs = append(f.unsubs, account+"/"+folder)
	return nil
}
func (f *fakeService) QueueEmail(string, []string, []string, []string, string, string, string, map[string][]byte, map[string][]byte, string, []string, bool, bool, bool, bool, int, []byte) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.queueErr != nil {
		return "", f.queueErr
	}
	f.queued++
	return "job", nil
}
func (f *fakeService) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
	return nil
}
func (f *fakeService) ReloadConfig() error { return nil }

func (f *fakeService) snapshot() (subs, unsubs []string, closed int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.subs...), append([]string(nil), f.unsubs...), f.closed
}

func (f *fakeService) counts() (queued, listens int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queued, f.listens
}

// useSlowServices makes the app's successive service starts return fakes
// in order, each connecting only when its release function is called.
func useSlowServices(t *testing.T, fakes ...*fakeService) (release []func()) {
	t.Helper()
	gates := make([]chan struct{}, len(fakes))
	release = make([]func(), len(fakes))
	for i := range fakes {
		gates[i] = make(chan struct{})
		var once sync.Once
		gate := gates[i]
		release[i] = func() { once.Do(func() { close(gate) }) }
	}
	var mu sync.Mutex
	next := 0
	previous := startService
	startService = func(*config.Config) *daemonclient.StartingService {
		mu.Lock()
		i := next
		next++
		mu.Unlock()
		if i >= len(fakes) {
			t.Errorf("unexpected service start #%d", i+1)
			i = len(fakes) - 1
		}
		return daemonclient.StartServiceFunc(func() daemonclient.Service {
			<-gates[i]
			return fakes[i]
		})
	}
	t.Cleanup(func() {
		for _, r := range release {
			r()
		}
		startService = previous
	})
	return release
}

func useSlowService(t *testing.T, fake *fakeService) (release func()) {
	t.Helper()
	return useSlowServices(t, fake)[0]
}

func newConfiguredModel(t *testing.T) *Model {
	t.Helper()
	cfg := &config.Config{
		Accounts:          []config.Account{{ID: "acct", Email: "me@example.com", Protocol: "maildir", MaildirPath: t.TempDir()}},
		HasSeenSetupGuide: true,
		DisableDaemon:     false,
	}
	m := NewModel(cfg, nil, nil, false, nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return m
}

// runBatch executes cmd (and nested batches) and returns the messages,
// skipping commands that do not finish quickly (such as event listeners).
func runBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if cmds, ok := nestedCmds(msg); ok {
			var msgs []tea.Msg
			for _, c := range cmds {
				msgs = append(msgs, runBatch(c)...)
			}
			return msgs
		}
		if msg == nil {
			return nil
		}
		return []tea.Msg{msg}
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

// nestedCmds returns the commands of a tea.Batch or tea.Sequence message
// (the latter's type is unexported, hence the reflection).
func nestedCmds(msg tea.Msg) ([]tea.Cmd, bool) {
	if batch, ok := msg.(tea.BatchMsg); ok {
		return batch, true
	}
	v := reflect.ValueOf(msg)
	if !v.IsValid() || v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeOf(tea.Cmd(nil)) {
		return nil, false
	}
	cmds := make([]tea.Cmd, v.Len())
	for i := range cmds {
		cmds[i] = v.Index(i).Interface().(tea.Cmd)
	}
	return cmds, true
}

// startCmds runs every command of cmd concurrently, as Bubble Tea does, and
// delivers their messages on the returned channel.
func startCmds(cmd tea.Cmd) <-chan tea.Msg {
	out := make(chan tea.Msg, 64)
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		go func() {
			msg := c()
			if cmds, ok := nestedCmds(msg); ok {
				for _, inner := range cmds {
					run(inner)
				}
				return
			}
			if msg != nil {
				out <- msg
			}
		}()
	}
	run(cmd)
	return out
}

// awaitMsg returns the first message on msgs that match accepts.
func awaitMsg[T tea.Msg](t *testing.T, msgs <-chan tea.Msg, match func(T) bool) T {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg := <-msgs:
			if v, ok := msg.(T); ok && (match == nil || match(v)) {
				return v
			}
		case <-deadline:
			var zero T
			t.Fatalf("no %T arrived", zero)
			return zero
		}
	}
}

// updateQuickly runs m.Update(msg) and fails if it blocks.
func updateQuickly(t *testing.T, m *Model, msg tea.Msg) tea.Cmd {
	t.Helper()
	returned := make(chan tea.Cmd, 1)
	go func() {
		_, cmd := m.Update(msg)
		returned <- cmd
	}()
	select {
	case cmd := <-returned:
		return cmd
	case <-time.After(time.Second):
		t.Fatalf("Update(%T) waited for the mail service", msg)
		return nil
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestOpeningInboxDoesNotWaitForTheService(t *testing.T) {
	fake := newFakeService(true)
	release := useSlowService(t, fake)
	m := newConfiguredModel(t)

	cmd := updateQuickly(t, m, tui.GoToInboxMsg{})
	if _, ok := m.current.(*tui.FolderInbox); !ok {
		t.Fatalf("current view is %T, want the folder inbox", m.current)
	}
	if m.service == nil || m.serviceReady() {
		t.Fatal("the service should exist and still be connecting")
	}

	// The inbox's own commands report when the service has connected.
	msgs := startCmds(cmd)
	release()
	ready := awaitMsg[serviceReadyMsg](t, msgs, nil)
	if ready.svc != m.service {
		t.Fatal("serviceReadyMsg is for another service")
	}
	_, cmd = m.Update(ready)
	startCmds(cmd)
	eventually(t, "INBOX is subscribed", func() bool {
		subs, _, _ := fake.snapshot()
		return len(subs) == 1 && subs[0] == "acct/INBOX"
	})
	if _, listens := fake.counts(); listens != 1 {
		t.Fatalf("%d event listeners started, want 1", listens)
	}

	// Switching folders moves the subscription without waiting on the
	// daemon's RPCs.
	gate := make(chan struct{})
	fake.mu.Lock()
	fake.gate = gate
	fake.mu.Unlock()
	cmd = updateQuickly(t, m, tui.SwitchFolderMsg{FolderName: "Archive", PreviousFolder: "INBOX"})
	startCmds(cmd)
	close(gate)
	eventually(t, "INBOX is unsubscribed", func() bool {
		_, unsubs, _ := fake.snapshot()
		return len(unsubs) == 1 && unsubs[0] == "acct/INBOX"
	})

	// A repeated ready message does not start a second listener.
	m.Update(serviceReadyMsg{svc: m.service})
	if _, listens := fake.counts(); listens != 1 {
		t.Fatalf("%d event listeners after a second ready message, want 1", listens)
	}
}

func TestServiceShutDownWhileStartingIsDiscarded(t *testing.T) {
	first, second := newFakeService(true), newFakeService(true)
	releases := useSlowServices(t, first, second)
	m := newConfiguredModel(t)
	m.Update(tui.GoToInboxMsg{})
	old := m.service

	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape}) // back to the menu while connecting
	if m.service != nil {
		t.Fatal("esc did not shut the service down")
	}
	releases[0]()
	<-old.(*daemonclient.StartingService).Ready()
	m.Update(serviceReadyMsg{svc: old})
	eventually(t, "the discarded service is closed", func() bool {
		_, _, closed := first.snapshot()
		return closed == 1
	})

	// Re-enter with a new service; the old one's messages must not disturb it.
	m.Update(tui.GoToInboxMsg{})
	releases[1]()
	<-m.service.(*daemonclient.StartingService).Ready()
	_, cmd := m.Update(serviceReadyMsg{svc: m.service})
	startCmds(cmd)
	m.Update(serviceReadyMsg{svc: old})
	m.Update(serviceEventMsg{svc: old, event: &daemonrpc.Event{Type: daemonrpc.EventNewMail}})
	m.Update(serviceEventMsg{svc: old, closed: true})
	if _, listens := second.counts(); listens != 1 || !m.eventsListening {
		t.Fatalf("new service has %d listeners (eventsListening=%v), want exactly 1", listens, m.eventsListening)
	}
	if subs, _, _ := first.snapshot(); len(subs) != 0 {
		t.Fatalf("a discarded service was subscribed: %v", subs)
	}
}

func sendFromComposer(t *testing.T, m *Model) tea.Cmd {
	t.Helper()
	m.current = tui.NewComposerWithAccounts(m.config.Accounts, "acct", "you@example.com", "Hi", "Body", true)
	return updateQuickly(t, m, tui.SendEmailMsg{To: "you@example.com", Subject: "Hi", Body: "Body", AccountID: "acct"})
}

func TestSendingDoesNotWaitForTheService(t *testing.T) {
	fake := newFakeService(true)
	release := useSlowService(t, fake)
	m := newConfiguredModel(t)
	msgs := startCmds(sendFromComposer(t, m))
	if m.service == nil {
		t.Fatal("sending should start the mail service")
	}
	release()
	done := awaitMsg[sendFinishedMsg](t, msgs, nil)
	if _, ok := done.result.(tui.EmailQueuedMsg); !ok {
		t.Fatalf("send finished with %#v, want the queued email", done.result)
	}
	if queued, _ := fake.counts(); queued != 1 {
		t.Fatalf("%d emails queued, want 1", queued)
	}
}

func TestQuitWaitsForEmailsBeingSent(t *testing.T) {
	fake := newFakeService(true)
	release := useSlowService(t, fake)
	m := newConfiguredModel(t)
	msgs := startCmds(sendFromComposer(t, m))

	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.quitting || m.service == nil {
		t.Fatal("ctrl+c quit while the email was still waiting for the service")
	}
	release()
	done := awaitMsg[sendFinishedMsg](t, msgs, nil)
	m.Update(done)
	if !m.quitting {
		t.Fatal("the program did not quit once the email was handed off")
	}
	if queued, _ := fake.counts(); queued != 1 {
		t.Fatalf("%d emails queued, want 1", queued)
	}
}

func TestSecondQuitKeepsUnsentEmailAsDraft(t *testing.T) {
	useSlowService(t, newFakeService(true)) // never connects
	m := newConfiguredModel(t)
	sendFromComposer(t, m)
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !m.quitting {
		t.Fatal("a second ctrl+c did not quit")
	}
	cache, err := config.LoadDraftsCache()
	if err != nil || len(cache.Drafts) != 1 || cache.Drafts[0].Subject != "Hi" {
		t.Fatalf("drafts after quitting with an unsent email: %+v, %v", cache, err)
	}
	t.Cleanup(func() { _ = config.DeleteDraft(cache.Drafts[0].ID) })
}

func TestSendFailureIsReportedWithoutTheInbox(t *testing.T) {
	fake := newFakeService(true)
	release := useSlowService(t, fake)
	m := newConfiguredModel(t)
	msgs := startCmds(sendFromComposer(t, m))
	release()
	ready := awaitMsg[serviceReadyMsg](t, msgs, nil)
	_, cmd := m.Update(ready)
	if _, listens := fake.counts(); listens != 1 {
		t.Fatalf("%d event listeners after sending from the menu, want 1", listens)
	}
	if subs, _, _ := fake.snapshot(); len(subs) != 0 {
		t.Fatalf("sending from the menu subscribed to %v", subs)
	}
	data, _ := json.Marshal(daemonrpc.SendFailedEvent{Subject: "Hi", Error: "550 rejected"})
	fake.events <- &daemonrpc.Event{Type: daemonrpc.EventSendFailed, Data: data}
	event := awaitMsg[serviceEventMsg](t, startCmds(cmd), nil)
	m.Update(event)
	if !m.showErrorNotif {
		t.Fatal("the failed send was not reported")
	}
}

func TestSendingAfterLeavingTheInboxDoesNotWatchIt(t *testing.T) {
	first, second := newFakeService(true), newFakeService(true)
	releases := useSlowServices(t, first, second)
	releases[0]()
	m := newConfiguredModel(t)
	m.Update(tui.GoToInboxMsg{})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	msgs := startCmds(sendFromComposer(t, m))
	releases[1]()
	ready := awaitMsg[serviceReadyMsg](t, msgs, func(r serviceReadyMsg) bool { return r.svc == m.service })
	_, cmd := m.Update(ready)
	runBatch(cmd)
	if subs, _, _ := second.snapshot(); len(subs) != 0 {
		t.Fatalf("a send from the menu re-subscribed the closed inbox: %v", subs)
	}
}

func TestFailedSubscribeIsStillUnsubscribed(t *testing.T) {
	fake := newFakeService(true)
	fake.subscribeErr = fmt.Errorf("daemon connection lost")
	subs := newFolderSubscriptions()
	subs.set(map[string]string{"acct": "INBOX"})
	subs.apply(fake)
	subs.set(map[string]string{"acct": "Archive"})
	subs.apply(fake)
	if _, unsubs, _ := fake.snapshot(); len(unsubs) != 1 || unsubs[0] != "acct/INBOX" {
		t.Fatalf("unsubscriptions = %v; the daemon client replays a failed subscribe, so it must be undone", unsubs)
	}
}

// recordingIdle records the direct IDLE watches the app starts.
type recordingIdle struct{ watched []string }

func (r *recordingIdle) Watch(account *config.Account, folder string) {
	r.watched = append(r.watched, account.ID+"/"+folder)
}
func (r *recordingIdle) Stop(string) {}
func (r *recordingIdle) StopAll()    {}

func TestDirectIdleRestartsWhenSettingsChangeOrInboxReopens(t *testing.T) {
	fake := newFakeService(false) // no daemon: IDLE runs in the app
	useSlowService(t, fake)()
	m := newConfiguredModel(t)
	idle := &recordingIdle{}
	m.idleWatcher = idle
	m.Update(tui.GoToInboxMsg{})
	<-m.service.(*daemonclient.StartingService).Ready()
	m.Update(serviceReadyMsg{svc: m.service})
	if len(idle.watched) != 1 || idle.watched[0] != "acct/INBOX" {
		t.Fatalf("IDLE watches after opening the inbox = %v, want [acct/INBOX]", idle.watched)
	}

	m.Update(serviceReadyMsg{svc: m.service}) // nothing changed: no reconnect
	if len(idle.watched) != 1 {
		t.Fatalf("IDLE reconnected without a reason: %v", idle.watched)
	}

	m.reloadService() // an account was edited: IDLE holds the old settings
	if len(idle.watched) != 2 {
		t.Fatalf("IDLE watches after a config reload = %v, want a restart", idle.watched)
	}

	m.Update(tui.GoToInboxMsg{}) // back to the inbox without esc
	m.Update(serviceReadyMsg{svc: m.service})
	if len(idle.watched) != 3 {
		t.Fatalf("IDLE watches after re-entering the inbox = %v, want a restart", idle.watched)
	}
}

func countPreviewBodies(rec *recordingView) int {
	n := 0
	for _, msg := range rec.msgs {
		if _, ok := msg.(tui.PreviewBodyFetchedMsg); ok {
			n++
		}
	}
	return n
}

func TestPreviewBodyReachesTheViewOnceAndOnlyForItsFolder(t *testing.T) {
	m := newTestModel(t)
	m.config = &config.Config{}
	m.folderInbox = tui.NewFolderInbox([]string{"INBOX", "Archive"}, nil)
	rec := &recordingView{}
	m.current = rec

	m.Update(tui.PreviewBodyFetchedMsg{UID: 1, AccountID: "a", Body: "hello", Folder: "INBOX"})
	if n := countPreviewBodies(rec); n != 1 {
		t.Fatalf("the preview body reached the view %d times, want once", n)
	}
	// A body fetched for a folder the user has since left must not be shown.
	m.Update(tui.PreviewBodyFetchedMsg{UID: 1, AccountID: "a", Body: "other", Folder: "Archive"})
	if n := countPreviewBodies(rec); n != 1 {
		t.Fatal("a preview body from another folder reached the view")
	}
}

func inboxEmailCount(fi *tui.FolderInbox) int {
	n := 0
	for fi.GetInbox().GetEmailAtIndex(n) != nil {
		n++
	}
	return n
}

func TestAppendedPageIsAddedOnce(t *testing.T) {
	m := newTestModel(t)
	accounts := []config.Account{{ID: "a", Email: "me@example.com"}}
	m.config = &config.Config{Accounts: accounts}
	fi := tui.NewFolderInbox([]string{"INBOX"}, accounts)
	emails := make([]fetcher.Email, 12)
	for i := range emails {
		emails[i] = fetcher.Email{UID: uint32(i + 1), AccountID: "a", Subject: fmt.Sprintf("mail %d", i+1), Date: time.Now().Add(-time.Duration(i) * time.Hour)}
	}
	fi.SetEmails(emails[:10], accounts)
	m.folderInbox = fi
	m.current = fi
	m.Update(tui.FolderEmailsAppendedMsg{FolderName: "INBOX", AccountID: "a", Emails: emails[10:]})
	if n := inboxEmailCount(fi); n != 12 {
		t.Fatalf("after appending 2 emails to 10 the inbox lists %d", n)
	}
}

func schedulesPaint(cmd tea.Cmd) bool {
	for _, msg := range runBatch(cmd) {
		if _, ok := msg.(tui.GraphicsPaintMsg); ok {
			return true
		}
	}
	return false
}

func TestQuitDoesNotPlaceImagesAgain(t *testing.T) {
	m := newTestModel(t)
	m.current = &imageView{images: []tui.ScreenImage{{ID: 7, Placement: 1, Protocol: view.ImageProtocolKitty, Row: 1, Col: 1, Rows: 2, Cols: 2}}}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); !schedulesPaint(cmd) {
		t.Fatal("the view's image was never scheduled for painting")
	}
	m.quitCmd() // removes every image
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"}); schedulesPaint(cmd) {
		t.Fatal("an image placement was scheduled after quitting")
	}
}

// imageView is a view that shows fixed out-of-band images.
type imageView struct {
	recordingView
	images []tui.ScreenImage
}

func (v *imageView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	v.recordingView.Update(msg)
	return v, nil
}

func (v *imageView) VisibleImages() []tui.ScreenImage { return v.images }

func hasQuit(cmd tea.Cmd) bool {
	for _, msg := range runBatch(cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}

func TestCtrlCWhileQuittingExitsAtOnce(t *testing.T) {
	m := newConfiguredModel(t)
	m.quitCmd() // a quit is under way, e.g. stuck flushing a pending delete
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !hasQuit(cmd) {
		t.Fatal("ctrl+c during a quit that is taking long did not exit")
	}
}

func TestFailedSendDuringQuitWaitCancelsTheQuit(t *testing.T) {
	fake := newFakeService(true)
	fake.queueErr = fmt.Errorf("550 mailbox unavailable")
	release := useSlowService(t, fake)
	m := newConfiguredModel(t)
	msgs := startCmds(sendFromComposer(t, m))
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	release()
	done := awaitMsg[sendFinishedMsg](t, msgs, nil)
	_, cmd := m.Update(done)
	if m.quitting || hasQuit(cmd) {
		t.Fatal("matcha quit over a failed send, so the user never saw the error")
	}
	if _, ok := m.current.(*tui.Composer); !ok || !m.showErrorNotif {
		t.Fatalf("after the failed send current is %T (error shown: %v); want the composer back with the error", m.current, m.showErrorNotif)
	}
}

func TestQuitAfterHandoffDropsTheDraft(t *testing.T) {
	fake := newFakeService(true)
	release := useSlowService(t, fake)
	m := newConfiguredModel(t)
	composer := tui.NewComposerWithAccounts(m.config.Accounts, "acct", "you@example.com", "Hi", "Body", true)
	draft := composer.ToDraft()
	if err := config.SaveDraft(draft); err != nil {
		t.Fatal(err)
	}
	composer.SetDraftID(draft.ID)
	m.current = composer
	msgs := startCmds(updateQuickly(t, m, tui.SendEmailMsg{To: "you@example.com", Subject: "Hi", Body: "Body", AccountID: "acct"}))
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	release()
	m.Update(awaitMsg[sendFinishedMsg](t, msgs, nil))
	if !m.quitting {
		t.Fatal("the program did not quit once the email was handed off")
	}
	cache, err := config.LoadDraftsCache()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range cache.Drafts {
		if d.ID == draft.ID {
			t.Fatal("the draft of an email that was sent survived the quit")
		}
	}
}

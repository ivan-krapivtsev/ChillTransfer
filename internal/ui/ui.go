package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/ivan-krapivtsev/chilltransfer/internal/devices"
	"github.com/ivan-krapivtsev/chilltransfer/internal/engine"
)

// defaultPollInterval is how often progress/state widgets are refreshed while a run is active.
const defaultPollInterval = 500 * time.Millisecond

// UI is the main window controller: device panel, drop zone + queue, mode
// settings and run controls (SPEC §6). Construct via New with an injected
// DeviceLister; Clock defaults to real time and drives copier timing.
type UI struct {
	App fyne.App
	// I18N holds the user-visible string tables for the active language;
	// New defaults it to DetectLanguage() (SPEC §7: RU + EN).
	I18N *I18N
	// Lister provides the device list; inject a fake in tests.
	Lister DeviceLister
	// Clock drives copier timing; inject a fake clock for deterministic tests.
	Clock engine.Clock
	// PollInterval is the progress polling period while a run is active
	// (default 500 ms). Zero disables the automatic loop so tests can drive
	// poll() manually from a single goroutine.
	PollInterval time.Duration

	Queue *engine.Queue

	Window fyne.Window // created by New; parent of native dialogs

	// Widgets, exported so tests can inspect and drive them.
	DeviceSelect     *widget.Select
	SensorLabel      *widget.Label
	RefreshButton    *widget.Button
	AddFilesButton   *widget.Button
	AddFolderButton  *widget.Button
	QueueList        *widget.List
	ProgressBar      *widget.ProgressBar
	CurrentFileLabel *widget.Label
	SpeedLabel       *widget.Label
	ETALabel         *widget.Label
	ModeRadio        *widget.RadioGroup
	TPauseEntry      *widget.Entry
	TResumeEntry     *widget.Entry
	SensorPollEntry  *widget.Entry
	WorkEntry        *widget.Entry
	RestEntry        *widget.Entry
	StateLabel       *widget.Label
	TempLabel        *widget.Label
	StartButton      *widget.Button
	PauseButton      *widget.Button
	StopButton       *widget.Button
	ErrorLabel       *widget.Label

	devices []devices.Device // last successful Lister.List result, aligned with DeviceSelect.Options

	mu        sync.Mutex // guards running, copier, pauseGate, ctxCancel, runDone, pollStop
	running   bool
	copier    *engine.Copier
	pauseGate *ManualPause
	ctxCancel context.CancelFunc
	runDone   chan struct{}
	pollStop  chan struct{}
}

// New builds the main window with injected dependencies (device lister; clock
// defaults to real time) and performs the initial device enumeration. The UI
// language is detected from locale environment variables (SPEC §7).
func New(a fyne.App, lister DeviceLister) *UI {
	return NewWithI18N(a, lister, NewI18N(DetectLanguage()))
}

// NewWithI18N is like New but uses an explicit localization instance instead of
// DetectLanguage; tests pin a language for deterministic widget text.
func NewWithI18N(a fyne.App, lister DeviceLister, i *I18N) *UI {
	if a == nil {
		panic("ui: app is required")
	}
	if lister == nil {
		lister = ProductionLister{}
	}
	u := &UI{
		App:          a,
		I18N:         i,
		Lister:       lister,
		Clock:        engine.RealClock{},
		PollInterval: defaultPollInterval,
		Queue:        engine.NewQueue(),
	}
	u.buildWidgets()

	w := a.NewWindow("ChillTransfer")
	w.Resize(fyne.NewSize(780, 640))
	w.SetContent(u.Content())
	w.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) { u.HandleDrop(uris) })
	u.Window = w

	u.refreshDevices() // initial load; errors are reported in ErrorLabel
	return u
}

// Content assembles the main window layout (SPEC §6): device panel on top,
// drop zone + queue in the middle, mode settings and controls at the bottom.
func (u *UI) Content() fyne.CanvasObject {
	deviceRow := container.NewBorder(
		nil, nil,
		widget.NewLabel(u.I18N.T(KeyDeviceLabel)),
		u.RefreshButton,
		u.DeviceSelect,
	)
	dropZone := container.NewVBox(
		widget.NewLabelWithStyle(u.I18N.T(KeyDropZone), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		container.New(layout.NewCenterLayout(), container.NewHBox(u.AddFilesButton, u.AddFolderButton)),
	)
	progressRow := container.NewHBox(u.CurrentFileLabel, layout.NewSpacer(), u.SpeedLabel, u.ETALabel)
	modePanel := container.NewVBox(
		container.NewHBox(widget.NewLabel(u.I18N.T(KeyModeLabel)), u.ModeRadio),
		container.NewHBox(
			widget.NewLabel(u.I18N.T(KeyParamTPause)), u.TPauseEntry, widget.NewLabel(u.I18N.T(KeyUnitDegC)),
			widget.NewLabel(u.I18N.T(KeyParamTResume)), u.TResumeEntry, widget.NewLabel(u.I18N.T(KeyUnitDegC)),
			widget.NewLabel(u.I18N.T(KeyParamPoll)), u.SensorPollEntry, widget.NewLabel(u.I18N.T(KeyUnitSec)),
		),
		container.NewHBox(
			widget.NewLabel(u.I18N.T(KeyParamWork)), u.WorkEntry, widget.NewLabel(u.I18N.T(KeyUnitMin)),
			widget.NewLabel(u.I18N.T(KeyParamRest)), u.RestEntry, widget.NewLabel(u.I18N.T(KeyUnitMin)),
		),
	)
	stateRow := container.NewHBox(u.StateLabel, layout.NewSpacer(), u.TempLabel)
	controlsRow := container.NewHBox(u.StartButton, u.PauseButton, u.StopButton)

	return container.NewPadded(container.NewVBox(
		deviceRow,
		u.SensorLabel,
		widget.NewSeparator(),
		dropZone,
		widget.NewSeparator(),
		widget.NewLabel(u.I18N.T(KeyQueueLabel)),
		u.QueueList,
		widget.NewSeparator(),
		u.ProgressBar,
		progressRow,
		stateRow,
		widget.NewSeparator(),
		modePanel,
		controlsRow,
		u.ErrorLabel,
	))
}

func (u *UI) buildWidgets() {
	u.DeviceSelect = widget.NewSelect(nil, nil)
	u.DeviceSelect.OnChanged = u.onDeviceSelected
	u.SensorLabel = widget.NewLabel(u.I18N.T(KeySensorNone))
	u.RefreshButton = widget.NewButton(u.I18N.T(KeyBtnRefresh), func() { u.refreshDevices() })

	u.AddFilesButton = widget.NewButton(u.I18N.T(KeyBtnAddFiles), u.onAddFilesTapped)
	u.AddFolderButton = widget.NewButton(u.I18N.T(KeyBtnAddFolder), u.onAddFolderTapped)

	u.QueueList = widget.NewList(
		func() int { return len(u.Queue.Items()) },
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewLabel(""), layout.NewSpacer(), widget.NewButton("✕", func() {}))
		},
		func(id widget.ListItemID, item fyne.CanvasObject) { u.updateQueueRow(id, item) },
	)
	u.QueueList.Resize(fyne.NewSize(0, 150)) // reserve height inside the VBox layout

	u.ProgressBar = widget.NewProgressBar()
	u.CurrentFileLabel = widget.NewLabel("")
	u.SpeedLabel = widget.NewLabel(u.I18N.T(KeySpeedLabel) + " —")
	u.ETALabel = widget.NewLabel(u.I18N.T(KeyETALabel) + " —")

	// Radio options carry the localized labels; selection logic maps them back
	// to canonical modes via selectedMode (T-007).
	u.ModeRadio = widget.NewRadioGroup([]string{u.modeLabel(ModeSensor), u.modeLabel(ModeCycle)}, nil)
	u.ModeRadio.Horizontal = true
	u.ModeRadio.SetSelected(u.modeLabel(ModeCycle)) // safe default before a device is selected

	// Defaults per SPEC §5: 50/42 °C, 3 s poll, 10 min work / 5 min rest.
	u.TPauseEntry = widget.NewEntry()
	u.TPauseEntry.SetText(strconv.FormatFloat(engine.DefaultTPauseC, 'f', -1, 64))
	u.TResumeEntry = widget.NewEntry()
	u.TResumeEntry.SetText(strconv.FormatFloat(engine.DefaultTResumeC, 'f', -1, 64))
	u.SensorPollEntry = widget.NewEntry()
	u.SensorPollEntry.SetText(strconv.Itoa(DefaultPollSeconds))
	u.WorkEntry = widget.NewEntry()
	u.WorkEntry.SetText(strconv.Itoa(int(engine.DefaultWorkDuration.Minutes())))
	u.RestEntry = widget.NewEntry()
	u.RestEntry.SetText(strconv.Itoa(int(engine.DefaultRestDuration.Minutes())))

	u.StateLabel = widget.NewLabel(u.I18N.T(KeyStateLabel) + " " + u.I18N.StateText(engine.StateIdle))
	u.TempLabel = widget.NewLabel("")

	u.StartButton = widget.NewButton(u.I18N.T(KeyBtnStart), func() { _ = u.Start() })
	u.PauseButton = widget.NewButton(u.I18N.T(KeyBtnPause), u.TogglePause)
	u.StopButton = widget.NewButton(u.I18N.T(KeyBtnStop), u.Stop)
	u.ErrorLabel = widget.NewLabel("")

	u.setRunningUI(false)
}

// Running reports whether a copy run is in progress.
func (u *UI) Running() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.running
}

// SelectedDevice returns the device currently selected in the dropdown, or nil.
func (u *UI) SelectedDevice() *devices.Device { return u.selectedDevice() }

// HandleDrop adds dropped files/folders to the queue (FR-2). It is extracted
// from the window's SetOnDropped callback so headless tests can drive it
// directly — the fyne/test driver does not implement drop events.
func (u *UI) HandleDrop(uris []fyne.URI) {
	if len(uris) == 0 {
		return
	}
	paths := make([]string, 0, len(uris))
	for _, uri := range uris {
		paths = append(paths, uri.Path())
	}
	u.addPaths(paths)
}

// onAddFilesTapped opens a native file dialog; the chosen file is added to the queue.
func (u *UI) onAddFilesTapped() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if reader == nil || err != nil {
			return // cancelled or no selection
		}
		path := reader.URI().Path()
		_ = reader.Close()
		u.addPaths([]string{path})
	}, u.Window)
}

// onAddFolderTapped opens a native folder dialog; the chosen folder is added to the queue.
func (u *UI) onAddFolderTapped() {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if uri == nil || err != nil {
			return // cancelled or no selection
		}
		u.addPaths([]string{uri.Path()})
	}, u.Window)
}

func (u *UI) addPaths(paths []string) {
	if err := u.Queue.Add(paths); err != nil {
		u.setError(err.Error())
		return
	}
	u.clearError()
	u.refreshQueueList()
}

// refreshDevices re-enumerates devices via Lister, rebuilds the dropdown and
// restores the previous selection by device ID (SPEC FR-1 "Обновить").
func (u *UI) refreshDevices() {
	devs, err := u.Lister.List(false)
	if err != nil {
		u.setError(fmt.Sprintf(u.I18N.T(KeyErrDevices), err))
		return
	}
	prevID := ""
	if dev := u.selectedDevice(); dev != nil {
		prevID = dev.ID
	}
	u.devices = devs

	options := make([]string, 0, len(devs))
	for _, d := range devs {
		options = append(options, deviceOptionText(d))
	}
	u.DeviceSelect.Options = options

	selected := false
	if prevID != "" {
		for i, d := range devs {
			if d.ID == prevID {
				if u.DeviceSelect.Selected != options[i] {
					u.DeviceSelect.SetSelected(options[i])
				}
				selected = true
				break
			}
		}
	}
	switch {
	case !selected && len(devs) > 0:
		u.DeviceSelect.SetSelected(options[0]) // initial load: pick the first device
	case !selected && len(devs) == 0:
		u.SensorLabel.SetText(u.I18N.T(KeySensorNone))
		u.TempLabel.SetText("")
	}
}

// onDeviceSelected updates the sensor status line and auto-selects the mode
// suggested by the device (sensor → Датчик, otherwise Цикл); the user can
// override the radio afterwards (SPEC §4).
func (u *UI) onDeviceSelected(_ string) {
	dev := u.selectedDevice()
	if dev == nil {
		u.SensorLabel.SetText(u.I18N.T(KeySensorNone))
		u.TempLabel.SetText("")
		return
	}
	if dev.TempSource != nil {
		u.SensorLabel.SetText(u.I18N.T(KeySensorFound))
		u.ModeRadio.SetSelected(u.modeLabel(ModeSensor))
	} else {
		u.SensorLabel.SetText(u.I18N.T(KeySensorMissing))
		u.ModeRadio.SetSelected(u.modeLabel(ModeCycle))
	}
	u.updateTemp()
}

// modeLabel returns the user-visible radio option label for a canonical copy
// mode; ModeRadio options are localized while selection logic stays on the
// language-neutral ModeSensor/ModeCycle identifiers.
func (u *UI) modeLabel(mode string) string {
	if mode == ModeSensor {
		return u.I18N.T(KeyModeSensor)
	}
	return u.I18N.T(KeyModeCycle)
}

// selectedMode maps the radio's localized selection back to a canonical copy
// mode; any unexpected value falls back to cycle.
func (u *UI) selectedMode() string {
	if u.ModeRadio.Selected == u.modeLabel(ModeSensor) {
		return ModeSensor
	}
	return ModeCycle
}

func (u *UI) deviceIndex() int {
	for i, opt := range u.DeviceSelect.Options {
		if opt == u.DeviceSelect.Selected {
			return i
		}
	}
	return -1
}

func (u *UI) selectedDevice() *devices.Device {
	i := u.deviceIndex()
	if i < 0 || i >= len(u.devices) {
		return nil
	}
	return &u.devices[i]
}

// updateTemp shows the live temperature of the selected device when a sensor source exists.
func (u *UI) updateTemp() {
	dev := u.selectedDevice()
	if dev == nil || dev.TempSource == nil {
		u.TempLabel.SetText("")
		return
	}
	temp, ok := dev.TempSource.Read()
	if !ok {
		u.TempLabel.SetText(u.I18N.T(KeyTempUnknown))
		return
	}
	u.TempLabel.SetText(fmt.Sprintf(u.I18N.T(KeyTempValue), temp))
}

func (u *UI) refreshQueueList() { u.QueueList.Refresh() }

// updateQueueRow renders one queue row: "name (size) — status" plus a remove
// button that is only available before start (FR-2).
func (u *UI) updateQueueRow(id widget.ListItemID, item fyne.CanvasObject) {
	items := u.Queue.Items()
	if int(id) >= len(items) {
		return
	}
	it := items[id]
	row := item.(*fyne.Container)
	label := row.Objects[0].(*widget.Label)
	removeBtn := row.Objects[2].(*widget.Button)

	label.SetText(fmt.Sprintf("%s (%s) — %s", currentFileText(it.Path), formatBytes(it.SizeBytes), it.Status.String()))
	if u.Running() {
		removeBtn.Hide() // removal is only allowed before start (FR-2)
		return
	}
	removeBtn.Show()
	removeBtn.OnTapped = func() { u.removeQueueItem(int(id)) }
}

// removeQueueItem removes a queue item by index; it is a no-op while a run is active.
func (u *UI) removeQueueItem(index int) {
	if u.Running() {
		return
	}
	if err := u.Queue.Remove(index); err != nil {
		u.setError(err.Error())
		return
	}
	u.clearError()
	u.refreshQueueList()
}

// Start validates the selected device and mode inputs, builds the governor for
// the active mode and runs a new copier in a goroutine (FR-3). On failure it
// returns an error and shows it in ErrorLabel; on success progress widgets are
// refreshed by polling copier.Snapshot() every PollInterval.
func (u *UI) Start() error {
	if u.Running() {
		return errors.New("ui: run already in progress")
	}

	dev := u.selectedDevice()
	if dev == nil {
		return u.failStart(u.I18N.T(KeyErrSelectDevice))
	}
	if dev.MountPoint == "" {
		return u.failStart(u.I18N.T(KeyErrNotMounted))
	}

	mode := u.selectedMode()
	var (
		gov          engine.Governor
		tickInterval time.Duration // 0 keeps the copier default (500 ms)
	)
	switch mode {
	case ModeSensor:
		if dev.TempSource == nil {
			return u.failStart(u.I18N.T(KeyErrNoSensor))
		}
		tPause, err := parseFloatParam(u.I18N, u.TPauseEntry.Text, u.I18N.T(KeyParamTPause))
		if err != nil {
			return u.failStart(err.Error())
		}
		tResume, err := parseFloatParam(u.I18N, u.TResumeEntry.Text, u.I18N.T(KeyParamTResume))
		if err != nil {
			return u.failStart(err.Error())
		}
		pollSec, err := parseFloatParam(u.I18N, u.SensorPollEntry.Text, u.I18N.T(KeyParamPoll))
		if err != nil {
			return u.failStart(err.Error())
		}
		if verr := ValidateSensorParams(u.I18N, tPause, tResume, pollSec); verr != nil {
			return u.failStart(verr.Error())
		}
		g, gerr := engine.NewSensorGovernor(dev.TempSource, tPause, tResume)
		if gerr != nil {
			return u.failStart(gerr.Error())
		}
		gov = g
		tickInterval = time.Duration(pollSec * float64(time.Second)) // FR-4: sensor polling interval
	default: // ModeCycle (any unexpected value falls back to cycle)
		workMin, err := parseFloatParam(u.I18N, u.WorkEntry.Text, u.I18N.T(KeyParamWork))
		if err != nil {
			return u.failStart(err.Error())
		}
		restMin, err := parseFloatParam(u.I18N, u.RestEntry.Text, u.I18N.T(KeyParamRest))
		if err != nil {
			return u.failStart(err.Error())
		}
		if verr := ValidateCycleParams(u.I18N, workMin, restMin); verr != nil {
			return u.failStart(verr.Error())
		}
		g, gerr := engine.NewCycleGovernor(
			time.Duration(workMin*float64(time.Minute)),
			time.Duration(restMin*float64(time.Minute)))
		if gerr != nil {
			return u.failStart(gerr.Error())
		}
		gov = g
	}

	// Manual pause maps to the mode's "intentionally not copying" state:
	// paused_thermal for sensor mode, resting for cycle mode (see ManualPause).
	pauseState := engine.StateResting
	if mode == ModeSensor {
		pauseState = engine.StatePausedThermal
	}

	gate := NewManualPause(gov, pauseState)
	copier := engine.NewCopier(u.Queue, dev.MountPoint, gate, u.Clock)
	if tickInterval > 0 {
		copier.TickInterval = tickInterval
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var pollStop chan struct{}
	interval := u.PollInterval
	u.mu.Lock()
	if u.running { // double-check: a concurrent Start may have won the race
		u.mu.Unlock()
		cancel()
		return errors.New("ui: run already in progress")
	}
	u.copier = copier
	u.pauseGate = gate
	u.ctxCancel = cancel
	u.runDone = done
	if interval > 0 {
		pollStop = make(chan struct{})
		u.pollStop = pollStop
	} else {
		interval = -1 // no automatic loop; tests drive poll() manually
	}
	u.running = true
	u.mu.Unlock()

	go func() {
		defer close(done)
		if err := copier.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			u.setError(fmt.Sprintf(u.I18N.T(KeyErrCopy), err.Error())) // StopOnError path
		}
	}()
	if pollStop != nil {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					u.Poll()
				case <-pollStop:
					return
				}
			}
		}()
	}

	u.clearError()
	u.setRunningUI(true)
	return nil
}

// Stop cancels the active run context; the copier finishes with StateCancelled.
func (u *UI) Stop() {
	u.mu.Lock()
	cancel := u.ctxCancel
	running := u.running
	u.mu.Unlock()
	if running && cancel != nil {
		cancel()
	}
}

// TogglePause flips the manual pause gate (FR-3 "Пауза/Продолжить"). While
// paused, the copier blocks within one governor tick and reports the mode's
// pause state; resuming takes effect on the next allowed tick.
func (u *UI) TogglePause() {
	u.mu.Lock()
	gate := u.pauseGate
	running := u.running
	u.mu.Unlock()
	if !running || gate == nil {
		return
	}
	paused := !gate.Paused()
	gate.SetPaused(paused)
	if paused {
		u.PauseButton.SetText(u.I18N.T(KeyBtnResume))
	} else {
		u.PauseButton.SetText(u.I18N.T(KeyBtnPause))
	}
}

// Poll refreshes progress, speed/ETA, state and temperature widgets from a
// copier snapshot. It is called by the polling ticker while a run is active;
// with PollInterval == 0 tests call it directly from a single goroutine.
func (u *UI) Poll() {
	u.mu.Lock()
	copier := u.copier
	running := u.running
	u.mu.Unlock()
	if !running || copier == nil {
		return
	}

	snap := copier.Snapshot()
	u.ProgressBar.SetValue(snap.OverallPercent / 100)
	u.CurrentFileLabel.SetText(currentFileText(snap.CurrentFile))
	u.SpeedLabel.SetText(u.I18N.T(KeySpeedLabel) + " " + formatSpeed(snap.SpeedBps))
	if snap.ETA > 0 {
		u.ETALabel.SetText(u.I18N.T(KeyETALabel) + " " + formatDuration(snap.ETA))
	} else {
		u.ETALabel.SetText(u.I18N.T(KeyETALabel) + " —")
	}
	u.StateLabel.SetText(u.I18N.T(KeyStateLabel) + " " + u.I18N.StateText(snap.State))
	u.updateTemp()

	if !snap.State.Active() {
		u.finalizeRun()
	}
}

// finalizeRun stops the polling loop and re-enables idle controls after a terminal state.
func (u *UI) finalizeRun() {
	u.mu.Lock()
	if !u.running {
		u.mu.Unlock()
		return
	}
	u.running = false
	if u.pollStop != nil {
		close(u.pollStop)
	}
	u.ctxCancel = nil
	u.pauseGate = nil
	u.mu.Unlock()

	u.setRunningUI(false)
	u.refreshQueueList() // final item statuses (done/failed/skipped)
}

// setRunningUI toggles control availability for the running/idle phase: while a
// run is active the destination device and mode parameters are fixed.
func (u *UI) setRunningUI(running bool) {
	if running {
		u.StartButton.Disable()
		u.PauseButton.Enable()
		u.StopButton.Enable()
		u.DeviceSelect.Disable()
		u.RefreshButton.Disable()
		u.ModeRadio.Disable()
		u.TPauseEntry.Disable()
		u.TResumeEntry.Disable()
		u.SensorPollEntry.Disable()
		u.WorkEntry.Disable()
		u.RestEntry.Disable()
	} else {
		u.StartButton.Enable()
		u.PauseButton.Disable()
		u.StopButton.Disable()
		u.DeviceSelect.Enable()
		u.RefreshButton.Enable()
		u.ModeRadio.Enable()
		u.TPauseEntry.Enable()
		u.TResumeEntry.Enable()
		u.SensorPollEntry.Enable()
		u.WorkEntry.Enable()
		u.RestEntry.Enable()
		u.PauseButton.SetText(u.I18N.T(KeyBtnPause))
	}
	u.refreshQueueList() // rebuild rows: remove buttons only when idle
}

func (u *UI) setError(msg string) { u.ErrorLabel.SetText(msg) }

func (u *UI) clearError() { u.ErrorLabel.SetText("") }

func (u *UI) failStart(msg string) error {
	u.setError(msg)
	return errors.New("ui: " + msg)
}

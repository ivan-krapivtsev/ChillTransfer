package ui

import (
	"os"
	"strings"

	"github.com/ivan-krapivtsev/chilltransfer/internal/engine"
)

// Supported UI languages (SPEC §7: RU + EN).
const (
	LangRU = "ru" // Russian — the product's default language
	LangEN = "en" // English — fallback for any other locale
)

// I18N holds the user-visible string table for one UI language. Obtain an
// instance via NewI18N and translate with T, which fails soft: an unknown key
// is returned unchanged so a missing translation degrades to the key text
// instead of an empty label.
type I18N struct {
	lang  string
	table map[string]string
}

// DetectLanguage selects the UI language from locale environment variables
// with standard precedence: LC_ALL if set, otherwise LANG. A "ru" prefix such
// as "ru_RU.UTF-8" selects Russian; any other value — and unset or empty
// variables — fall back to English.
func DetectLanguage() string {
	for _, name := range []string{"LC_ALL", "LANG"} {
		if v, ok := os.LookupEnv(name); ok && v != "" {
			if strings.HasPrefix(strings.ToLower(v), LangRU) {
				return LangRU
			}
			return LangEN
		}
	}
	return LangEN
}

// NewI18N returns the string tables for lang (LangRU or LangEN, case
// insensitive). Any other language code falls back to English so a malformed
// locale never yields an empty UI.
func NewI18N(lang string) *I18N {
	if strings.ToLower(lang) == LangRU {
		return &I18N{lang: LangRU, table: ruStrings}
	}
	return &I18N{lang: LangEN, table: enStrings}
}

// T translates key into the active language. Unknown keys are returned as-is
// (fail-soft); a nil receiver behaves like an empty lookup.
func (i *I18N) T(key string) string {
	if i == nil {
		return key
	}
	if v, ok := i.table[key]; ok {
		return v
	}
	return key
}

// Lang reports the active language code (LangRU or LangEN).
func (i *I18N) Lang() string { return i.lang }

// StateText maps an engine state to its user-visible name. Unknown states keep
// the engine identifier so a new state never renders as an empty label.
func (i *I18N) StateText(s engine.State) string {
	switch s {
	case engine.StateIdle:
		return i.T(KeyStateIdle)
	case engine.StateCopying:
		return i.T(KeyStateCopying)
	case engine.StatePausedThermal:
		return i.T(KeyStatePausedThermal)
	case engine.StateResting:
		return i.T(KeyStateResting)
	case engine.StateDone:
		return i.T(KeyStateDone)
	case engine.StateError:
		return i.T(KeyStateError)
	case engine.StateCancelled:
		return i.T(KeyStateCancelled)
	default:
		return s.String()
	}
}

// String keys for every user-visible UI label (SPEC §7). Production code must
// reference these constants only; allKeys lists them so tests can assert both
// language tables stay complete.
const (
	KeyDeviceLabel   = "device.label"   // device panel: dropdown caption
	KeySensorNone    = "sensor.none"    // sensor status: no device selected
	KeySensorFound   = "sensor.found"   // sensor status: temperature source present
	KeySensorMissing = "sensor.missing" // sensor status: no temperature source
	KeyBtnRefresh    = "btn.refresh"    // device panel refresh button

	KeyDropZone     = "dropzone.text"  // drop zone hint
	KeyBtnAddFiles  = "btn.add_files"  // add-files dialog button
	KeyBtnAddFolder = "btn.add_folder" // add-folder dialog button

	KeyQueueLabel = "queue.label" // queue list caption

	KeySpeedLabel = "speed.label" // progress line: throughput caption
	KeyETALabel   = "eta.label"   // progress line: ETA caption

	KeyStateLabel         = "state.label" // state line caption
	KeyStateIdle          = "state.idle"  // engine.StateIdle
	KeyStateCopying       = "state.copying"
	KeyStatePausedThermal = "state.paused_thermal"
	KeyStateResting       = "state.resting"
	KeyStateDone          = "state.done"
	KeyStateError         = "state.error"
	KeyStateCancelled     = "state.cancelled"

	KeyTempUnknown = "temp.unknown" // temperature line: no readable value
	KeyTempValue   = "temp.value"   // temperature line, format "%.1f" + unit

	KeyModeLabel    = "mode.label"  // mode settings caption
	KeyModeSensor   = "mode.sensor" // radio option: sensor mode label
	KeyModeCycle    = "mode.cycle"  // radio option: cycle mode label
	KeyParamTPause  = "param.t_pause"
	KeyParamTResume = "param.t_resume"
	KeyUnitDegC     = "unit.deg_c" // degree-Celsius unit next to T entries
	KeyParamPoll    = "param.poll" // sensor poll interval entry caption
	KeyUnitSec      = "unit.sec"   // seconds unit next to the poll entry
	KeyParamWork    = "param.work" // cycle work duration entry caption
	KeyUnitMin      = "unit.min"   // minutes unit next to work/rest entries
	KeyParamRest    = "param.rest" // cycle rest duration entry caption

	KeyBtnStart  = "btn.start"  // run controls: start
	KeyBtnPause  = "btn.pause"  // run controls: pause (idle label)
	KeyBtnResume = "btn.resume" // run controls: resume while paused
	KeyBtnStop   = "btn.stop"   // run controls: stop

	KeyErrDevices      = "err.devices"       // device enumeration failed, format "%v"
	KeyErrSelectDevice = "err.select_device" // Start with no device selected
	KeyErrNotMounted   = "err.not_mounted"   // Start on an unmounted device
	KeyErrNoSensor     = "err.no_sensor"     // sensor mode without a temperature source
	KeyErrCopy         = "err.copy"          // copier stopped on error, format "%s"

	KeyErrTPauseRange  = "err.t_pause_range"  // T_pause outside [30..80] °C
	KeyErrTResumeRange = "err.t_resume_range" // T_resume outside [30..80] °C
	KeyErrHysteresis   = "err.hysteresis"     // T_resume must be below T_pause
	KeyErrPollPositive = "err.poll_positive"  // poll interval must be > 0
	KeyErrWorkRange    = "err.work_range"     // work duration outside [1..60] min
	KeyErrRestRange    = "err.rest_range"     // rest duration outside [1..60] min
	KeyErrBadNumber    = "err.bad_number"     // non-numeric entry value, format "%s: ... %q"
)

// allKeys is the explicit list of every UI string key; tests assert both
// language tables cover exactly these keys.
var allKeys = []string{
	KeyDeviceLabel, KeySensorNone, KeySensorFound, KeySensorMissing, KeyBtnRefresh,
	KeyDropZone, KeyBtnAddFiles, KeyBtnAddFolder,
	KeyQueueLabel,
	KeySpeedLabel, KeyETALabel,
	KeyStateLabel, KeyStateIdle, KeyStateCopying, KeyStatePausedThermal,
	KeyStateResting, KeyStateDone, KeyStateError, KeyStateCancelled,
	KeyTempUnknown, KeyTempValue,
	KeyModeLabel, KeyModeSensor, KeyModeCycle,
	KeyParamTPause, KeyParamTResume, KeyUnitDegC, KeyParamPoll, KeyUnitSec,
	KeyParamWork, KeyUnitMin, KeyParamRest,
	KeyBtnStart, KeyBtnPause, KeyBtnResume, KeyBtnStop,
	KeyErrDevices, KeyErrSelectDevice, KeyErrNotMounted, KeyErrNoSensor, KeyErrCopy,
	KeyErrTPauseRange, KeyErrTResumeRange, KeyErrHysteresis, KeyErrPollPositive,
	KeyErrWorkRange, KeyErrRestRange, KeyErrBadNumber,
}

// ruStrings is the Russian table (the product's default language). Values with
// % verbs are format strings used via fmt.Sprintf.
var ruStrings = map[string]string{
	KeyDeviceLabel:   "Устройство:",
	KeySensorNone:    "Датчик: —",
	KeySensorFound:   "Датчик: обнаружен",
	KeySensorMissing: "Датчик: не обнаружен",
	KeyBtnRefresh:    "Обновить",

	KeyDropZone:     "Перетащите файлы и папки сюда",
	KeyBtnAddFiles:  "Добавить файлы",
	KeyBtnAddFolder: "Добавить папку",

	KeyQueueLabel: "Очередь:",

	KeySpeedLabel: "Скорость:",
	KeyETALabel:   "ETA:",

	KeyStateLabel:         "Состояние:",
	KeyStateIdle:          "ожидание",
	KeyStateCopying:       "копирование",
	KeyStatePausedThermal: "тепловая пауза",
	KeyStateResting:       "отдых",
	KeyStateDone:          "готово",
	KeyStateError:         "ошибка",
	KeyStateCancelled:     "отменено",

	KeyTempUnknown: "Т: —",
	KeyTempValue:   "Т: %.1f°C",

	KeyModeLabel:    "Режим:",
	KeyModeSensor:   "Датчик",
	KeyModeCycle:    "Цикл",
	KeyParamTPause:  "T_pause",
	KeyParamTResume: "T_resume",
	KeyUnitDegC:     "°C",
	KeyParamPoll:    "опрос",
	KeyUnitSec:      "с",
	KeyParamWork:    "Работа",
	KeyUnitMin:      "мин",
	KeyParamRest:    "Отдых",

	KeyBtnStart:  "Старт",
	KeyBtnPause:  "Пауза",
	KeyBtnResume: "Продолжить",
	KeyBtnStop:   "Стоп",

	KeyErrDevices:      "устройства: %v",
	KeyErrSelectDevice: "выберите устройство",
	KeyErrNotMounted:   "устройство не смонтировано — выберите другое",
	KeyErrNoSensor:     "датчик не обнаружен — используйте режим «Цикл»",
	KeyErrCopy:         "копирование: %s",

	KeyErrTPauseRange:  "T_pause: %g вне диапазона %d…%d °C",
	KeyErrTResumeRange: "T_resume: %g вне диапазона %d…%d °C",
	KeyErrHysteresis:   "T_resume (%g) должен быть меньше T_pause (%g)",
	KeyErrPollPositive: "интервал опроса: %g с — должно быть больше нуля",
	KeyErrWorkRange:    "рабочий интервал: %g мин вне диапазона %d…%d",
	KeyErrRestRange:    "отдых: %g мин вне диапазона %d…%d",
	KeyErrBadNumber:    "%s: некорректное число %q",
}

// enStrings is the English table. Values with % verbs are format strings used
// via fmt.Sprintf.
var enStrings = map[string]string{
	KeyDeviceLabel:   "Device:",
	KeySensorNone:    "Sensor: —",
	KeySensorFound:   "Sensor: detected",
	KeySensorMissing: "Sensor: not found",
	KeyBtnRefresh:    "Refresh",

	KeyDropZone:     "Drop files and folders here",
	KeyBtnAddFiles:  "Add files",
	KeyBtnAddFolder: "Add folder",

	KeyQueueLabel: "Queue:",

	KeySpeedLabel: "Speed:",
	KeyETALabel:   "ETA:",

	KeyStateLabel:         "State:",
	KeyStateIdle:          "idle",
	KeyStateCopying:       "copying",
	KeyStatePausedThermal: "thermal pause",
	KeyStateResting:       "resting",
	KeyStateDone:          "done",
	KeyStateError:         "error",
	KeyStateCancelled:     "cancelled",

	KeyTempUnknown: "Temp: —",
	KeyTempValue:   "Temp: %.1f°C",

	KeyModeLabel:    "Mode:",
	KeyModeSensor:   "Sensor",
	KeyModeCycle:    "Cycle",
	KeyParamTPause:  "T_pause",
	KeyParamTResume: "T_resume",
	KeyUnitDegC:     "°C",
	KeyParamPoll:    "Poll",
	KeyUnitSec:      "s",
	KeyParamWork:    "Work",
	KeyUnitMin:      "min",
	KeyParamRest:    "Rest",

	KeyBtnStart:  "Start",
	KeyBtnPause:  "Pause",
	KeyBtnResume: "Resume",
	KeyBtnStop:   "Stop",

	KeyErrDevices:      "devices: %v",
	KeyErrSelectDevice: "select a device",
	KeyErrNotMounted:   "device is not mounted — select another one",
	KeyErrNoSensor:     "sensor not detected — use Cycle mode",
	KeyErrCopy:         "copy error: %s",

	KeyErrTPauseRange:  "T_pause: %g out of range %d–%d °C",
	KeyErrTResumeRange: "T_resume: %g out of range %d–%d °C",
	KeyErrHysteresis:   "T_resume (%g) must be less than T_pause (%g)",
	KeyErrPollPositive: "poll interval: %g s — must be greater than zero",
	KeyErrWorkRange:    "work duration: %g min out of range %d–%d",
	KeyErrRestRange:    "rest duration: %g min out of range %d–%d",
	KeyErrBadNumber:    "%s: invalid number %q",
}

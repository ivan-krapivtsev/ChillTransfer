package profiles

// Canonical mode identifiers stored in Profile.Mode (SPEC FR-4/FR-5). These are
// storage values written to profiles.json; UI captions for the same modes live
// with the UI and are not persisted.
const (
	ModeSensor = "sensor" // thermal controller driven by a temperature sensor
	ModeCycle  = "cycle"  // fixed work/rest cycle
)

// Profile is the persisted settings block for one device (SPEC FR-6).
//
// Zero-value semantics: DeviceID is the only required field. Every other field
// left at its zero value means "no preference stored — use the project default",
// so a profile never freezes today's defaults into the file and upgrading the
// defaults still affects devices saved before the upgrade:
//
//	Mode         "" → auto: sensor when the device has a sensor, else cycle
//	TPauseC      0 → engine.DefaultTPauseC (50 °C, SPEC §5)
//	TResumeC     0 → engine.DefaultTResumeC (42 °C, SPEC §5)
//	PollSeconds  0 → default sensor poll interval (3 s, SPEC §5)
//	WorkMinutes  0 → engine.DefaultWorkDuration (10 min, SPEC §5)
//	RestMinutes  0 → engine.DefaultRestDuration (5 min, SPEC §5)
//
// The store performs no range validation: the UI owns input validation
// (SPEC §5 ranges) before calling Store.Set.
type Profile struct {
	// DeviceID matches devices.Device.ID — USB serial on Windows, media name
	// plus size on macOS, model/serial or kernel name on Linux.
	DeviceID string `json:"deviceID"`

	// Mode is ModeSensor or ModeCycle; "" means "decide automatically".
	Mode string `json:"mode,omitempty"`

	// TPauseC and TResumeC are sensor-mode thresholds in Celsius
	// (pause at >= TPauseC, resume at <= TResumeC).
	TPauseC  float64 `json:"tPauseC,omitempty"`
	TResumeC float64 `json:"tResumeC,omitempty"`

	// PollSeconds is the sensor polling interval in seconds.
	PollSeconds int `json:"pollSeconds,omitempty"`

	// WorkMinutes and RestMinutes are cycle-mode phase durations.
	WorkMinutes int `json:"workMinutes,omitempty"`
	RestMinutes int `json:"restMinutes,omitempty"`
}

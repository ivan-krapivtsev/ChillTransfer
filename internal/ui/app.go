// Package ui implements the Fyne user interface for ChillTransfer (T-005):
// device selection, drop zone and queue, mode settings and run controls
// (SPEC §6), localized RU/EN with system light/dark theming (T-007, SPEC §7).
// Dependencies (device lister, clock) are injected so headless tests never
// touch real hardware.
package ui

import "fyne.io/fyne/v2"

// NewWindow creates the main application window wired to production
// dependencies (platform device enumeration, real clock). Tests use New with
// an injected DeviceLister instead. The UI language is detected from locale
// environment variables (DetectLanguage).
//
// Theming (SPEC §7): no theme is set here on purpose — Fyne v2 follows the
// system light/dark appearance by default: app settings resolve the OS variant
// at startup (verified against fyne v2.8.2: app/settings.go setupTheme →
// internal/app.DefaultVariant, e.g. AppKit NSAppearance on macOS). A manual
// light/dark toggle is deferred to v1.x: Fyne 2.8 only offers the deprecated
// theme.Light/DarkTheme for runtime switching and no public way back to
// "follow system".
func NewWindow(a fyne.App) fyne.Window {
	return NewWithI18N(a, ProductionLister{}, NewI18N(DetectLanguage())).Window
}

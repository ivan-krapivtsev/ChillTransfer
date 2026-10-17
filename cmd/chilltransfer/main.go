package main

import (
	"fyne.io/fyne/v2/app"

	"github.com/ivan-krapivtsev/chilltransfer/internal/ui"
)

func main() {
	a := app.NewWithID("com.chilltransfer.app")
	w := ui.NewWindow(a)
	w.ShowAndRun()
}

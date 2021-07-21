package main

import (
	"fmt"
	"os"
)

func main() {
	restore, err := EnableRawMode()
	shutdown(err)
	defer restore()

	config := NewConfig()
	editor := NewEditor(config)
	for {
		editor.RefreshScreen()
		editor.ProcessKeyPress()
	}
}

func shutdown(err error) {
	if err == nil {
		return
	}

	_ = fmt.Errorf("shutdown for error %s occurs", err)

	exit(1)
}

func exit(code int) {
	_, _ = os.Stdout.WriteString(
		"\x1b[2J" + // clean the screen
			"\x1b[H") // reposition the cursor

	os.Exit(code)
}

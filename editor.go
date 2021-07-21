package main

import (
	"bufio"
	"os"
)

type Editor struct {
	reader *bufio.Reader
	writer *bufio.Writer
	config *Config
}

func NewEditor(config *Config) *Editor {
	return &Editor{
		reader: bufio.NewReader(os.Stdin),
		writer: bufio.NewWriter(os.Stdout),
		config: config,
	}
}

func ctrlKey(k byte) rune {
	return rune(k & 0x1f)
}

func (e *Editor) ReadKey() rune {
	char, _, _ := e.reader.ReadRune()

	// Determine if need handle empty input
	//for ; size != 0 || (err != nil && err != io.EOF); {
	//	char, size, err = reader.ReadRune()
	//}

	return char
}

func (e *Editor) ProcessKeyPress() {
	char := e.ReadKey()
	switch char {
	case ctrlKey('q'):
		exit(0)
	}
}

func (e *Editor) RefreshScreen() {
	// \x1b => <esc>

	// clean the screen
	e.writer.WriteString("\x1b[2J")
	// reposition the cursor
	e.writer.WriteString("\x1b[H")

	// draw rows
	for y := 0; y < e.config.screenRows; y++ {
		e.writer.WriteString("~\r\n")
	}

	// reposition the cursor
	e.writer.WriteString("\x1b[H")
	e.writer.Flush()
}

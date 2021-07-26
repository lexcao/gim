package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unsafe"
)

type (
	EditorRow struct {
		line   string
		render string
	}

	EditorConfig struct {
		originTermios          *syscall.Termios
		x, y                   int
		renderX                int
		screenRows, screenCols int
		offRow, offCol         int
		rows                   []EditorRow
		dirty                  bool
		filename               string
		statusMessage          string
	}
)

var (
	E        = &EditorConfig{}
	writeBuf = bufio.NewWriter(os.Stdin)
)

const (
	EscapeChar           = '\x1b'
	Escape               = string(EscapeChar) // <esc>
	CleanScreen          = Escape + "[2J"
	CleanLine            = Escape + "[K"
	CursorReposition     = Escape + "[H"
	CursorForwardFaraway = Escape + "[999C"
	CursorDownFaraway    = Escape + "[999B"
	CursorPosition       = Escape + "[6n"
	CursorHide           = Escape + "[?25l"
	CursorShow           = Escape + "[?25h"
	ColorInverted        = Escape + "[7m"
	ColorBack            = Escape + "[m"
	NewLine              = "\r\n"
	Tilde                = "~"

	TabStop = "    "
)

const (
	GimVersion = "0.0.1"
	EmptyFile  = "[New File]"
)

const (
	Enter      = '\r'
	Backspace  = 127
	ArrowLeft  = iota + 1000 // <esc>[D
	ArrowRight               // <esc>[C
	ArrowUp                  // <esc>[A
	ArrowDown                // <esc>[B
	HomeKey                  // <esc>[1~ | <esc>[7~ | <esc>[H | <esc>OH
	DelKey                   // <esc>[3~
	EndKey                   // <esc>[4~ | <esc>[8~ | <esc>[F | <esc>OF
	PageUp                   // <esc>[5~
	PageDown                 // <esc>[6~
)

func main() {
	EnableRawMode()
	defer DisableRawMode()

	initEditor()
	if len(os.Args) > 1 {
		editorOpen(os.Args[1])
	} else {
		//editorOpen("test.txt")
	}

	StatusMessage("HELP: Ctrl-s = save | Ctrl-q = quit | Ctrl-F = find")

	for {
		editorRefreshScreen()
		editorProcessKeyPress()
	}
}

/* init */

func initEditor() {
	E.screenRows, E.screenCols = GetWindowSize()
	E.screenRows -= 2 // 1 for status bar, 1 for status message
	E.filename = EmptyFile
}

/* file io */

func editorOpen(filename string) {
	file, err := os.Open(filename)
	maybe(err)
	defer file.Close()

	var rows []EditorRow
	reader := bufio.NewReader(file)

	// TODO determine \t is read properly
	for line, isPrefix, err := reader.ReadLine(); isPrefix || err == nil; {
		rows = append(rows, EditorRow{line: string(line)})
		line, isPrefix, err = reader.ReadLine()
	}

	E.rows = rows
	E.filename = filename
	editorRenderRows()
}

func editorSave() {
	if E.filename == EmptyFile {
		filename, ok := editorPrompt("Save as: ", nil)
		if !ok {
			StatusMessage("Save aborted")
			return
		}
		E.filename = filename
	}

	file, err := os.OpenFile(E.filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	maybe(err)
	defer file.Close()

	var size int
	writer := bufio.NewWriter(file)
	for _, row := range E.rows {
		size += len(row.line)
		writer.WriteString(row.line)
		writer.WriteString(NewLine)
	}
	writer.Flush()

	StatusMessage("%d bytes written to disk", size)

	E.dirty = false
}

func editorRenderRows() {
	for i := 0; i < len(E.rows); i++ {
		editorRenderRow(&E.rows[i])
	}
}

func editorRenderRow(row *EditorRow) {
	line := row.line
	// TODO verify \t placement works
	line = strings.ReplaceAll(line, "\t", TabStop)
	row.render = line
}

/* find */
func editorFind() {
	lastX, lastY := E.x, E.y
	lastOffCol, lastOffRow := E.offCol, E.offRow
	editorPrompt("Search: %s (Use ESC/Arrows/Enter)", editorFindCallBack)

	E.x, E.y = lastX, lastY
	E.offCol, E.offRow = lastOffCol, lastOffRow
}

var lastMatch = -1
var direction = 1

func editorFindCallBack(query string, key rune) {
	if key == EndKey || key == EscapeChar {
		lastMatch = -1
		direction = 1
		return
	} else if key == ArrowRight || key == ArrowDown {
		direction = 1
	} else if key == ArrowLeft || key == ArrowUp {
		direction = -1
	} else {
		lastMatch = -1
		direction = 1
	}

	if lastMatch == -1 {
		direction = 1
	}
	current := lastMatch

	for range E.rows {
		current += direction
		if current == -1 {
			current = len(E.rows) - 1
		} else if current == len(E.rows) {
			current = 0
		}

		row := E.rows[current]
		match := strings.Index(row.render, query)
		if match != -1 {
			lastMatch = current
			E.y = current
			E.x = Render2X(&row, match)
			E.offRow = len(E.rows)
			break
		}
	}

	StatusMessage("Not found %s", query)
}

/* Editor */

func ctrlKey(k byte) rune {
	return rune(k & 0x1f)
}

func editorScroll() {
	E.renderX = 0
	if row, ok := E.GetCurRow(); ok {
		E.renderX = X2Render(row, E.x)
	}

	if E.y < E.offRow {
		E.offRow = E.y
	}
	if E.y >= E.offRow+E.screenRows {
		E.offRow = E.y - E.screenRows + 1
	}
	if E.renderX < E.offCol {
		E.offCol = E.renderX
	}
	if E.renderX >= E.offCol+E.screenCols {
		E.offCol = E.renderX - E.screenCols + 1
	}
}

func (e *EditorConfig) GetCurRow() (row *EditorRow, ok bool) {
	if ok = e.y < len(e.rows); ok {
		row = &e.rows[e.y]
	}

	return
}

func editorMoveCursor(key rune) {
	row, ok := E.GetCurRow()

	switch key {
	case ArrowLeft:
		if E.x != 0 {
			E.x--
		} else if E.y > 0 {
			// move to the end of the previous line
			E.y--
			E.x = len(E.rows[E.y].line)
		}
	case ArrowRight:
		if ok && E.x < len(row.line) {
			E.x++
		} else if ok && E.x == len(row.line) {
			// move to the start of the next line
			E.y++
			E.x = 0
		}
	case ArrowUp:
		if E.y != 0 {
			E.y--
		}
	case ArrowDown:
		if E.y < len(E.rows) {
			E.y++
		}
	}

	if row, ok = E.GetCurRow(); ok && E.x > len(row.line) {
		E.x = len(row.line)
	} else if !ok {
		E.x = 0
	}
}

func editorMapArrowKey(key rune) rune {
	switch key {
	case 'A':
		return ArrowUp
	case 'B':
		return ArrowDown
	case 'C':
		return ArrowRight
	case 'D':
		return ArrowLeft

	}
	return EscapeChar
}

func editorPrompt(prompt string, callback func(string, rune)) (string, bool) {
	var buffer strings.Builder

	for {
		StatusMessage(prompt, buffer.String())
		editorRefreshScreen()

		char := editorReadKey()
		if char == Enter {
			StatusMessage("")
			if callback != nil {
				callback(buffer.String(), char)
			}
			return buffer.String(), true
		} else if char == DelKey || char == ctrlKey('h') || char == Backspace {
			last := buffer.String()[:buffer.Len()-1]
			buffer = strings.Builder{}
			buffer.WriteString(last)
		} else if char == EscapeChar {
			StatusMessage("")
			if callback != nil {
				callback(buffer.String(), char)
			}
			return "", false
		} else if !unicode.IsControl(char) && char < 128 {
			buffer.WriteRune(char)
		}
		if callback != nil {
			callback(buffer.String(), char)
		}
	}
}

func editorInsertRow(at int, line string) {
	source := E.rows
	if at < 0 || at > len(source) {
		return
	}

	dist := make([]EditorRow, at)
	copy(dist, source[:at])

	row := EditorRow{line: line}
	dist = append(dist, row)

	if at < len(source) {
		dist = append(dist, source[at:]...)
	}

	editorRenderRow(&dist[at])
	E.rows = dist
	E.dirty = true
}

func editorDeleteRow(at int) {
	source := E.rows
	if at < 0 || at > len(source) {
		return
	}

	dist := make([]EditorRow, at)
	copy(dist, source[:at])

	if at < len(source)-1 {
		dist = append(dist, source[at+1:]...)
	}

	E.rows = dist
	E.dirty = true
}

func editorRowAppendString(row *EditorRow, line string) {
	row.line = row.line + line
	editorRenderRow(row)

	E.dirty = true
}

func editorRowDeleteChar(row *EditorRow, at int) {
	if at < 0 || at >= len(row.line) {
		return
	}

	var builder strings.Builder
	builder.WriteString(row.line[:at])
	if at < len(row.line)-1 {
		builder.WriteString(row.line[at+1:])
	}

	row.line = builder.String()
	editorRenderRow(row)
	E.dirty = true
}

func editorRowInsertChar(row *EditorRow, at int, char rune) {
	if at < 0 || at > len(row.line) {
		at = len(row.line)
	}

	var builder strings.Builder
	builder.Write([]byte(row.line[:at]))

	builder.WriteByte(byte(char))

	if at < len(row.line) {
		builder.Write([]byte(row.line[at:]))
	}
	row.line = builder.String()

	editorRenderRow(row)
	E.dirty = true
}

func editorDrawRows() {
	for y := 0; y < E.screenRows; y++ {
		writeBuf.WriteString(CleanLine)

		rowIndex := y + E.offRow
		if rowIndex < len(E.rows) {
			row := E.rows[rowIndex].render
			l := len(row)
			if l > E.screenCols {
				l = E.screenCols
			}
			// TODO handle moving horizontally
			row = row[:l]

			writeBuf.WriteString(row)
		} else {
			if len(E.rows) == 0 && y == E.screenRows/3 {
				editorDrawWelcome()
			} else {
				writeBuf.WriteString(Tilde)
			}
		}
		writeBuf.WriteString(NewLine)
	}
}

func editorDrawWelcome() {
	welcome := fmt.Sprintf("gim editor -- version %s", GimVersion)
	if len(welcome) > E.screenCols {
		welcome = welcome[:E.screenCols]
	}
	padding := (E.screenCols - len(welcome)) / 2
	if padding > 0 {
		writeBuf.WriteString(Tilde)
	}
	for ; padding > 0; padding-- {
		writeBuf.WriteString(" ")
	}

	writeBuf.WriteString(welcome)
}

func editorDrawStatusBar() {
	writeBuf.WriteString(ColorInverted)

	var builder strings.Builder
	builder.WriteString(E.filename)
	builder.WriteString(" - ")
	builder.WriteString(strconv.Itoa(len(E.rows)))
	builder.WriteString(" lines")
	if E.dirty {
		builder.WriteString(" (modified)")
	}

	leftStatus := builder.String()
	writeBuf.WriteString(leftStatus)
	rightStatus := fmt.Sprintf("%d/%d", E.y+1, len(E.rows))

	// padding middle
	for i := len(leftStatus); i < E.screenCols-len(rightStatus); i++ {
		writeBuf.WriteString(" ")
	}

	writeBuf.WriteString(rightStatus)
	writeBuf.WriteString(NewLine)

	writeBuf.WriteString(ColorBack)
}

func StatusMessage(format string, arg ...interface{}) {
	E.statusMessage = fmt.Sprintf(format, arg...)
	go func() {
		select {
		case <-time.After(5 * time.Second):
			E.statusMessage = ""
		}
	}()
}

func editorDrawStatusMessage() {
	writeBuf.WriteString(CleanLine)
	l := len(E.statusMessage)

	if l > E.screenCols {
		l = E.screenCols
	}

	if l > 0 {
		writeBuf.WriteString(E.statusMessage[:l])
	}
}

func editorRefreshScreen() {
	editorScroll()

	writeBuf.WriteString(CursorHide)
	writeBuf.WriteString(CursorReposition)

	editorDrawRows()
	editorDrawStatusBar()
	editorDrawStatusMessage()

	writeBuf.WriteString(move(E.y-E.offRow+1, E.renderX-E.offCol+1))
	writeBuf.WriteString(CursorShow)
	writeBuf.Flush()
}

func editorInsertNewLine() {
	if E.x == 0 {
		editorInsertRow(E.y, "")
	} else {
		line := E.rows[E.y].line
		editorInsertRow(E.y+1, line[E.x:])
		E.rows[E.y].line = line[:E.x]
		editorRenderRow(&E.rows[E.y])
	}

	E.y++
	E.x = 0
}

func editorInsertChar(char rune) {
	if E.y == len(E.rows) {
		editorInsertRow(len(E.rows), "")
	}
	editorRowInsertChar(&E.rows[E.y], E.x, char)
	E.x++
}

func editorDeleteChar() {
	if E.y == len(E.rows) {
		editorDeleteRow(len(E.rows))
		E.y--
		return
	}
	if E.x == 0 && E.y == 0 {
		return
	}

	row := &E.rows[E.y]
	if E.x > 0 {
		editorRowDeleteChar(row, E.x-1)
		E.x--
	} else {
		upRow := &E.rows[E.y-1]
		E.x = len(upRow.line)
		editorRowAppendString(upRow, row.line)
		editorDeleteRow(E.y)
		E.y--
	}
}

var quitTimes = 3

func editorProcessKeyPress() {
	c := editorReadKey()
	StatusMessage(string(c))

	switch c {
	case Enter:
		editorInsertNewLine()

	case ctrlKey('q'):
		if E.dirty && quitTimes > 0 {
			StatusMessage("WARNING!! File has unsaved changes. Press Ctrl-q %d more times to quit", quitTimes)
			quitTimes--
			return
		}
		exit(0)
	case ctrlKey('s'):
		editorSave()
	case ctrlKey('f'):
		editorFind()
	case PageUp, PageDown:
		if c == PageUp {
			E.y = E.offRow
		} else {
			E.y = E.offRow + E.screenRows - 1
			if E.y > len(E.rows) {
				E.y = len(E.rows)
			}
		}

		for times := E.screenRows; times > 0; times-- {
			if c == PageUp {
				editorMoveCursor(ArrowUp)
			} else {
				editorMoveCursor(ArrowDown)
			}
		}
	case HomeKey:
		E.x = 0
	case EndKey:
		if E.y < len(E.rows) {
			E.x = len(E.rows[E.y].line)
		}
	case DelKey:
		editorMoveCursor(ArrowRight)
		fallthrough
	case Backspace, ctrlKey('h'):
		editorDeleteChar()
	case ArrowUp, ArrowDown, ArrowRight, ArrowLeft:
		editorMoveCursor(c)
	case ctrlKey('l'), EscapeChar:

	default:
		editorInsertChar(c)
	}

	quitTimes = 3
}

func readRune() rune {
	var (
		buffer [1]byte
		size   int
		err    error
	)

	for size, err = os.Stdin.Read(buffer[:]); size != 1; {
		size, err = os.Stdin.Read(buffer[:])
	}

	maybe(err)

	return rune(buffer[0])
}

func editorReadKey() (char rune) {
	char = readRune()

	if char != EscapeChar {
		return
	}

	// <esc>[
	return editorReadMoreKey()
}

func editorReadMoreKey() rune {
	var buffer [2]byte
	if size, _ := os.Stdin.Read(buffer[:]); size != 2 {
		return EscapeChar
	}

	if buffer[0] == '[' {
		if buffer[1] >= '0' && buffer[1] <= '9' {
			var oneMoreByte [1]byte
			if size, _ := os.Stdin.Read(oneMoreByte[:]); size != 1 {
				return EscapeChar
			}

			if oneMoreByte[0] == '~' {
				switch buffer[1] {
				case '1':
					return HomeKey
				case '3':
					return DelKey
				case '4':
					return EndKey
				case '5':
					return PageUp
				case '6':
					return PageDown
				case '7':
					return HomeKey
				case '8':
					return EndKey
				}
			}

		} else {
			switch buffer[1] {
			case 'A', 'B', 'C', 'D':
				return editorMapArrowKey(rune(buffer[1]))
			case 'H':
				return HomeKey
			case 'F':
				return EndKey
			}
		}
	} else if buffer[0] == 'O' {
		switch buffer[1] {
		case 'H':
			return HomeKey
		case 'F':
			return EndKey
		}
	}

	return EscapeChar
}

/* Terminal */

func EnableRawMode() {
	origin := tcGetAttr(syscall.Stdin)
	E.originTermios = origin

	raw := SetRawTermios(*origin)

	tcSetAttr(syscall.Stdin, raw)
}

func DisableRawMode() {
	tcSetAttr(syscall.Stdin, E.originTermios)
}

func tcSetAttr(fd int, termios *syscall.Termios) {
	_, _, errNo := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		syscall.TIOCSETAF,
		uintptr(unsafe.Pointer(termios)),
	)

	if errNo != 0 {
		log.Fatalf("Problem setting termial attributes: %s\n", errNo)
	}
}

func tcGetAttr(fd int) *syscall.Termios {
	termios := &syscall.Termios{}
	_, _, errNo := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		syscall.TIOCGETA,
		uintptr(unsafe.Pointer(termios)),
	)

	if errNo != 0 {
		log.Fatalf("Problem getting termial attributes: %s\n", errNo)
	}

	return termios
}

func ioctlGetWinSize(ws *WinSize) syscall.Errno {
	_, _, errNo := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(syscall.Stdout),
		syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&ws)),
	)
	return errNo
}

func SetRawTermios(term syscall.Termios) *syscall.Termios {
	term.Lflag &^= syscall.ECHO | // echo the input
		syscall.ICANON | // disable canonical mode
		syscall.ISIG | // disable C-C and C-Z
		syscall.IEXTEN // disable C-V, and fix C-O

	term.Iflag &^= syscall.IXON | // disable C-S and C-Q
		syscall.ICRNL | // fix C-M, carriage return and new line
		syscall.BRKINT | // disable break SIGINT signal
		syscall.INPCK | // disable parity checking
		syscall.ISTRIP // 8th bit of each input byte

	term.Oflag &^= syscall.OPOST // disable post processing of output

	term.Cflag |= syscall.CS8 // set character size to 8 bits per byte

	term.Cc[syscall.VMIN] = 0  // minimum number of bytes of input
	term.Cc[syscall.VTIME] = 1 // maximum amount of time to wait, current 1 / 10
	return &term
}

type WinSize struct {
	Row uint16
	Col uint16
}

func GetCursorPosition() (row int, col int) {
	// will response <esc>[24;80R
	exec(CursorPosition)

	var buf strings.Builder

	var i int
	for i < 32 {
		c := readRune()
		buf.WriteRune(c)
		if c == 'R' {
			break
		}
		i++
	}

	response := buf.String()[2:]
	fmt.Sscanf(response, "%d;%d", &row, &col)

	//fmt.Printf("\r\n buf: '%q' \r\n", response)
	//fmt.Printf("row: %d, col: %d", row, col)

	return
}

func GetWindowSize() (int, int) {
	var ws WinSize
	errNo := ioctlGetWinSize(&ws)

	if ws.Col != 0 {
		return int(ws.Row), int(ws.Col)
	} else if errNo == 0 {
		// move cursor to bottom-right corner, then get the position
		exec(CursorForwardFaraway + CursorDownFaraway)
		return GetCursorPosition()
	} else {
		maybe(errors.New("getWindowSize"))
		return 0, 0
	}
}

/* Utils */

func Render2X(row *EditorRow, render int) int {
	var curRender, x int
	for ; x < len(row.line); x++ {
		if row.line[x] == '\t' {
			curRender += 3
		}
		curRender++

		if curRender > render {
			return x
		}
	}
	return x
}
func X2Render(row *EditorRow, x int) int {
	var render int
	for j := 0; j < x; j++ {
		if row.line[j] == '\t' {
			render += 3
		}
		render++
	}
	return render
}
func move(x, y int) string {
	return fmt.Sprintf("%s[%d;%dH", Escape, x, y)
}

func exec(cmd string) {
	os.Stdout.WriteString(cmd)
}

func maybe(err error) {
	if err == nil {
		return
	}

	_ = fmt.Errorf("shutdown for error %s occurs", err)

	exit(1)
}

func exit(code int) {
	_, _ = os.Stdout.WriteString(CleanScreen + CursorReposition)

	os.Exit(code)
}

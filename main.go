package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
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
	NewLine              = "\r\n"
	Tilde                = "~"

	TabStop = "    "
)

const (
	GimVersion = "0.0.1"
)

const (
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
		editorOpen("test.txt")
	}

	for {
		editorRefreshScreen()
		editorProcessKeyPress()
	}
}

/* init */
func initEditor() {
	E.screenRows, E.screenCols = GetWindowSize()
}

/* file io */

func editorOpen(fileName string) {
	file, err := os.Open(fileName)
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
	editorRenderRow()
}

func editorRenderRow() {
	for i := 0; i < len(E.rows); i++ {
		line := E.rows[i].line
		// TODO verify \t placement works
		line = strings.ReplaceAll(line, "\t", TabStop)
		E.rows[i].render = line
	}
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
			writeBuf.WriteString(NewLine)
		} else {
			if len(E.rows) == 0 && y == E.screenRows/3 {
				editorDrawWelcome()
			} else {
				writeBuf.WriteString(Tilde)
			}

			if y < E.screenRows-1 {
				writeBuf.WriteString(NewLine)
			}
		}
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

func editorRefreshScreen() {
	editorScroll()

	writeBuf.WriteString(CursorHide)
	writeBuf.WriteString(CursorReposition)

	editorDrawRows()

	writeBuf.WriteString(move(E.y-E.offRow+1, E.renderX-E.offCol+1))
	writeBuf.WriteString(CursorShow)
	writeBuf.Flush()
}

func editorProcessKeyPress() {
	c := editorReadKey()

	switch c {
	case ctrlKey('q'):
		exit(0)
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

	case ArrowUp, ArrowDown, ArrowRight, ArrowLeft:
		editorMoveCursor(c)
	}
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

	buf := strings.Builder{}

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

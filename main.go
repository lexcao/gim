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
		size int
		line string
	}

	EditorConfig struct {
		originTermios          *syscall.Termios
		x, y                   int
		screenRows, screenCols int
		numRows                int
		row                    *EditorRow
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
	editorOpen()

	for {
		editorRefreshScreen()
		editorProcessKeyPress()
	}
}

/* init */
func initEditor() {
	E.x, E.y = 0, 0
	E.screenRows, E.screenCols = GetWindowSize()
	E.numRows = 0
}

/* file io */

func editorOpen() {
	line := "Hello, world!"
	E.row = &EditorRow{
		size: len(line),
		line: line,
	}
	E.numRows = 1
}

/* Editor */

func ctrlKey(k byte) rune {
	return rune(k & 0x1f)
}

func editorMoveCursor(key rune) {
	switch key {
	case ArrowDown:
		if E.x != E.screenCols-1 {
			E.x++
		}
	case ArrowUp:
		if E.x != 0 {
			E.x--
		}
	case ArrowLeft:
		if E.y != 0 {
			E.y--
		}
	case ArrowRight:
		if E.y != E.screenRows-1 {
			E.y++
		}
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
		if y < E.numRows {
			line := E.row.line

			if E.row.size > E.screenCols {
				line = line[:E.screenCols]
			}
			writeBuf.WriteString(line)
			writeBuf.WriteString(NewLine)

			continue
		}
		if y == E.screenRows/3 {
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
		} else {
			writeBuf.WriteString(Tilde)
		}

		writeBuf.WriteString(CleanLine)
		if y < E.screenRows-1 {
			writeBuf.WriteString(NewLine)
		}
	}
}

func editorRefreshScreen() {
	writeBuf.WriteString(CursorHide)
	writeBuf.WriteString(CursorReposition)

	editorDrawRows()

	writeBuf.WriteString(move(E.x+1, E.y+1))
	writeBuf.WriteString(CursorShow)
	writeBuf.Flush()
}

func editorProcessKeyPress() {
	c := editorReadKey()

	switch c {
	case ctrlKey('q'):
		exit(0)
	case PageUp, PageDown:
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
		E.x = E.screenCols - 1
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
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
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

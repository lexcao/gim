package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"
)

type Config struct {
	originTermios          *syscall.Termios
	screenRows, screenCols int
}

var (
	config = &Config{}
	w      = bufio.NewWriter(os.Stdin)
)

const (
	Escape           = "\x1b" // <esc>
	CleanScreen      = Escape + "[2J"
	RepositionCursor = Escape + "[H"
	NewLine          = "\r\n"
	Tilde            = "~"
	EmptyLine        = Tilde + NewLine
)

func main() {
	EnableRawMode()
	defer DisableRawMode()

	for {
		editorRefreshScreen()
		editorProcessKeyPress()
	}
}

/* Editor */

func ctrlKey(k byte) rune {
	return rune(k & 0x1f)
}

func editorDrawRows() {
	config.screenRows = 5
	for y := 0; y < config.screenRows; y++ {
		w.WriteString(EmptyLine)
	}
}

func editorRefreshScreen() {
	w.WriteString(CleanScreen)
	w.WriteString(RepositionCursor)

	editorDrawRows()

	w.WriteString(RepositionCursor)
	w.Flush()
}

func editorProcessKeyPress() {
	c := editorReadKey()

	switch c {
	case ctrlKey('q'):
		exit(0)
	}
}

func editorReadKey() rune {
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

/* Terminal */

func EnableRawMode() {
	origin := tcGetAttr(syscall.Stdin)
	config.originTermios = origin

	raw := SetRawTermios(*origin)

	tcSetAttr(syscall.Stdin, raw)
}

func DisableRawMode() {
	tcSetAttr(syscall.Stdin, config.originTermios)
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

func GetWindowSize() (int, int) {
	var ws WinSize
	_, _, errNo := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(syscall.Stdout),
		syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&ws)),
	)

	if errNo != 0 {
		return 0, 0
	}
	//if errNo  || ws.Col == 0 {
	//	if n, _ := os.Stdout.WriteString("\x1b[999C\x1b[999B"); n == 12 {
	//		_, _, _ = bufio.NewReader(os.Stdin).ReadRune()
	//	}
	//}
	return int(ws.Row), int(ws.Col)
}

/* Utils */

func maybe(err error) {
	if err == nil {
		return
	}

	_ = fmt.Errorf("shutdown for error %s occurs", err)

	exit(1)
}

func exit(code int) {
	_, _ = os.Stdout.WriteString(CleanScreen + RepositionCursor)

	os.Exit(code)
}

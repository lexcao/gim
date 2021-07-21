package main

import (
	"bufio"
	"os"
	"syscall"
	"unsafe"
)

func getTermios(fd uintptr) (*syscall.Termios, syscall.Errno) {
	var t syscall.Termios
	_, _, err := syscall.Syscall6(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCGETA,
		uintptr(unsafe.Pointer(&t)),
		0, 0, 0)

	return &t, err
}

func setTermios(fd uintptr, term *syscall.Termios) syscall.Errno {
	_, _, err := syscall.Syscall6(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCSETAF,
		uintptr(unsafe.Pointer(term)),
		0, 0, 0)
	return err
}

func setRaw(term *syscall.Termios) {
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
	term.Cc[syscall.VTIME] = 1 // maximum amount of time to wait, current 1 / 10 second
}

func EnableRawMode() (func(), error) {
	var err error
	fd := os.Stdin.Fd()
	t, errNo := getTermios(fd)
	if errNo != 0 {
		return func() {}, errNo
	}

	origin := t
	setRaw(t)
	errNo = setTermios(fd, t)
	if errNo != 0 {
		return func() {}, errNo
	}

	return func() {
		_ = setTermios(fd, origin)
	}, err
}

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

func GetWindowSize() (int, int) {
	var ws winsize
	retCode, _, _ := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)))

	if int(retCode) == -1 || ws.Col == 0 {
		if n, _ := os.Stdout.WriteString("\x1b[999C\x1b[999B"); n == 12 {
			_, _, _ = bufio.NewReader(os.Stdin).ReadRune()
		}
	}
	return int(ws.Row), int(ws.Col)
}

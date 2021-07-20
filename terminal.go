package main

import (
	"os"
	"syscall"
	"unsafe"
)

func getTermios(fd uintptr) *syscall.Termios {
	var t syscall.Termios
	_, _, err := syscall.Syscall6(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCGETA,
		uintptr(unsafe.Pointer(&t)),
		0, 0, 0)

	if err != 0 {
		panic(err)
	}

	return &t
}

func setTermios(fd uintptr, term *syscall.Termios) {
	_, _, err := syscall.Syscall6(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCSETAF,
		uintptr(unsafe.Pointer(term)),
		0, 0, 0)
	if err != 0 {
		panic(err)
	}
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
	fd := os.Stdin.Fd()
	t := getTermios(fd)
	origin := t
	setRaw(t)
	setTermios(fd, t)

	return func() {
		setTermios(fd, origin)
	}, nil
}

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"unicode"
)

func main() {
	restore, err := EnableRawMode()
	shutdown(err)
	defer restore()

	reader := bufio.NewReader(os.Stdin)
	for {
		char, err := reader.ReadByte()
		if err != nil && err != io.EOF {
			shutdown(err)
		}

		if unicode.IsControl(rune(char)) {
			fmt.Printf("%d\r\n", char)
		} else {
			fmt.Printf("%d ('%c')\r\n", char, char)
		}
		if char == 'q' {
			break
		}
	}
}

func shutdown(err error) {
	if err != nil {
		_ = fmt.Errorf("shutdown for error %s occurs", err)
		os.Exit(1)
	}
}

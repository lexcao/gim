package main

import "syscall"

type Config struct {
	originTermios          syscall.Termios
	screenRows, screenCols int
}

func NewConfig() *Config {
	c := Config{}

	rows, cols := GetWindowSize()
	c.screenRows, c.screenCols = rows, cols

	return &c
}

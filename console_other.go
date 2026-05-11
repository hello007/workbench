//go:build !windows

package main

import "fmt"

func consolePrint(msg string) {
	fmt.Print(msg)
}

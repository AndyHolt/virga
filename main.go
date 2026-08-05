/*
Copyright © 2026 Andy Holt <andrew.holt@hotmail.co.uk>
*/
package main

import (
	"os"

	"github.com/AndyHolt/virga/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

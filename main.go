package main

import (
	"os"

	"hr-cli/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}

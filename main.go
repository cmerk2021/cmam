// Command cmam is a tiny package manager for prebuilt Go CLI tools.
package main

import (
	"os"

	"github.com/cmerk2021/cmam/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}

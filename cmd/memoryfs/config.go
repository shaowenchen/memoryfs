package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/shaowenchen/memoryfs/pkg/cliconfig"
)

const configUsage = `memoryfs config — manage saved CLI connection info

Subcommands:
  show   print the saved config (mount writes this file on success)
  path   print the file path
  clear  delete the saved config

The file stores nodes/uri-prefix/api-token from the last successful
"memoryfs mount" so status/benchmark can be invoked without re-typing.
Override location via $MEMORYFS_CONFIG.
`

func runConfig(args []string) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, configUsage)
		os.Exit(2)
	}
	switch strings.ToLower(args[0]) {
	case "show":
		cfg, err := cliconfig.Load()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "no saved config (%s); run `memoryfs mount` first\n", cliconfig.Path())
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "load: %v\n", err)
			os.Exit(1)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "encode: %v\n", err)
			os.Exit(1)
		}
	case "path":
		fmt.Println(cliconfig.Path())
	case "clear":
		if err := cliconfig.Remove(); err != nil {
			fmt.Fprintf(os.Stderr, "clear: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("removed %s\n", cliconfig.Path())
	case "help", "-h", "--help":
		fmt.Print(configUsage)
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n\n%s", args[0], configUsage)
		os.Exit(2)
	}
}

package main

import (
	"errors"
	"log"
	"os"

	"github.com/shaowenchen/memoryfs/pkg/cliconfig"
	"github.com/shaowenchen/memoryfs/pkg/ports"
)

// connInfo is the resolved seed/uri-prefix/token for a CLI subcommand.
type connInfo struct {
	Seed      string
	URIPrefix string
	Token     string
	Source    string // "flag", "env", "config", or "default"
}

// resolveConn implements a uniform precedence chain for subcommands that
// need to talk to the cluster:
//
//	1. Explicit -nodes / -uri-prefix / -api-token flags.
//	2. $MEMORYFS_NODES / $MEMORYFS_URI_PREFIX / $MEMORYFS_API_TOKEN.
//	3. ~/.memoryfs/config.json (written by `memoryfs mount`).
//	4. Built-in defaults (http://127.0.0.1:19800, empty prefix, empty token).
//
// Empty fields in flags/env trigger the fallback per-field, so a user can
// override just the prefix without losing the saved nodes/token.
func resolveConn(flagSeed, flagPrefix, flagToken string) connInfo {
	out := connInfo{Source: "default"}

	out.Seed = firstNode(flagSeed)
	if out.Seed != "" {
		out.Source = "flag"
	} else if env := firstNode(os.Getenv("MEMORYFS_NODES")); env != "" {
		out.Seed = env
		out.Source = "env"
	}

	out.URIPrefix = flagPrefix
	if out.URIPrefix == "" {
		out.URIPrefix = os.Getenv("MEMORYFS_URI_PREFIX")
	}

	out.Token = flagToken
	if out.Token == "" {
		out.Token = os.Getenv("MEMORYFS_API_TOKEN")
	}

	if out.Seed == "" || (out.URIPrefix == "" && out.Token == "") {
		if cfg, err := cliconfig.Load(); err == nil {
			if out.Seed == "" {
				if n := firstNode(joinNodes(cfg.Nodes)); n != "" {
					out.Seed = n
					if out.Source == "default" {
						out.Source = "config"
					}
				}
			}
			if out.URIPrefix == "" && cfg.URIPrefix != "" {
				out.URIPrefix = cfg.URIPrefix
			}
			if out.Token == "" && cfg.APIToken != "" {
				out.Token = cfg.APIToken
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Printf("warning: load cli config: %v", err)
		}
	}

	if out.Seed == "" {
		out.Seed = ports.DefaultHTTPURL()
	}
	return out
}

func joinNodes(nodes []string) string {
	out := ""
	for _, n := range nodes {
		if out == "" {
			out = n
		} else {
			out += "," + n
		}
	}
	return out
}

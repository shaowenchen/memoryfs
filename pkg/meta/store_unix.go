//go:build !windows

package meta

import "syscall"

func syscallGetuid() int { return syscall.Getuid() }
func syscallGetgid() int { return syscall.Getgid() }

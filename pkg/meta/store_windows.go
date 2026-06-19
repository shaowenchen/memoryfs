//go:build windows

package meta

func syscallGetuid() int { return 0 }
func syscallGetgid() int { return 0 }

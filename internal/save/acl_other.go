//go:build !windows

package save

func restrictPrivateFile(string) error { return nil }

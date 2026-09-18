//go:build windows

package daemon

// Windows does not support fsync on a directory handle. The report file itself
// is flushed before the atomic rename; duplicate replay remains safe if a crash
// loses the directory entry or resurrects an acknowledged one.
func syncTerminalReportDir(string) error { return nil }

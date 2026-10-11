package trash

// sameDevice is never reached on Windows, which has no trash folgit uses.
func sameDevice(a, b string) bool { return false }

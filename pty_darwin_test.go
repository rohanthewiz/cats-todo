package main

import (
	"bytes"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPTY allocates a pty pair without a pty library: posix_openpt, grantpt and
// unlockpt are ioctls on the /dev/ptmx clone device on darwin, and
// TIOCPTYGNAME names the slave. The master stays a raw fd so the test can poll
// and close it without the runtime poller in the way.
func openPTY() (master int, slave *os.File, err error) {
	master, err = unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, nil, err
	}
	fail := func(err error) (int, *os.File, error) {
		_ = unix.Close(master)
		return -1, nil, err
	}

	for _, req := range []uintptr{unix.TIOCPTYGRANT, unix.TIOCPTYUNLK} {
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(master), req, 0); errno != 0 {
			return fail(errno)
		}
	}
	var name [128]byte
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(master), unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); errno != 0 {
		return fail(errno)
	}
	path := string(name[:bytes.IndexByte(name[:], 0)])

	slave, err = os.OpenFile(path, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return fail(err)
	}
	return master, slave, nil
}

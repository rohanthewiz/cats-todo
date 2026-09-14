package main

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPTY allocates a pty pair without a pty library: on linux unlockpt is
// TIOCSPTLCK on the /dev/ptmx clone device (grantpt is a no-op with devpts)
// and TIOCGPTN gives the slave's number under /dev/pts. The master stays a raw
// fd so the test can poll and close it without the runtime poller in the way.
func openPTY() (master int, slave *os.File, err error) {
	master, err = unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, nil, err
	}
	fail := func(err error) (int, *os.File, error) {
		_ = unix.Close(master)
		return -1, nil, err
	}

	var unlock int32
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(master), unix.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		return fail(errno)
	}
	n, err := unix.IoctlGetUint32(master, unix.TIOCGPTN)
	if err != nil {
		return fail(err)
	}

	slave, err = os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return fail(err)
	}
	return master, slave, nil
}

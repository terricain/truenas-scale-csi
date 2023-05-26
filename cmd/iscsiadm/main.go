package main

import (
	"os"
	"syscall"

	"k8s.io/klog/v2"
)

var iscsiPaths = []string{
	"/sbin/iscsiadm",
	"/usr/local/sbin/iscsiadm",
	"/bin/iscsiadm",
	"/usr/local/bin/iscsiadm",
}

func main() {
	chrootDir := os.Getenv("HOST_DIR")
	if chrootDir == "" {
		chrootDir = "/host"
	}

	if err := syscall.Chroot(chrootDir); err != nil {
		klog.ErrorS(err, "failed to chroot", "chrootDir", chrootDir)
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	iscsiadmPath := os.Getenv("ISCSIADM_PATH")
	if iscsiadmPath == "" {
		for _, path := range iscsiPaths {
			if _, err := os.Stat(path); err == nil {
				iscsiadmPath = path
				break
			}
		}
	}

	if iscsiadmPath == "" {
		klog.Error("iscsiadm binary not found, consider specifying ISCSIADM_PATH")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	// Replace first argument which is the binary path with the real
	// iscsiadm
	args := []string{iscsiadmPath}
	args = append(args, os.Args[1:]...)
	if err := syscall.Exec(iscsiadmPath, args, os.Environ()); err != nil {
		klog.ErrorS(err, "failed to exec", "execPath", iscsiadmPath)
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
}

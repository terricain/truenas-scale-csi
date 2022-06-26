package main

import (
	"os"
	"syscall"

	"github.com/rs/zerolog/log"
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
		log.Fatal().Err(err).Str("chroot_dir", chrootDir).Msg("Failed to chroot")
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
		log.Fatal().Msg("iscsiadm binary not found, consider specifying ISCSIADM_PATH")
	}

	// Replace first argument which is the binary path with the real
	// iscsiadm
	args := []string{iscsiadmPath}
	args = append(args, os.Args[1:]...)
	if err := syscall.Exec(iscsiadmPath, args, os.Environ()); err != nil {
		log.Fatal().Err(err).Msg("Failed to exec")
	}
}

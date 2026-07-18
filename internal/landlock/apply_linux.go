//go:build linux

package landlock

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

const handledReadAccess = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR

// Apply installs a read-only Landlock allowlist for the current process. It
// must be called in the agent child after setup and before exec. The caller
// supplies runtime roots explicitly because Landlock has no deny rules: broad
// roots are intentionally visible in the API and should never include the
// protected workspace.
func Apply(plan Plan) error {
	fd, err := createRuleset()
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	// Directory traversal/listing is allowed for the transaction root; actual
	// file reads still require an explicit per-file READ_FILE rule.
	if err := addPathRule(fd, plan.Root, unix.LANDLOCK_ACCESS_FS_READ_DIR); err != nil {
		return err
	}
	for _, root := range plan.RuntimeRoots {
		if err := addPathRule(fd, root, handledReadAccess); err != nil {
			return err
		}
	}
	for _, dir := range plan.AllowedDirs {
		if err := addPathRule(fd, dir, unix.LANDLOCK_ACCESS_FS_READ_DIR); err != nil {
			return err
		}
	}
	for _, file := range plan.AllowedFiles {
		if err := addPathRule(fd, file, unix.LANDLOCK_ACCESS_FS_READ_FILE); err != nil {
			return err
		}
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("apply Landlock: set no_new_privs: %w", err)
	}
	if _, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(fd), 0, 0, 0, 0, 0); errno != 0 {
		return fmt.Errorf("apply Landlock: restrict self: %w", errno)
	}
	return nil
}

func createRuleset() (int, error) {
	attr := unix.LandlockRulesetAttr{Access_fs: handledReadAccess}
	fd, _, errno := unix.Syscall6(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr), 0, 0, 0, 0,
	)
	if errno != 0 {
		return -1, fmt.Errorf("apply Landlock: create ruleset: %w", errno)
	}
	return int(fd), nil
}

func addPathRule(rulesetFD int, path string, access uint64) error {
	pathFD, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("apply Landlock: open rule path %s: %w", path, err)
	}
	defer unix.Close(pathFD)
	attr := unix.LandlockPathBeneathAttr{Allowed_access: access, Parent_fd: int32(pathFD)}
	if _, _, errno := unix.Syscall6(
		unix.SYS_LANDLOCK_ADD_RULE,
		uintptr(rulesetFD),
		uintptr(unix.LANDLOCK_RULE_PATH_BENEATH),
		uintptr(unsafe.Pointer(&attr)),
		0, 0, 0,
	); errno != 0 {
		return fmt.Errorf("apply Landlock: add rule %s: %w", path, errno)
	}
	return nil
}

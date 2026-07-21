//go:build windows

package platform

import (
	"fmt"
	"io"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WindowsJob owns the child process boundary. Filesystem enforcement remains
// the separately signed minifilter service; this helper never pretends that a
// Job Object protects files by itself.
type WindowsJob struct{ handle windows.Handle }

func StartInWindowsJob(command string, args ...string) (*exec.Cmd, *WindowsJob, error) {
	return StartInWindowsJobWithOptions(WindowsJobOptions{Command: command, Args: args})
}

// WindowsJobOptions keeps process launch configuration in one auditable
// object.  The caller should set Dir to the disposable VHDX workspace; this
// helper never treats a Job Object as filesystem protection.
type WindowsJobOptions struct {
	Command     string
	Args        []string
	Dir         string
	Environment []string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
}

func StartInWindowsJobWithOptions(options WindowsJobOptions) (*exec.Cmd, *WindowsJob, error) {
	if options.Command == "" {
		return nil, nil, fmt.Errorf("Windows Job Object command is required")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create Rewind Job Object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptrPointer(&info), uint32(sizeOf(info))); err != nil {
		_ = windows.CloseHandle(job)
		return nil, nil, fmt.Errorf("configure Rewind Job Object: %w", err)
	}
	cmd := exec.Command(options.Command, options.Args...)
	cmd.Dir = options.Dir
	cmd.Env = options.Environment
	cmd.Stdin = options.Stdin
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr
	if err := cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		return nil, nil, err
	}
	access := uint32(windows.PROCESS_QUERY_INFORMATION | windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE)
	process, err := windows.OpenProcess(access, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		_ = windows.CloseHandle(job)
		return nil, nil, fmt.Errorf("open child for Rewind Job Object: %w", err)
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		_ = cmd.Process.Kill()
		_ = windows.CloseHandle(job)
		return nil, nil, fmt.Errorf("assign child to Rewind Job Object: %w", err)
	}
	return cmd, &WindowsJob{handle: job}, nil
}

// Wait waits for the command and closes the Job Object in the same lifecycle
// step. Closing the job is what terminates any remaining descendants after a
// crash or an interrupted parent. Callers that need to inspect the process
// error can use the returned error directly.
func (j *WindowsJob) Wait(cmd *exec.Cmd) error {
	if cmd == nil {
		_ = j.Close()
		return fmt.Errorf("Windows Job Object command is nil")
	}
	err := cmd.Wait()
	closeErr := j.Close()
	if err != nil && closeErr != nil {
		return fmt.Errorf("wait Windows Job Object command: %v; close job: %w", err, closeErr)
	}
	if err != nil {
		return err
	}
	return closeErr
}

func (j *WindowsJob) Close() error {
	if j == nil || j.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(j.handle)
	j.handle = 0
	return err
}

// These tiny wrappers keep the ABI-specific unsafe conversion in one file.
// They are compiled only on Windows and are intentionally not exposed as a
// general memory API.
func uintptrPointer[T any](value *T) uintptr { return uintptr(unsafe.Pointer(value)) }
func sizeOf[T any](value T) uintptr          { return unsafe.Sizeof(value) }

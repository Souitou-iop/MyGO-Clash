package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

// Paths are where the service lives.
type Paths struct {
	Label      string // the service's name
	Bin        string
	Unit       string
	Home       string
	Socket     string
	CoreSocket string
	Logs       string
}

// Layout returns the paths of the service of the app named slug.
func Layout(slug string) Paths {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	name := slug + "-service"
	return Paths{
		Label:      name,
		Bin:        filepath.Join(pf, name, name+".exe"),
		Home:       filepath.Join(pd, name, "core"),
		Socket:     `\\.\pipe\` + name,
		CoreSocket: `\\.\pipe\` + name + "-core",
		Logs:       filepath.Join(pd, name, "logs"),
	}
}

func install(cfg Config) error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("installing the service needs an administrator")
	}
	if !sidRe.MatchString(cfg.Owner) {
		return fmt.Errorf("invalid owner %q", cfg.Owner)
	}
	p := Layout(cfg.Slug)
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if s, err := m.OpenService(p.Label); err == nil {
		stopAndWait(s)
		_ = s.Delete()
		s.Close()
		time.Sleep(time.Second) // the SCM deletes asynchronously
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := copyExecutable(exe, p.Bin); err != nil {
		return err
	}
	s, err := m.CreateService(p.Label, p.Bin, mgr.Config{
		DisplayName: "MyGO-Clash Service",
		Description: "Runs the MyGO-Clash core with the privileges TUN mode needs.",
		StartType:   mgr.StartAutomatic,
	}, "service", "run", "--name", cfg.Slug, "--owner", cfg.Owner)
	if err != nil {
		return err
	}
	defer s.Close()
	_ = s.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 2 * time.Second}}, 86400)
	return s.Start()
}

func uninstall(slug string) error {
	p := Layout(slug)
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if s, err := m.OpenService(p.Label); err == nil {
		stopAndWait(s)
		_ = s.Delete()
		s.Close()
	}
	time.Sleep(time.Second)
	_ = os.RemoveAll(filepath.Dir(p.Bin))
	_ = os.RemoveAll(filepath.Dir(p.Home))
	return nil
}

func stopAndWait(s *mgr.Service) {
	status, err := s.Control(0x00000001) // SERVICE_CONTROL_STOP
	if err != nil {
		return
	}
	for i := 0; i < 50 && status.State != 1; i++ { // SERVICE_STOPPED
		time.Sleep(100 * time.Millisecond)
		if status, err = s.Query(); err != nil {
			return
		}
	}
}

var (
	shell32         = windows.NewLazySystemDLL("shell32.dll")
	shellExecuteExW = shell32.NewProc("ShellExecuteExW")
)

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         windows.Handle
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     windows.Handle
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    windows.Handle
	dwHotKey     uint32
	hIcon        windows.Handle
	hProcess     windows.Handle
}

// Elevate runs the app's executable as an administrator with args, which
// shows the UAC prompt, and waits for it.
func Elevate(ctx context.Context, prompt string, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = syscall.EscapeArg(a)
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	info := shellExecuteInfo{fMask: 0x00000040 /* SEE_MASK_NOCLOSEPROCESS */, lpVerb: verb, lpFile: file, lpParameters: params, nShow: 0}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if r, _, err := shellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return ErrCanceled
		}
		return err
	}
	defer windows.CloseHandle(info.hProcess)
	for {
		ev, err := windows.WaitForSingleObject(info.hProcess, 200)
		if err != nil {
			return err
		}
		if ev == windows.WAIT_OBJECT_0 {
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the elevated command failed (exit code %d)", code)
	}
	return nil
}

// CurrentOwner identifies the user running the app to the service: their SID.
func CurrentOwner() string {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return ""
	}
	return u.User.Sid.String()
}

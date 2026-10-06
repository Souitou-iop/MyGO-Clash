//go:build windows

package paths

import "os"

func ownedByMe(os.FileInfo) bool { return true } // Windows uses named pipes

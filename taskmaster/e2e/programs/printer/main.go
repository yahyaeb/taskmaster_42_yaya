// printer writes observable runtime info to stdout, then runs forever.
// The daemon config redirects stdout to a file so tests can read it.
//
// Output lines:
//
//	ENV:<key>=<value>      for every env var prefixed with TM_
//	CWD:<path>             current working directory
//	UMASK_PERM:<octal>     permissions on a 0666 file created in the workingdir
//	                       (effective perms = 0666 & ~umask, proving umask was applied)
package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	// Print all env vars injected by the daemon config (prefix TM_ avoids noise).
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "TM_") {
			fmt.Println("ENV:" + e)
		}
	}

	// Print working directory set by the daemon config.
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println("CWD:error:" + err.Error())
	} else {
		fmt.Println("CWD:" + cwd)
	}

	// Create a file with mode 0666; effective permissions will be 0666 & ~umask.
	// This proves that the umask the daemon applied before exec took effect.
	testFile := "printer_umask_probe"
	f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err == nil {
		f.Close()
		if fi, err := os.Stat(testFile); err == nil {
			fmt.Printf("UMASK_PERM:%04o\n", fi.Mode().Perm())
		}
		os.Remove(testFile)
	}

	// Flush stdout before sleeping (important when stdout is a file).
	os.Stdout.Sync()

	// Stay alive so the daemon sees it as RUNNING, not as an immediate exit.
	for {
		time.Sleep(time.Hour)
	}
}

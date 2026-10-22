// This harmless helper is packaged only by the isolated NSIS regression test.
package main

import "os"

func main() {
    if os.Getenv("NEKOBOX_TEST_INSTALLER_FAIL")=="1" { os.Exit(7) }
}

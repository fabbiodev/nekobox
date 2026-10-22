package main

import (
	"strings"
	"testing"
)

func TestNSISPathIsLastAndUnquoted(t *testing.T) {
	for _, dir := range []string{`C:\Program Files\nekobox`, `E:\Apps (test) & stuff\NekoBox`} {
		got := nsisCommandLine(`C:\Downloads\update package.exe`, dir, "/CHOCOLATEY=0")
		if !strings.HasPrefix(got, `"C:\Downloads\update package.exe" /S /UNPACK=1 `) {
			t.Fatal(got)
		}
		if !strings.HasSuffix(got, " /D="+dir) || strings.Contains(got, `"/D=`) {
			t.Fatal(got)
		}
	}
}

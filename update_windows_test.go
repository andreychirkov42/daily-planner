package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceExe(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "Clepsy.exe")
	_ = os.WriteFile(self, []byte("старая"), 0o755)
	if err := replaceExe(self, []byte("новая")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(self); string(b) != "новая" {
		t.Fatalf("на месте exe: %q", b)
	}
	if b, _ := os.ReadFile(self + ".old"); string(b) != "старая" {
		t.Fatalf("старая версия должна остаться в .old до перезапуска: %q", b)
	}
	if _, err := os.Stat(self + ".new"); !os.IsNotExist(err) {
		t.Fatal("временный .new должен исчезнуть")
	}
}

package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// updatePlatform — ключ сборки этой системы в latest.json.
const updatePlatform = "windows-x86_64"

// installUpdate кладёт новую версию на место exe. Запущенный exe Windows не даёт перезаписать,
// но даёт переименовать: старый уходит в Clepsy.exe.old (его удалит новая версия при запуске).
// Только переименования и запуск самого себя — без скриптов и PowerShell, которые настораживают антивирус.
func installUpdate(b []byte) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return err
	}
	return replaceExe(self, b)
}

func replaceExe(self string, b []byte) error {
	newPath, oldPath := self+".new", self+".old"
	if err := os.WriteFile(newPath, b, 0o755); err != nil {
		return errors.New("не удалось записать новую версию рядом с программой: " + err.Error())
	}
	_ = os.Remove(oldPath)
	if err := os.Rename(self, oldPath); err != nil {
		_ = os.Remove(newPath)
		return errors.New("не удалось освободить место для новой версии: " + err.Error())
	}
	if err := os.Rename(newPath, self); err != nil {
		_ = os.Rename(oldPath, self) // возвращаем как было
		return errors.New("не удалось поставить новую версию: " + err.Error())
	}
	return nil
}

// restartAfterUpdate запускает новую версию с теми же параметрами и завершает эту.
// Окно программы было открыто — новая версия откроет его снова, иначе тихо встанет в трей.
func restartAfterUpdate() {
	self, _ := os.Executable()
	args := []string{"-restarted", "-updated-from=" + appVersion}
	for _, a := range os.Args[1:] {
		if a == "-autostart" || a == "-popup" || a == "-restarted" || strings.HasPrefix(a, "-updated-from") {
			continue
		}
		args = append(args, a)
	}
	if mainWindowShown == nil || !mainWindowShown() {
		args = append(args, "-autostart")
	}
	if err := exec.Command(self, args...).Start(); err != nil {
		log.Printf("перезапуск после обновления: %v", err)
		return
	}
	if quitApp != nil {
		quitApp()
		time.Sleep(5 * time.Second) // трей закрывается не мгновенно; страховка, если не закрылся
	}
	os.Exit(0)
}

// cleanupOldExe удаляет exe прошлой версии, оставшийся после обновления.
func cleanupOldExe() {
	self, err := os.Executable()
	if err != nil {
		return
	}
	old := self + ".old"
	for i := 0; i < 10; i++ { // старая версия может ещё завершаться
		if err := os.Remove(old); err == nil || os.IsNotExist(err) {
			return
		}
		time.Sleep(time.Second)
	}
}

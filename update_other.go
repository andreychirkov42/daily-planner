//go:build !windows

package main

import "errors"

// На других системах сервер сам не обновляется: на Android это делает оболочка на Java.
const updateAsset = ""

func installUpdate([]byte) error {
	return errors.New("обновление ставится только на Windows")
}

func restartAfterUpdate() {}

func cleanupOldExe() {}

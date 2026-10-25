//go:build !windows

package main

// runApp на других системах просто держит сервер запущенным.
func runApp(url string, s *Store, _ bool) {
	openBrowser(url)
	select {}
}

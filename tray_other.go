//go:build !windows

package main

// runApp на других системах просто держит сервер запущенным.
func runApp(o appOptions) {
	url := o.url
	openBrowser(url)
	select {}
}
